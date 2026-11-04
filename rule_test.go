package grammar

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

var ruleTestTokenise = SimpleTokeniser([]TokenDef{
	{Ptn: `\s+`},
	{Name: "op", Ptn: `[\[\],;]`},
	{Name: "number", Ptn: `[0-9]+`},
})

type rtNum struct {
	Tok `tok:"number"`
}

type rtListNone struct {
	Seq
	Open  Match   `tok:"op,["`
	Items []rtNum `sep:"op,,"`
	Close Match   `tok:"op,]"`
}

type rtListOptional struct {
	Seq
	Open  Match   `tok:"op,["`
	Items []rtNum `sep:"op,," trail:"optional"`
	Close Match   `tok:"op,]"`
}

type rtListRequired struct {
	Seq
	Open  Match   `tok:"op,["`
	Items []rtNum `sep:"op,;" trail:"required"`
	Close Match   `tok:"op,]"`
}

func TestSeparatorTrailModes(t *testing.T) {
	tests := []struct {
		src   string
		rule  func() (any, func() int)
		want  int // number of items, -1 for a parse error
		label string
	}{
		{`[]`, newNone, 0, "none"},
		{`[1]`, newNone, 1, "none"},
		{`[1, 2]`, newNone, 2, "none"},
		{`[1, 2,]`, newNone, -1, "none"},
		{`[,]`, newNone, -1, "none"},
		{`[1 2]`, newNone, -1, "none"},

		{`[]`, newOptional, 0, "optional"},
		{`[1]`, newOptional, 1, "optional"},
		{`[1,]`, newOptional, 1, "optional"},
		{`[1, 2]`, newOptional, 2, "optional"},
		{`[1, 2,]`, newOptional, 2, "optional"},
		{`[1, 2,,]`, newOptional, -1, "optional"},
		{`[,]`, newOptional, -1, "optional"},

		{`[]`, newRequired, 0, "required"},
		{`[1;]`, newRequired, 1, "required"},
		{`[1; 2;]`, newRequired, 2, "required"},
		{`[1]`, newRequired, -1, "required"},
		{`[1; 2]`, newRequired, -1, "required"},
		{`[;]`, newRequired, -1, "required"},
	}
	for _, tt := range tests {
		t.Run(tt.label+" "+tt.src, func(t *testing.T) {
			toks, err := ruleTestTokenise(tt.src)
			if err != nil {
				t.Fatal(err)
			}
			dest, count := tt.rule()
			parseErr := Parse(dest, toks)
			switch {
			case tt.want < 0 && parseErr == nil:
				t.Errorf("expected an error, parsed %d items", count())
			case tt.want >= 0 && parseErr != nil:
				t.Errorf("unexpected error: %s", parseErr)
			case tt.want >= 0 && count() != tt.want:
				t.Errorf("got %d items, want %d", count(), tt.want)
			}
		})
	}
}

func newNone() (any, func() int) {
	r := new(rtListNone)
	return r, func() int { return len(r.Items) }
}

func newOptional() (any, func() int) {
	r := new(rtListOptional)
	return r, func() int { return len(r.Items) }
}

func newRequired() (any, func() int) {
	r := new(rtListRequired)
	return r, func() int { return len(r.Items) }
}

// When Max is reached the separator after the last item must not be consumed.
type rtListMax struct {
	Seq
	Open  Match   `tok:"op,["`
	Items []rtNum `sep:"op,," size:"-2"`
	Comma Match   `tok:"op,,"`
	Last  rtNum
	Close Match `tok:"op,]"`
}

func TestSeparatorNotConsumedAtMax(t *testing.T) {
	toks, _ := ruleTestTokenise(`[1, 2, 3]`)
	var r rtListMax
	if err := Parse(&r, toks); err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 2 || r.Last.TokValue != "3" {
		t.Errorf("got items %v, last %q", r.Items, r.Last.TokValue)
	}
}

// OneOf repeated fields: a fixed size must be honoured and separators used.
type rtOneOfFixed struct {
	OneOf
	Pair []rtNum `size:"2"`
}

