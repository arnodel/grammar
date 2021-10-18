package grammar

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

// SimpleToken is a simple implementation of the both the Token and the Parser
// interfaces.  So it can be used in rules to match a token field.  It is also
// the concrete type of Tokens returned by SimpleTokenStream.
type SimpleToken struct {
	TokType  string
	TokValue string
}

var _ Token = SimpleToken{}
var _ Parser = &SimpleToken{}

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
func (t *SimpleToken) Parse(_ interface{}, s *ParserState, opts ParseOptions) *ParseError {
	pos := s.Save()
	tok := s.Next()
	if err := opts.MatchToken(tok); err != nil {
		parseErr := &ParseError{
			Token: tok,
			Err:   err,
			Pos:   pos,
		}
		// log.Printf("!!! Token Parse Error: %s", parseErr)
		return parseErr
	}
	t.TokType = tok.Type()
	t.TokValue = tok.Value()
	// log.Printf("+++ parsed tok #%d: (%s, %q)", pos, t.TokType, t.TokValue)
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

func (s *SimpleTokenStream) Dump(w io.Writer) {
	for i, tok := range s.tokens {
		currentMarker := " "
		if i == s.currentPos {
			currentMarker = "*"
		}
		fmt.Fprintf(w, "%3d%s %s %q\n", i, currentMarker, tok.Type(), tok.Value())
	}
}

// A TokenDef defines a type of token and the pattern that matches it.  Used by
// SimpleTokeniser to create tokenisers simply.
type TokenDef struct {
	Ptn     string              // The regular expression the token should match
	Name    string              // The name given to this token type
	Special func(string) string // If defined, it takes over the tokenising for this pattern
}

// SimpleTokeniser takes a list of TokenDefs and returns a function that can
// tokenise a string.  Designed for simple use-cases.
func SimpleTokeniser(tokenDefs []TokenDef) func(string) (*SimpleTokenStream, error) {
	ptns := make([]string, len(tokenDefs))
	for i, tokenDef := range tokenDefs {
		ptns[i] = fmt.Sprintf(`(%s)`, tokenDef.Ptn)
	}
	ptn := regexp.MustCompile(fmt.Sprintf(`^(?:%s)`, strings.Join(ptns, "|")))
	return func(s string) (*SimpleTokenStream, error) {
		var toks []Token
		for len(s) > 0 {
			matches := ptn.FindStringSubmatch(s)
			if matches == nil {
				return nil, fmt.Errorf("invalid input string")
			}
			tokType := ""
			tokValue := matches[0]
			for i, match := range matches[1:] {
				if match != "" {
					tokDef := tokenDefs[i]
					if tokDef.Special != nil {
						tokValue = tokDef.Special(s)
					}
					tokType = tokDef.Name
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
