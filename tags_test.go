package grammar

import (
	"reflect"
	"strings"
	"testing"
)

var tagTestTokenise = SimpleTokeniser([]TokenDef{
	{Name: "space", Ptn: `\s+`}, // kept, so that drop can be tested
	{Name: "op", Ptn: `[\[\](),]`},
	{Name: "number", Ptn: `[0-9]+`},
	{Name: "word", Ptn: `[a-z]+`},
})

type ttNum struct {
	Tok `tok:"number"`
}

// --- drop ---

type ttDropSeq struct {
	Seq   `drop:"space"`
	Open  Match `tok:"op,("`
	Items []ttNum
	Close Match `tok:"op,)"`
}

type ttNoDropSeq struct {
	Seq
	Open  Match `tok:"op,("`
	Items []ttNum
	Close Match `tok:"op,)"`
}

type ttDropOneOf struct {
	OneOf `drop:"space"`
	Num   *ttNum
	Word  *Tok `tok:"word"`
}

func TestDropTag(t *testing.T) {
	toks, _ := tagTestTokenise(` ( 1 2 ) `)
	var r ttDropSeq
	if err := Parse(&r, toks, Exhaust); err != nil {
		t.Fatalf("drop: unexpected error: %s", err)
	}
	if len(r.Items) != 2 {
		t.Errorf("drop: got %d items, want 2", len(r.Items))
	}

	// Without drop the spaces are in the way.
	toks, _ = tagTestTokenise(`( 1 )`)
	var n ttNoDropSeq
	if err := Parse(&n, toks); err == nil {
		t.Error("no drop: expected an error on the space token")
	}

	// Dropping happens before each field of the rule that has the tag, not
	// inside nested rules: Items of ttNoDropSeq is reached with the spaces
	// still there when nested in a dropping rule.
	type outer struct {
		Seq   `drop:"space"`
		Inner ttNoDropSeq
	}
	toks, _ = tagTestTokenise(` (1)`)
	var o outer
	if err := Parse(&o, toks, Exhaust); err != nil {
		t.Errorf("nested: unexpected error: %s", err)
	}
	toks, _ = tagTestTokenise(` ( 1 )`)
	o = outer{}
	if err := Parse(&o, toks); err == nil {
		t.Error("nested: inner rule without drop should fail on inner spaces")
	}

	// OneOf drops before trying the alternatives.
	toks, _ = tagTestTokenise(`  abc`)
	var d ttDropOneOf
	if err := Parse(&d, toks, Exhaust); err != nil {
		t.Fatalf("oneof drop: unexpected error: %s", err)
	}
	if d.Word == nil || d.Word.TokValue != "abc" {
		t.Errorf("oneof drop: got %+v", d)
	}
}

// --- size ---

type ttSizeExact struct {
	Seq   `drop:"space"`
	Items []ttNum `size:"2"`
}

type ttSizeRange struct {
	Seq   `drop:"space"`
	Items []ttNum `size:"1-3"`
}

type ttSizeMin struct {
	Seq   `drop:"space"`
	Items []ttNum `size:"2-"`
}

type ttSizeMax struct {
	Seq   `drop:"space"`
	Items []ttNum `size:"-2"`
}

func TestSizeTag(t *testing.T) {
	tests := []struct {
		rule  func() (any, func() int)
		src   string
		want  int // items parsed by the rule, -1 for an error
		label string
	}{
		{newSized[ttSizeExact], `1`, -1, "exact 2"},
		{newSized[ttSizeExact], `1 2`, 2, "exact 2"},
		{newSized[ttSizeExact], `1 2 3`, 2, "exact 2"}, // 3 is left over
		{newSized[ttSizeRange], ``, -1, "1-3"},
		{newSized[ttSizeRange], `1`, 1, "1-3"},
		{newSized[ttSizeRange], `1 2 3`, 3, "1-3"},
		{newSized[ttSizeRange], `1 2 3 4`, 3, "1-3"},
		{newSized[ttSizeMin], `1`, -1, "2-"},
		{newSized[ttSizeMin], `1 2 3 4 5`, 5, "2-"},
		{newSized[ttSizeMax], ``, -1, "-2"}, // empty match for the Seq
		{newSized[ttSizeMax], `1`, 1, "-2"},
		{newSized[ttSizeMax], `1 2 3`, 2, "-2"},
	}
	for _, tt := range tests {
		t.Run(tt.label+" "+tt.src, func(t *testing.T) {
			toks, _ := tagTestTokenise(tt.src)
			dest, count := tt.rule()
			err := Parse(dest, toks)
			switch {
			case tt.want < 0 && err == nil:
				t.Errorf("expected an error, got %d items", count())
			case tt.want >= 0 && err != nil:
				t.Errorf("unexpected error: %s", err)
			case tt.want >= 0 && count() != tt.want:
				t.Errorf("got %d items, want %d", count(), tt.want)
			}
		})
	}
}

