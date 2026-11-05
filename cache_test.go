package grammar

import (
	"bytes"
	"sync"
	"testing"
)

// A grammar used by no other test, so that its definitions are not cached
// when the test starts.
type ccList struct {
	Seq
	Open  Match    `tok:"op,["`
	Items []ccItem `sep:"op,,"`
	Close Match    `tok:"op,]"`
}

type ccItem struct {
	OneOf
	Num  *ccNum
	List *ccList
	Word *ccWord
}

type ccNum struct {
	Tok `tok:"number"`
}

type ccWord struct {
	OneOf2[ccUpper, ccLower]
}

type ccUpper struct {
	Tok `tok:"upper"`
}

type ccLower struct {
	Tok `tok:"lower"`
}

var ccTokenise = SimpleTokeniser([]TokenDef{
	{Ptn: `\s+`},
	{Name: "op", Ptn: `[\[\],]`},
	{Name: "number", Ptn: `[0-9]+`},
	{Name: "upper", Ptn: `[A-Z]+`},
	{Name: "lower", Ptn: `[a-z]+`},
})

// TestConcurrentFirstUse parses with a grammar from several goroutines at
// once, before any of its definitions are cached (run with -race).
func TestConcurrentFirstUse(t *testing.T) {
	const src = `[1, [2, x], Y, [[3]], z]`
	var want string
	{
		toks, _ := ccTokenise(src)
		var l ccList
		if err := Parse(&l, toks, Exhaust); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		PrettyWrite(&buf, &l)
		want = buf.String()
	}
	// The grammar is now cached; concurrency on first use is tested with a
	// second grammar below, this part checks concurrent use of the cache.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				toks, _ := ccTokenise(src)
				var l ccList
				if err := Parse(&l, toks, Exhaust); err != nil {
					t.Error(err)
					return
				}
				var buf bytes.Buffer
				PrettyWrite(&buf, &l)
				if buf.String() != want {
					t.Errorf("got\n%s\nwant\n%s", buf.String(), want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

type cc2Prog struct {
	Seq
	Stmts []cc2Stmt `sep:"op,,"`
}

type cc2Stmt struct {
	OneOf
	List *ccList // shares rules with the grammar above
	Pair *cc2Pair
}

type cc2Pair struct {
	Seq
	Open  Match `tok:"op,["`
	Key   ccWord
	Value cc2Stmt
	Close Match `tok:"op,]"`
}

func TestConcurrentFirstUseUncached(t *testing.T) {
	const src = `[a [1]], [B [c [2, 3]]], [4]`
	var wg sync.WaitGroup
	results := make([]string, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			toks, _ := ccTokenise(src)
			var p cc2Prog
			if err := Parse(&p, toks, Exhaust); err != nil {
				t.Error(err)
				return
			}
			var buf bytes.Buffer
			PrettyWrite(&buf, &p)
			results[i] = buf.String()
		}()
	}
	wg.Wait()
	for i, r := range results {
		if r != results[0] {
			t.Errorf("goroutine %d got a different result", i)
		}
	}
}
