package grammar

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

type Position struct {
	Line, Column int
}

func (p *Position) Advance(s string) {
	for len(s) > 0 {
		i := strings.IndexAny(s, "\r\n")
		if i == -1 {
			p.Column += len([]rune(s))
			return
		}
		p.Column = 1
		p.Line++
		if s[0] == '\r' && len(s) > 1 && s[1] == '\n' {
			i++
		}
		s = s[i+1:]
	}
}

func (p Position) String() string {
	return fmt.Sprintf("Ln %d, Col %d", p.Line, p.Column)
}

// SimpleToken is a simple implementation of the both the Token and the Parser
// interfaces.  So it can be used in rules to match a token field.  It is also
// the concrete type of Tokens returned by SimpleTokenStream.
type SimpleToken struct {
	TokType  string
	TokValue string
	TokPos   Position
}

var _ Token = SimpleToken{}
var _ Parser = &SimpleToken{}
var _ PrettyWriter = SimpleToken{}

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

func (t SimpleToken) Position() Position {
	return t.TokPos
}

func (t SimpleToken) PrettyWrite(out io.Writer, r any, pfx string, indent int) (err error) {
	return writeLine(out, pfx, indent, fmt.Sprintf("%s = %q %s", t.TokType, t.TokValue, t.TokPos))
}

// Parse tries to match the next token in the given TokenStream with the
// ParseOptions, using opts.MatchToken.  If they match, the receiver is loaded
// with the token data, if not a non-nil *ParseError is returned.  In any event
// the next token in the token stream has been consumed.
func (t *SimpleToken) Parse(_ interface{}, s *ParserState, opts TokenOptions) *ParseError {
	tok, err := opts.MatchNextToken(s)
	if err != nil {
		return err
	}
	t.TokType = tok.Type()
	t.TokValue = tok.Value()
	t.TokPos = tok.Position()
	// log.Printf("+++ parsed tok #%d: (%s, %q)", pos, t.TokType, t.TokValue)
	return nil
}

// SimpleTokenStream is a very simple implementation of the TokenStream
// interface which the Parse function requires.
type SimpleTokenStream struct {
	tokens     []Token
	currentPos int
	eof        Token // returned once the tokens are exhausted
}

// NewSimpleTokenStream makes a token stream from a slice of tokens.  The EOF
// token it returns at the end is positioned just after the last token.
func NewSimpleTokenStream(toks []Token) *SimpleTokenStream {
	pos := Position{Line: 1, Column: 1}
	if n := len(toks); n > 0 {
		pos = toks[n-1].Position()
		pos.Advance(toks[n-1].Value())
	}
	return &SimpleTokenStream{
		tokens: toks,
		eof:    eofAt(pos),
	}
}

func eofAt(pos Position) SimpleToken {
	return SimpleToken{TokType: EOF.TokType, TokValue: EOF.TokValue, TokPos: pos}
}

var _ TokenStream = (*SimpleTokenStream)(nil)

// Next consumes the next token in the token stream and returns it.  If the
// stream is exhausted, EOF is returned.
func (s *SimpleTokenStream) Next() Token {
	if s.currentPos >= len(s.tokens) {
		if s.eof != nil {
			return s.eof
		}
		return EOF
	}
	tok := s.tokens[s.currentPos]
	s.currentPos++
	// log.Printf("Next token: %d %q", s.currentPos-1, tok.Value())
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
		pos := Position{Line: 1, Column: 1}
		for len(s) > 0 {
			matches := ptns[mode].FindStringSubmatch(s)
			if matches == nil {
				return nil, fmt.Errorf("invalid token starting at %v", pos)
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
				toks = append(toks, SimpleToken{TokType: tokType, TokValue: tokValue, TokPos: pos})
			}
			s = s[len(tokValue):]
			pos.Advance(tokValue)
		}
		return &SimpleTokenStream{tokens: toks, eof: eofAt(pos)}, nil
	}
}