type rtOneOfSep struct {
	OneOf
	Nums []rtNum `sep:"op,,"`
}

func TestOneOfRepeatedField(t *testing.T) {
	toks, _ := ruleTestTokenise(`1 2`)
	var fixed rtOneOfFixed
	if err := Parse(&fixed, toks); err != nil {
		t.Fatal(err)
	}
	if len(fixed.Pair) != 2 {
		t.Errorf("fixed size: got %d items, want 2", len(fixed.Pair))
	}

	toks, _ = ruleTestTokenise(`1`)
	var short rtOneOfFixed
	if err := Parse(&short, toks); err == nil {
		t.Error("fixed size: expected an error for a single item")
	}

	toks, _ = ruleTestTokenise(`1, 2, 3`)
	var sep rtOneOfSep
	if err := Parse(&sep, toks); err != nil {
		t.Fatal(err)
	}
	if len(sep.Nums) != 3 {
		t.Errorf("with separator: got %d items, want 3", len(sep.Nums))
	}
}

func TestTrailTagValidation(t *testing.T) {
	type noSep struct {
		Seq
		Items []rtNum `trail:"optional"`
	}
	type badValue struct {
		Seq
		Items []rtNum `sep:"op,," trail:"sometimes"`
	}
	for _, tp := range []any{noSep{}, badValue{}} {
		if _, err := calcRuleDef(reflectTypeOf(tp)); err == nil {
			t.Errorf("%T: expected a rule definition error", tp)
		}
	}
}

func TestMergeKeepsErr(t *testing.T) {
	tok := SimpleToken{TokType: "op", TokValue: "]"}
	e1 := &ParseError{Err: errors.New("empty match for rule X"), Token: tok, Pos: 3}
	e2 := &ParseError{Token: tok, Pos: 3, TokenParseOptions: []TokenParseOptions{{TokenType: "number"}}}
	for _, merged := range []*ParseError{e1.Merge(e2), e2.Merge(e1)} {
		if merged.Err == nil {
			t.Fatal("Err dropped by Merge")
		}
		msg := merged.Error()
		if !strings.Contains(msg, "empty match for rule X") || !strings.Contains(msg, "type number") {
			t.Errorf("message lacks one of the merged parts: %s", msg)
		}
	}
	// The internal recursion marker must not win over a real error.
	rec := &ParseError{Err: errRecursion, Token: tok, Pos: 3}
	if merged := rec.Merge(e1); merged.Err != e1.Err {
		t.Errorf("recursion marker kept over real error: %v", merged.Err)
	}
	if merged := rec.Merge(e2); merged.Err != nil {
		t.Errorf("recursion marker kept when options are available: %v", merged.Err)
	}
}

func TestEOFPosition(t *testing.T) {
	toks, _ := ruleTestTokenise("[1,\n 2 ")
	for i := 0; i < 4; i++ {
		toks.Next()
	}
	eof := toks.Next()
	if eof.Type() != EOF.Type() {
		t.Fatalf("expected EOF, got %v", eof)
	}
	if got, want := eof.Position(), (Position{Line: 2, Column: 4}); got != want {
		t.Errorf("tokeniser EOF position: got %v, want %v", got, want)
	}

	manual := NewSimpleTokenStream([]Token{SimpleToken{TokType: "number", TokValue: "12", TokPos: Position{Line: 1, Column: 1}}})
	manual.Next()
	if got, want := manual.Next().Position(), (Position{Line: 1, Column: 3}); got != want {
		t.Errorf("manual stream EOF position: got %v, want %v", got, want)
	}

	// And it shows in error messages.
	var r rtListNone
	toks, _ = ruleTestTokenise(`[1, 2`)
	err := Parse(&r, toks)
	if err == nil || !strings.HasPrefix(err.Error(), "Ln 1, Col 6: EOF") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func reflectTypeOf(v any) reflect.Type { return reflect.TypeOf(v) }
