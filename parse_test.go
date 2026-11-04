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
