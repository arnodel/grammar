package json

import (
	"fmt"
	"regexp"

	"github.com/arnodel/grammar"
)

type Token struct {
	tokType  string
	tokValue string
}

var EOF = Token{
	tokType:  "EOF",
	tokValue: "EOF",
}

func (t Token) Type() string {
	return t.tokType
}

func (t Token) Value() string {
	return t.tokValue
}

func (t *Token) Parse(s grammar.TokenStream, opts grammar.ParseOptions) error {
	tok := s.Next()
	if err := opts.MatchToken(tok); err != nil {
		return err
	}
	t.tokType = tok.Type()
	t.tokValue = tok.Value()
	return nil
}

type SliceTokenStream struct {
	tokens     []grammar.Token
	currentPos int
}

func (s *SliceTokenStream) Next() grammar.Token {
	if s.currentPos >= len(s.tokens) {
		return EOF
	}
	tok := s.tokens[s.currentPos]
	s.currentPos++
	return tok
}

func (s *SliceTokenStream) Save() int {
	return s.currentPos
}

func (s *SliceTokenStream) Restore(pos int) {
	s.currentPos = pos
}

var jsonTokenPtn = regexp.MustCompile(`^(?:(\s+)|(null)|(true|false)|([{},:[\]])|("[^"]*")|(-?[0-9]+(?:\.[0-9]+)?))`)
var tokTypes = []string{"space", "null", "bool", "op", "string", "number"}

func TokeniseJsonString(s string) (*SliceTokenStream, error) {
	var toks []grammar.Token
	for len(s) > 0 {
		var tokType string
		matches := jsonTokenPtn.FindStringSubmatch(s)
		if matches == nil {
			return nil, fmt.Errorf("invalid json")
		}
		tokValue := matches[0]
		for i, match := range matches[1:] {
			if match != "" {
				tokType = tokTypes[i]
				break
			}
		}
		if tokType != "space" {
			toks = append(toks, Token{tokType: tokType, tokValue: tokValue})
		}
		s = s[len(tokValue):]
	}
	return &SliceTokenStream{tokens: toks}, nil
}
