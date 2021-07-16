package grammar

import (
	"fmt"
	"regexp"
	"strings"
)

type SimpleToken struct {
	tokType  string
	tokValue string
}

var EOF = SimpleToken{
	tokType:  "EOF",
	tokValue: "EOF",
}

func (t SimpleToken) Type() string {
	return t.tokType
}

func (t SimpleToken) Value() string {
	return t.tokValue
}

func (t *SimpleToken) Parse(s TokenStream, opts ParseOptions) *ParseError {
	tok := s.Next()
	if err := opts.MatchToken(tok); err != nil {
		return &ParseError{
			Token: tok,
			Err:   err,
			Pos:   s.Save(),
		}
	}
	t.tokType = tok.Type()
	t.tokValue = tok.Value()
	return nil
}

type SimpleTokenStream struct {
	tokens     []Token
	currentPos int
}

func (s *SimpleTokenStream) Next() Token {
	if s.currentPos >= len(s.tokens) {
		return EOF
	}
	tok := s.tokens[s.currentPos]
	s.currentPos++
	return tok
}

func (s *SimpleTokenStream) Save() int {
	return s.currentPos
}

func (s *SimpleTokenStream) Restore(pos int) {
	s.currentPos = pos
}

type TokenDef struct {
	Ptn  string
	Name string
}

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
					tokType = tokenDefs[i].Name
					break
				}
			}
			if tokType != "" {
				toks = append(toks, SimpleToken{tokType: tokType, tokValue: tokValue})
			}
			s = s[len(tokValue):]
		}
		return &SimpleTokenStream{tokens: toks}, nil
	}
}
