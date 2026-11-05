package grammar

import (
	"errors"
	"testing"
)

type ptNum struct {
	Tok `tok:"number"`
}

type ptSum struct {
	Seq
	Lhs  ptNum
	Plus Match `tok:"op,+"`
	Rhs  ptNum
}

type ptExpr struct {
	OneOf
	Sum *ptSum
	Num *ptNum
}

type ptSpaced struct {
	Seq `drop:"space"`
	A   ptNum
	B   ptNum
}

var parseTestTokenise = SimpleTokeniser([]TokenDef{
	{Ptn: `\s+`},
	{Name: "op", Ptn: `[+]`},
	{Name: "number", Ptn: `[0-9]+`},
})

func TestExhaust(t *testing.T) {
	tests := []struct {
		src       string
		exhaust   bool
		wantErr   bool
		wantIs    error  // expected Err of the returned error, if wantErr
		wantCol   int    // expected column of the returned error, if wantErr
		wantValue string // expected token value of the returned error, if wantErr
	}{
		{src: `1 + 2`, exhaust: true},
		{src: `1 + 2 3`, exhaust: false},
		// Leftover input, and the parser never got past it.
		{src: `1 + 2 3`, exhaust: true, wantErr: true, wantIs: ErrNotExhausted, wantCol: 7, wantValue: "3"},
		// Leftover `+`, but the parser got further looking for its right operand.
		{src: `1 +`, exhaust: true, wantErr: true, wantCol: 4, wantValue: "EOF"},
		// A plain failure is unaffected.
		{src: `+`, exhaust: true, wantErr: true, wantCol: 1, wantValue: "+"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			toks, _ := parseTestTokenise(tt.src)
			var e ptExpr
			var opts []ParseOption
			if tt.exhaust {
				opts = append(opts, Exhaust)
			}
			err := Parse(&e, toks, opts...)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			if tt.wantIs != nil && !errors.Is(err.Err, tt.wantIs) {
				t.Errorf("Err = %v, want %v", err.Err, tt.wantIs)
			}
			if err.Token.Position().Column != tt.wantCol || err.Token.Value() != tt.wantValue {
				t.Errorf("error at %v on %q, want column %d on %q: %s", err.Token.Position(), err.Token.Value(), tt.wantCol, tt.wantValue, err)
			}
		})
	}
}

func TestExhaustSkipsDroppedTokens(t *testing.T) {
	toks := NewSimpleTokenStream([]Token{
		SimpleToken{TokType: "number", TokValue: "1"},
		SimpleToken{TokType: "space", TokValue: " "},
		SimpleToken{TokType: "number", TokValue: "2"},
		SimpleToken{TokType: "space", TokValue: " "},
	})
	var r ptSpaced
	if err := Parse(&r, toks, Exhaust); err != nil {
		t.Fatalf("trailing dropped token counted as leftover: %s", err)
	}
}

// A custom rule reporting a failure that is not a token mismatch.
type ptEven struct{ Value string }

func (e *ptEven) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	pos := s.Save()
	tok, err := opts.MatchNextToken(s)
	if err != nil {
		return err
	}
	if tok.Value()[len(tok.Value())-1]%2 != 0 {
		return s.Fail(pos, tok, errors.New("odd number"))
	}
	e.Value = tok.Value()
	return nil
}

type ptEvenOrOp struct {
	OneOf
	Even *ptEven `tok:"number"`
	Op   *Match  `tok:"op"`
}

func TestFail(t *testing.T) {
	toks, _ := parseTestTokenise(`4`)
	var r ptEvenOrOp
	if err := Parse(&r, toks); err != nil || r.Even == nil || r.Even.Value != "4" {
		t.Errorf("err=%v r=%+v", err, r)
	}
	toks, _ = parseTestTokenise(`3`)
	r = ptEvenOrOp{}
	err := Parse(&r, toks)
	// Both reasons for failing at the same position are reported.
	if want := `Ln 1, Col 1: number with value "3": expected token with type op; odd number`; err == nil || err.Error() != want {
		t.Errorf("got %v, want %s", err, want)
	}
}

func TestFailureRecord(t *testing.T) {
	toks, _ := parseTestTokenise(`1 + 2`)
	s := &ParserState{TokenStream: toks, memo: memoTable{}}
	num := TokenOptions{TokenParseOptions: []TokenParseOptions{{TokenType: "number"}}}
	op := TokenOptions{TokenParseOptions: []TokenParseOptions{{TokenType: "op"}}}
	plus := TokenOptions{TokenParseOptions: []TokenParseOptions{{TokenType: "op", TokenValue: "+"}}}
	// Failures are recorded in one shared error, which describes the
	// furthest of them with all the alternatives tried there.
	_, err1 := op.MatchNextToken(s) // fails at 0
	s.Restore(1)
	_, err2 := num.MatchNextToken(s) // fails at 1
	s.Restore(1)
	_, err3 := plus.MatchNextToken(s) // succeeds
	s.Restore(1)
	_, err4 := op.MatchNextToken(s) // succeeds
	s.Restore(0)
	_, err5 := op.MatchNextToken(s) // fails at 0 again, not recorded
	if err1 == nil || err1 != err2 || err3 != nil || err4 != nil || err5 != err1 {
		t.Fatalf("unexpected errors: %v %v %v %v %v", err1, err2, err3, err4, err5)
	}
	if err1.Pos != 1 || err1.Token.Value() != "+" || len(err1.TokenParseOptions) != 1 || err1.TokenParseOptions[0].TokenType != "number" {
		t.Errorf("record: %+v", *err1)
	}
	// A further failure replaces the record; one at the same position adds
	// its alternatives, once.
	s.Restore(2)
	op.MatchNextToken(s)
	s.Restore(2)
	op.MatchNextToken(s)
	s.Restore(2)
	plus.MatchNextToken(s)
	if err1.Pos != 2 || len(err1.TokenParseOptions) != 2 {
		t.Errorf("record: %+v", *err1)
	}
	// An error allocated by a custom rule is folded into the record.
	custom := &ParseError{Err: errors.New("custom"), Token: SimpleToken{}, Pos: 2}
	if folded := s.fold(custom); folded != err1 || err1.Err != custom.Err || len(err1.TokenParseOptions) != 2 {
		t.Errorf("fold: %+v", *err1)
	}
	// The result is a copy.
	if res := s.result(); res == err1 || res.Pos != 2 || res.Err != custom.Err {
		t.Errorf("result: %+v", *res)
	}
}
