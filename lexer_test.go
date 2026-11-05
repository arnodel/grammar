package grammar

import (
	"regexp"
	"strings"
	"testing"
)

func tokenTypesAndValues(t *testing.T, tokenise func(string) (*SimpleTokenStream, error), src string) string {
	t.Helper()
	toks, err := tokenise(src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %s", src, err)
	}
	var parts []string
	for _, tok := range toks.tokens {
		parts = append(parts, tok.Type()+":"+tok.Value())
	}
	return strings.Join(parts, " ")
}

func TestTokeniserLongestMatch(t *testing.T) {
	tokenise := SimpleTokeniser([]TokenDef{
		{Ptn: `\s+`},
		{Name: "kw", Ptn: `let`},
		{Name: "ident", Ptn: `[a-z]+`},
		{Name: "op", Ptn: `=`},
		{Name: "op", Ptn: `==`},
	})
	// Ties go to the earliest definition, so "let" is a keyword; longer
	// identifiers starting with a keyword are identifiers; the longer of
	// "=" and "==" wins whatever the order.
	if got, want := tokenTypesAndValues(t, tokenise, "let letter = x == y"), "kw:let ident:letter op:= ident:x op:== ident:y"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTokeniserPatternKinds(t *testing.T) {
	// One definition of each matching strategy, including a pattern with a
	// capturing group, which was once forbidden.
	tokenise := SimpleTokeniser([]TokenDef{
		{Ptn: `\s+`},                              // class+
		{Name: "bkt", Ptn: `[()]`},                // class
		{Name: "arrow", Ptn: `->`},                // literal
		{Name: "num", Ptn: `([0-9]+)(\.[0-9]+)?`}, // regexp, with groups
		{Name: "word", Ptn: `(?i)abc`},            // regexp (case folding is not a literal)
		{Name: "str", Scan: scanDoubleQuoted},     // scan only
	})
	if got, want := tokenTypesAndValues(t, tokenise, `(1.5 -> ABC) "a\"b" 42`), `bkt:( num:1.5 arrow:-> word:ABC bkt:) str:"a\"b" num:42`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTokeniserQuotedPattern(t *testing.T) {
	// A \Q without \E used to swallow the closing bracket of the anchor.
	tokenise := SimpleTokeniser([]TokenDef{
		{Name: "lit", Ptn: `\Qa.b`},
		{Name: "other", Ptn: `[a-z.)]+`},
	})
	if got, want := tokenTypesAndValues(t, tokenise, `a.bxa.b)`), `lit:a.b other:xa.b)`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// scanDoubleQuoted scans a double-quoted string with backslash escapes.
func scanDoubleQuoted(s string) int {
	if len(s) == 0 || s[0] != '"' {
		return 0
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return 0
}

func TestTokeniserScanWithPattern(t *testing.T) {
	// The pattern decides whether Scan is tried, Scan decides the length.
	tokenise := SimpleTokeniser([]TokenDef{
		{Ptn: `\s+`},
		{Name: "comment", Ptn: `/\*`, Scan: scanNestedComment},
		{Name: "op", Ptn: `[/*]`},
		{Name: "word", Ptn: `[a-z]+`},
	})
	if got, want := tokenTypesAndValues(t, tokenise, `a /* x /* y */ z */ / b`), `word:a comment:/* x /* y */ z */ op:/ word:b`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// An unterminated comment: Scan returns 0, "/" matches instead.
	if got, want := tokenTypesAndValues(t, tokenise, `/* x`), `op:/ op:* word:x`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func scanNestedComment(s string) int {
	depth := 0
	for i := 0; i+1 < len(s); i++ {
		switch s[i : i+2] {
		case "/*":
			depth++
			i++
		case "*/":
			depth--
			i++
			if depth == 0 {
				return i + 1
			}
		}
	}
	return 0
}

func TestTokeniserModes(t *testing.T) {
	tokenise := SimpleTokeniser([]TokenDef{
		{Ptn: `\s+`, Mode: "code"},
		{Name: "word", Ptn: `[a-z]+`, Mode: "code"},
		{Name: "open", Ptn: `"`, Mode: "code", PushMode: "str"},
		{Name: "end", Ptn: `\}`, Mode: "code", PopMode: true},
		{Name: "text", Ptn: `[^"{]+`, Mode: "str"},
		{Name: "close", Ptn: `"`, Mode: "str", PopMode: true},
		{Name: "interp", Ptn: `\{`, Mode: "str", PushMode: "code"},
	})
	if got, want := tokenTypesAndValues(t, tokenise, `a "x {b "y"} z" c`), `word:a open:" text:x  interp:{ word:b open:" text:y close:" end:} text: z close:" word:c`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := tokenise(`a }`); err == nil || err.Error() != "no mode to pop" {
		t.Errorf("expected a pop error, got %v", err)
	}
}

func TestTokeniserErrors(t *testing.T) {
	tokenise := SimpleTokeniser([]TokenDef{
		{Ptn: `\s+`},
		{Name: "word", Ptn: `[a-z]*`}, // can match the empty string, which never counts
	})
	_, err := tokenise("ab\n  12")
	if err == nil || err.Error() != "invalid token starting at Ln 2, Col 3" {
		t.Errorf("unexpected error: %v", err)
	}
	for _, defs := range [][]TokenDef{
		{},
		{{Name: "x"}},
		{{Name: "x", Ptn: `(`}},
		{{Name: "x", Ptn: `a`, PushMode: "nope"}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("expected a panic for %v", defs)
				}
			}()
			SimpleTokeniser(defs)
		}()
	}
}

func TestTokeniserUnicode(t *testing.T) {
	tokenise := SimpleTokeniser([]TokenDef{
		{Ptn: `\s+`},
		{Name: "word", Ptn: `\pL+`},
		{Name: "arrow", Ptn: `→`},
		{Name: "num", Ptn: `[0-9]+`},
	})
	toks, err := tokenise("héllo → 42\nwörld")
	if err != nil {
		t.Fatal(err)
	}
	want := []SimpleToken{
		{"word", "héllo", Position{1, 1}},
		{"arrow", "→", Position{1, 7}},
		{"num", "42", Position{1, 9}},
		{"word", "wörld", Position{2, 1}},
	}
	for i, w := range want {
		got := toks.tokens[i]
		if got.Type() != w.TokType || got.Value() != w.TokValue || got.Position() != w.TokPos {
			t.Errorf("token %d: got %s %q %v, want %s %q %v", i, got.Type(), got.Value(), got.Position(), w.TokType, w.TokValue, w.TokPos)
		}
	}
	if eof := toks.eof.Position(); eof != (Position{2, 6}) {
		t.Errorf("EOF at %v", eof)
	}
}

func TestFirstBytes(t *testing.T) {
	tests := []struct {
		ptn      string
		in, out  string
		nullable bool
	}{
		{`abc`, "a", "bc", false},
		{`(?i)abc`, "aA", "b", false},
		{`[a-c]+`, "abc", "d", false},
		{`x?y`, "xy", "z", false},
		{`x*`, "x", "y", true},
		{`\bfoo`, "f", "o", false},
		{`a|bc|(d)`, "abd", "c", false},
		{`é`, "\xc3", "\xc3\xa9"[1:], false},
		{`.`, "a\xff", "", false},
		{`\pL`, "a\xc3", "1", false},
		{`^`, "", "a", true},
		// Invalid UTF-8 bytes decode to U+FFFD, which these match.
		{`[^a]`, "b\x80\xff", "a", false},
		{`\x{FFFD}`, "\x80\xff", "a", false},
		{`[é-ÿ]`, "\xc3", "\xc2\xc4", false},
	}
	for _, tt := range tests {
		tm, err := newTokenMatcher(TokenDef{Ptn: tt.ptn})
		if err != nil {
			t.Fatalf("%q: %s", tt.ptn, err)
		}
		for _, c := range []byte(tt.in) {
			if !tm.first.has(c) {
				t.Errorf("%q: first set should contain %q", tt.ptn, c)
			}
		}
		for _, c := range []byte(tt.out) {
			if tm.first.has(c) {
				t.Errorf("%q: first set should not contain %q", tt.ptn, c)
			}
		}
	}
}

// FuzzTokenMatcher checks a token matcher (first-byte filter, then fast path
// or regexp) against the regexp engine alone.
func FuzzTokenMatcher(f *testing.F) {
	for _, ptn := range []string{
		`\s+`, `[()]`, `->`, `([0-9]+)(\.[0-9]+)?`, `(?i)abc`, `let`, `[a-z]+`,
		`"[^"]*"`, `-?[0-9]+(?:\.[0-9]+)?`, `true|false`, `[a-zA-Z_][a-zA-Z0-9_-]*`,
		`\pL+`, `é`, `.`, `x*`, `\bfoo`, `a|bc|(d)`, `^`, `[^a]`, `\x{1F600}`,
		`(?i)é`, `[a-z]*`, `(?s).+`, `a{2,3}`, `[\x00-\x7f]+`, `ß`, `(?i)ß`, `[[:alpha:]]+`,
	} {
		for _, in := range []string{"", "a", "let", "letter", "  x", "1.5", "é", "\xff", "ABC", "aaaa", "\x00"} {
			f.Add(ptn, in)
		}
	}
	f.Fuzz(func(t *testing.T, ptn, in string) {
		if in == "" || ptn == "" {
			return
		}
		re, err := regexp.Compile(ptn)
		if err != nil {
			return
		}
		tm, err := newTokenMatcher(TokenDef{Ptn: ptn})
		if err != nil {
			t.Fatalf("regexp compiles but matcher does not: %s", err)
		}
		// The leftmost match starting at 0, if any, is the anchored match.
		want := 0
		if loc := re.FindStringIndex(in); loc != nil && loc[0] == 0 {
			want = loc[1]
		}
		got := 0
		if tm.first.has(in[0]) {
			got = tm.match(in)
		}
		if got != want {
			t.Errorf("pattern %q, input %q: got %d, want %d (kind %d, first has %v)", ptn, in, got, want, tm.kind, tm.first.has(in[0]))
		}
	})
}
