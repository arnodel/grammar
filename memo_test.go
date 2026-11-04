package grammar

import (
	"strings"
	"testing"
)

// A user-written token rule that honours the tok tag via opts, like
// SimpleToken.  It does not implement Memoisable, so it is parsed afresh
// every time and the differing options cannot be confused.
type mtWord struct{ Text string }

func (w *mtWord) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	tok, err := opts.MatchNextToken(s)
	if err != nil {
		return err
	}
	w.Text = strings.ToUpper(tok.Value())
	return nil
}

// The same, but memoised with the options in the key.
type mtMemoWord struct{ Text string }

var mtMemoWordParses int

func (w *mtMemoWord) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	mtMemoWordParses++
	tok, err := opts.MatchNextToken(s)
	if err != nil {
		return err
	}
	w.Text = strings.ToUpper(tok.Value())
	return nil
}

func (*mtMemoWord) MemoKey(opts TokenOptions) (any, bool) {
	return opts.Key(), true
}

// Stmt ::= "let" ident | ident
type mtStmt struct {
	OneOf
	Let  *mtLet
	Call *mtCall
}

type mtLet struct {
	Seq
	Kw   mtWord `tok:"kw,let"`
	Name mtWord `tok:"ident"`
}

type mtCall struct {
	Seq
	Fn mtWord `tok:"ident"`
}

type mtMemoStmt struct {
	OneOf
	Let  *mtMemoLet
	Call *mtMemoCall
}

type mtMemoLet struct {
	Seq
	Kw   mtMemoWord `tok:"kw,let"`
	Name mtMemoWord `tok:"ident"`
}

type mtMemoCall struct {
	Seq
	Fn mtMemoWord `tok:"ident"`
}

var memoTestTokenise = SimpleTokeniser([]TokenDef{
	{Ptn: `\s+`},
	{Name: "kw", Ptn: `let`},
	{Name: "ident", Ptn: `[a-z]+`},
})

func TestUnmemoisedCustomRuleHonoursOptions(t *testing.T) {
	toks, _ := memoTestTokenise(`foo`)
	var st mtStmt
	if err := Parse(&st, toks, Exhaust); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if st.Call == nil || st.Call.Fn.Text != "FOO" {
		t.Errorf("expected a call to FOO, got let=%v call=%v", st.Let, st.Call)
	}
	toks, _ = memoTestTokenise(`let x`)
	st = mtStmt{}
	if err := Parse(&st, toks, Exhaust); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if st.Let == nil || st.Let.Name.Text != "X" {
		t.Errorf("expected let X, got let=%v call=%v", st.Let, st.Call)
	}
}

func TestMemoisedCustomRuleKeyedOnOptions(t *testing.T) {
	toks, _ := memoTestTokenise(`foo`)
	var st mtMemoStmt
	if err := Parse(&st, toks, Exhaust); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if st.Call == nil || st.Call.Fn.Text != "FOO" {
		t.Errorf("expected a call to FOO, got let=%v call=%v", st.Let, st.Call)
	}
}

func TestMemoisedCustomRuleIsCached(t *testing.T) {
	// Two alternatives starting with the same memoised rule and options at
	// the same position: the second must come from the cache.
	type alt1 struct {
		Seq
		A mtMemoWord `tok:"ident"`
		B mtMemoWord `tok:"kw"`
	}
	type alt2 struct {
		Seq
		A mtMemoWord `tok:"ident"`
	}
	type top struct {
		OneOf
		X *alt1
		Y *alt2
	}
	toks, _ := memoTestTokenise(`foo`)
	var r top
	mtMemoWordParses = 0
	if err := Parse(&r, toks, Exhaust); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if r.Y == nil {
		t.Fatal("expected the second alternative to match")
	}
	// alt1.A (ident at 0), alt1.B (kw at 1), then alt2.A (ident at 0) cached.
	if mtMemoWordParses != 2 {
		t.Errorf("Parse called %d times, want 2", mtMemoWordParses)
	}
}

func TestTokenOptionsKey(t *testing.T) {
	a := tokenOptionsFromTagValue("kw,let")
	b := tokenOptionsFromTagValue("kw,let")
	c := tokenOptionsFromTagValue("ident")
	d := tokenOptionsFromTagValue("kw,let|ident")
	e := tokenOptionsFromTagValue("kw*,let|ident")
	if a.Key() != b.Key() {
		t.Error("equal options must have equal keys")
	}
	for _, other := range []TokenOptions{c, d, e} {
		if a.Key() == other.Key() {
			t.Errorf("different options %v and %v have the same key", a, other)
		}
	}
	if d.Key() == e.Key() {
		t.Error("DoNotConsume must be part of the key")
	}
	if (TokenOptions{}).Key() != nil {
		t.Error("empty options should have a nil key")
	}
}
