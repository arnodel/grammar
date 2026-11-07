package sjson

import (
	"errors"
	"testing"

	"github.com/arnodel/grammar"
)

// The root rule here embeds a OneOfN rather than a struct OneOf.  Parse with
// Exhaust looks the root rule's definition up to skip the tokens it drops,
// and used to crash on a root rule that has no definition (v0.2.0).
func TestExhaustWithOneOfNRoot(t *testing.T) {
	for _, tt := range []struct {
		src  string
		want error
	}{
		{`[1, 2]`, nil},
		{`[1, 2] 3`, grammar.ErrNotExhausted},
	} {
		toks, err := tokenise(tt.src)
		if err != nil {
			t.Fatal(err)
		}
		var json SJSON
		perr := grammar.Parse(&json, toks, grammar.Exhaust)
		switch {
		case perr == nil && tt.want != nil:
			t.Errorf("%s: parsed, want %s", tt.src, tt.want)
		case perr != nil && !errors.Is(perr.Err, tt.want):
			t.Errorf("%s: %s", tt.src, perr)
		}
	}
}
