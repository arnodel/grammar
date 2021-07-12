package grammar

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

type SliceTokenStream struct {
	tokens     []Token
	currentPos int
}

func (s *SliceTokenStream) Next() Token {
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
