package sql

import (
	"os"
	"strings"
	"testing"

	"github.com/arnodel/grammar"
)

// corpus returns the statements of testdata/queries.sql, which are separated
// by lines holding only ";".
func corpus(t testing.TB) []string {
	src, err := os.ReadFile("testdata/queries.sql")
	if err != nil {
		t.Fatal(err)
	}
	var stmts []string
	for _, stmt := range strings.Split(string(src), "\n;\n") {
		if strings.TrimSpace(stmt) != "" {
			stmts = append(stmts, strings.TrimSpace(stmt))
		}
	}
	return stmts
}

func parseStmt(t *testing.T, src string) *Stmt {
	toks, err := Tokenise(src)
	if err != nil {
		t.Fatalf("%s: %s", src, err)
	}
	var stmt Stmt
	if err := grammar.Parse(&stmt, toks, grammar.Exhaust); err != nil {
		t.Fatalf("%s:\n  %s", src, err)
	}
	return &stmt
}

func TestCorpus(t *testing.T) {
	stmts := corpus(t)
	for _, src := range stmts {
		parseStmt(t, src)
	}
	// All together as a script, with a trailing ";".
	toks, err := Tokenise(strings.Join(stmts, "\n;\n") + "\n;")
	if err != nil {
		t.Fatal(err)
	}
	var script Script
	if err := grammar.Parse(&script, toks, grammar.Exhaust); err != nil {
		t.Fatal(err)
	}
	if len(script.Stmts) != len(stmts) {
		t.Errorf("script has %d statements, want %d", len(script.Stmts), len(stmts))
	}
}

// TestErrors checks where errors are reported.  The full list of expected
// tokens is only pinned where it is short: an expression can be followed by
// some forty different tokens.
func TestErrors(t *testing.T) {
	tests := []struct {
		src  string
		want string // the start of the message
	}{
		{`SELECT FROM t`, `Ln 1, Col 8: kw with value "FROM": expected token with type ident or ckw or qident or number or string or blob or param, or value "DISTINCT" or "ALL" or "*" or "NOT"`},
		{`SELECT * FROM t WHERE`, `Ln 1, Col 22: EOF with value "EOF": expected token with type number`},
		{`SELECT * FROM t WHERE a = `, `Ln 1, Col 27: EOF with value "EOF": expected token with type number`},
		{`SELECT a b c FROM t`, `Ln 1, Col 12: ident with value "c": expected token with value "," or "FROM" or "WHERE" or "GROUP" or "WINDOW" or "UNION" or "INTERSECT" or "EXCEPT" or "ORDER" or "LIMIT"; expected end of input`},
		{`SELECT * FROM t JOIN`, `Ln 1, Col 21: EOF with value "EOF": expected token with type ident or ckw or qident, or value "("`},
		{`INSERT INTO t (a, b) VALUES (1, 2) ON CONFLICT DO`, `Ln 1, Col 50: EOF with value "EOF": expected token with value "NOTHING" or "UPDATE"`},
		{`SELECT CASE WHEN a THEN 1`, `Ln 1, Col 26: EOF with value "EOF": expected token with type concat or bitor, or value "COLLATE"`},
		{`SELECT 1 FROM t ORDER BY`, `Ln 1, Col 25: EOF with value "EOF": expected token with type number`},
		{`SELECT * FROM t WHERE x = 1 2`, `Ln 1, Col 29: number with value "2": expected token with type concat or bitor, or value "COLLATE"`},
	}
	for _, tt := range tests {
		toks, err := Tokenise(tt.src)
		if err != nil {
			t.Fatal(err)
		}
		var stmt Stmt
		perr := grammar.Parse(&stmt, toks, grammar.Exhaust)
		if perr == nil {
			t.Errorf("%s: expected an error", tt.src)
			continue
		}
		if !strings.HasPrefix(perr.Error(), tt.want) {
			t.Errorf("%s:\n got: %s\nwant: %s...", tt.src, perr, tt.want)
		}
	}
}

func BenchmarkParse(b *testing.B) {
	toks, err := Tokenise(strings.Join(corpus(b), "\n;\n"))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toks.Restore(0)
		var script Script
		if err := grammar.Parse(&script, toks, grammar.Exhaust); err != nil {
			b.Fatal(err)
		}
	}
}
