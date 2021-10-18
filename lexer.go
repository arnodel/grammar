package grammar

import (
	"errors"
	"fmt"
	"regexp"
)

// SimpleToken is a simple implementation of the both the Token and the Parser
// interfaces.  So it can be used in rules to match a token field.  It is also
// the concrete type of Tokens returned by SimpleTokenStream.
type SimpleToken struct {
	TokType  string
	TokValue string
}

var _ Token = SimpleToken{}

// EOF is the token that is returned when trying to consument an exhausted token
// stream.
var EOF = SimpleToken{
	TokType:  "EOF",
	TokValue: "EOF",
}

// Type returns the token type.
func (t SimpleToken) Type() string {
	return t.TokType
}

// Value returns the value of the token.
func (t SimpleToken) Value() string {
	return t.TokValue
}

// Parse tries to match the next token in the given TokenStream with the
// ParseOptions, using opts.MatchToken.  If they match, the receiver is loaded
// with the token data, if not a non-nil *ParseError is returned.  In any event
// the next token in the token stream has been consumed.
func (t *SimpleToken) Parse(_ interface{}, s TokenStream, opts ParseOptions) *ParseError {
	tok := s.Next()
	if err := opts.MatchToken(tok); err != nil {
		return &ParseError{
			Token: tok,
			Err:   err,
			Pos:   s.Save(),
		}
	}
	t.TokType = tok.Type()
	t.TokValue = tok.Value()
	return nil
}

// SimpleTokenStream is a very simple implementation of the TokenStream
// interface which the Parse function requires.
type SimpleTokenStream struct {
	tokens     []Token
	currentPos int
}

func NewSimpleTokenStream(toks []Token) *SimpleTokenStream {
	return &SimpleTokenStream{
		tokens: toks,
	}
}

var _ TokenStream = (*SimpleTokenStream)(nil)

// Next consumes the next token in the token stream and returns it.  If the
// stream is exhausted, EOF is returned.
func (s *SimpleTokenStream) Next() Token {
	if s.currentPos >= len(s.tokens) {
		// log.Print("Next token: EOF")
		return EOF
	}
	tok := s.tokens[s.currentPos]
	s.currentPos++
	// log.Printf("Next token: %q", tok.Value())
	return tok
}

// Save returns the current position in the token stream.
func (s *SimpleTokenStream) Save() int {
	// log.Print("Saving pos: ", s.currentPos)
	return s.currentPos
}

// Restore rewinds the token stream to the given position (which should have
// been obtained by s.Save()).  In general this may panic - in this
// implementation it's always possible to rewind.
func (s *SimpleTokenStream) Restore(pos int) {
	// log.Print("Restoring pos: ", pos)
	s.currentPos = pos
}

// A TokenDef defines a type of token and the pattern that matches it.  Used by
// SimpleTokeniser to create tokenisers simply.
type TokenDef struct {
	Ptn      string              // The regular expression the token should match
	Name     string              // The name given to this token type
	Special  func(string) string // If defined, it takes over the tokenising for this pattern
	Mode     string
	PushMode string
	PopMode  bool
}

type Mode struct {
}

// SimpleTokeniser takes a list of TokenDefs and returns a function that can
// tokenise a string.  Designed for simple use-cases.
func SimpleTokeniser(tokenDefs []TokenDef) func(string) (*SimpleTokenStream, error) {
	modeTokenDefs := make(map[string][]TokenDef)
	ptnStrings := make(map[string]string)
	for _, tokenDef := range tokenDefs {
		ptn := fmt.Sprintf(`(%s)`, tokenDef.Ptn)
		if _, ok := ptnStrings[tokenDef.Mode]; ok {
			ptnStrings[tokenDef.Mode] += "|" + ptn
		} else {
			ptnStrings[tokenDef.Mode] = `^(?:` + ptn
		}
		modeTokenDefs[tokenDef.Mode] = append(modeTokenDefs[tokenDef.Mode], tokenDef)
	}
	ptns := make(map[string]*regexp.Regexp)
	for m, s := range ptnStrings {
		ptns[m] = regexp.MustCompile(s + ")")
	}
	initialMode := tokenDefs[0].Mode
	return func(s string) (*SimpleTokenStream, error) {
		mode := initialMode
		var prevModes []string
		var toks []Token
		for len(s) > 0 {
			matches := ptns[mode].FindStringSubmatch(s)
			if matches == nil {
				return nil, fmt.Errorf("invalid input string")
			}
			tokType := ""
			tokValue := matches[0]
			for i, match := range matches[1:] {
				if match != "" {
					tokDef := modeTokenDefs[mode][i]
					if tokDef.Special != nil {
						tokValue = tokDef.Special(s)
					}
					tokType = tokDef.Name
					switch {
					case tokDef.PushMode != "":
						prevModes = append(prevModes, mode)
						mode = tokDef.PushMode
					case tokDef.PopMode:
						last := len(prevModes) - 1
						if last < 0 {
							return nil, errors.New("no mode to pop")
						}

						mode = prevModes[last]
						prevModes = prevModes[:last]
					}
					break
				}
			}
			if tokType != "" {
				toks = append(toks, SimpleToken{TokType: tokType, TokValue: tokValue})
			}
			s = s[len(tokValue):]
		}
		return &SimpleTokenStream{tokens: toks}, nil
	}
}