type sized interface {
	ttSizeExact | ttSizeRange | ttSizeMin | ttSizeMax
}

func newSized[T sized]() (any, func() int) {
	r := new(T)
	return r, func() int {
		switch v := any(r).(type) {
		case *ttSizeExact:
			return len(v.Items)
		case *ttSizeRange:
			return len(v.Items)
		case *ttSizeMin:
			return len(v.Items)
		case *ttSizeMax:
			return len(v.Items)
		}
		panic("unreachable")
	}
}

// --- lookahead with "*" ---

// A number followed by a bracket, where the bracket is checked but left for
// the next field to consume.
type ttLookahead struct {
	Seq
	Num   ttNum
	Ahead Match `tok:"op*,("`
	Open  Match `tok:"op,("`
}

// A list whose items stop at a "," without consuming it (negative lookahead
// is not available, but positive lookahead can guard a choice).
type ttGuarded struct {
	OneOf
	Call *ttCall
	Num  *ttNum
}

type ttCall struct {
	Seq
	Name  Tok   `tok:"word"`
	Guard Match `tok:"op*,("`
	Open  Match `tok:"op,("`
	Close Match `tok:"op,)"`
}

func TestNoConsumeTag(t *testing.T) {
	toks, _ := tagTestTokenise(`1(`)
	var r ttLookahead
	if err := Parse(&r, toks, Exhaust); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	toks, _ = tagTestTokenise(`1)`)
	r = ttLookahead{}
	err := Parse(&r, toks, Exhaust)
	if err == nil {
		t.Fatal("expected an error")
	}
	// The lookahead failure is reported on the token it looked at.
	if err.Token.Value() != ")" || !strings.Contains(err.Error(), `"("`) {
		t.Errorf("unexpected error: %s", err)
	}

	toks, _ = tagTestTokenise(`f()`)
	var g ttGuarded
	if err := Parse(&g, toks, Exhaust); err != nil || g.Call == nil {
		t.Errorf("guarded call: err=%v call=%v", err, g.Call)
	}
}

// --- error messages ---

type ttErrSeq struct {
	Seq
	Open  Match `tok:"op,("`
	Items []ttNum
	Close Match `tok:"op,)"`
}

type ttErrItems struct {
	Seq
	Items []ttNum `sep:"op,,"`
}

type ttErrDup struct {
	OneOf
	Word *Tok `tok:"word"`
	Any  *Tok `tok:"word|number"`
	Num  *ttNum
}

// The options recorded at the furthest position are reported once each.
func TestErrorOptionsUnique(t *testing.T) {
	toks, _ := tagTestTokenise(`,`)
	var r ttErrDup
	err := Parse(&r, toks)
	if err == nil {
		t.Fatal("expected an error")
	}
	want := []TokenParseOptions{{TokenType: "word"}, {TokenType: "number"}}
	if !reflect.DeepEqual(err.TokenParseOptions, want) {
		t.Errorf("got options %v, want %v", err.TokenParseOptions, want)
	}
}

func TestErrorMessages(t *testing.T) {
	tests := []struct {
		rule func() any
		src  string
		want string
	}{
		// Position, token and alternatives at the furthest failure.
		{func() any { return &ttErrSeq{} }, `(1 2 x`, `Ln 1, Col 6: word with value "x": expected token with type number, or value ")"`},
		{func() any { return &ttErrSeq{} }, `(1`, `Ln 1, Col 3: EOF with value "EOF": expected token with type number, or value ")"`},
		{func() any { return &ttErrSeq{} }, `x`, `Ln 1, Col 1: word with value "x": expected token with value "("`},
		// A separator that failed to match is an alternative too, even though
		// the rule succeeded without it.
		{func() any { return &ttErrItems{} }, `1 2`, `Ln 1, Col 3: number with value "2": expected token with value ","; expected end of input`},
	}
	for _, tt := range tests {
		toks, _ := SimpleTokeniser([]TokenDef{
			{Ptn: `\s+`},
			{Name: "op", Ptn: `[(),]`},
			{Name: "number", Ptn: `[0-9]+`},
			{Name: "word", Ptn: `[a-z]+`},
		})(tt.src)
		r := tt.rule()
		err := Parse(r, toks, Exhaust)
		if err == nil {
			t.Errorf("%q: expected an error", tt.src)
			continue
		}
		if err.Error() != tt.want {
			t.Errorf("%q:\n got %s\nwant %s", tt.src, err, tt.want)
		}
	}
}
