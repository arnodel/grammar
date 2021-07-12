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

func (t *SimpleToken) Parse(s TokenStream, opts ParseOptions) error {
	tok := s.Next()
	if err := opts.matchToken(tok); err != nil {
		return err
	}
	t.tokType = tok.Type()
	t.tokValue = tok.Value()
	return nil
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
