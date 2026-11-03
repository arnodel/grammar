package nest

import (
	"strings"
	"testing"
	"time"

	"github.com/arnodel/grammar"
)

// Same shape as jsonstream's filter grammar: a call can start a comparison
// or a test, and an argument is tried as a call and then as a logical expr.
type Logical struct {
	grammar.OneOf
	Cmp  *Cmp
	Test *Call
}

type Cmp struct {
	grammar.Seq
	Left  Call
	Op    grammar.Match `tok:"op,=="`
	Right Call
}

type Call struct {
	grammar.Seq
	Name  grammar.Match `tok:"call"`
	Arg   *Arg
	Close grammar.Match `tok:"bkt,)"`
}

type Arg struct {
	grammar.OneOf
	Num     *grammar.Tok `tok:"number"`
	Call    *Call
	Logical *Logical
}

var tokenise = grammar.SimpleTokeniser([]grammar.TokenDef{
	{Ptn: `\s+`},
	{Name: "call", Ptn: `[a-z]+\(`},
	{Name: "bkt", Ptn: `\)`},
	{Name: "op", Ptn: `==`},
	{Name: "number", Ptn: `[0-9]+`},
})

func TestNesting(t *testing.T) {
	for _, n := range []int{4, 8, 12, 16, 24} {
		toks, _ := tokenise(strings.Repeat("a(", n))
		var l Logical
		start := time.Now()
		err := grammar.Parse(&l, toks)
		d := time.Since(start)
		opts := 0
		if err != nil {
			opts = len(err.TokenParseOptions)
		}
		t.Logf("n=%2d %12v error options=%d", n, d, opts)
		if d > 5*time.Second {
			break
		}
	}
}
