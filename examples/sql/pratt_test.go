package sql

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arnodel/grammar"
)

// An operator-precedence (Pratt) parser for a subset of the expression
// grammar (binary operators, NOT and unary minus), as a custom rule, to
// measure what the grammar-based precedence costs against hand-written code.

type prattExpr struct{ Node any }

type prattBin struct {
	Op   string
	L, R any
}

type prattUn struct {
	Op string
	X  any
}

var prattPrec = map[string]int{"OR": 1, "AND": 2, "=": 4, "<": 5, ">": 5, "+": 7, "-": 7, "*": 8, "/": 8, "||": 9}

func (e *prattExpr) Parse(_ any, s *grammar.ParserState, _ grammar.TokenOptions) *grammar.ParseError {
	node, err := parsePratt(s, 0)
	e.Node = node
	return err
}

func parsePratt(s *grammar.ParserState, minPrec int) (any, *grammar.ParseError) {
	var left any
	tok := s.Peek()
	switch {
	case tok.Type() == "kw" && tok.Value() == "NOT":
		s.Next()
		x, err := parsePratt(s, 3)
		if err != nil {
			return nil, err
		}
		left = &prattUn{"NOT", x}
	case tok.Type() == "op" && tok.Value() == "-":
		s.Next()
		x, err := parsePratt(s, 10)
		if err != nil {
			return nil, err
		}
		left = &prattUn{"-", x}
	default:
		a := new(Atom)
		if err := grammar.ParseWithOptions(a, s, grammar.TokenOptions{}); err != nil {
			return nil, err
		}
		left = a
	}
	for {
		tok := s.Peek()
		prec, ok := prattPrec[tok.Value()]
		if !ok || prec < minPrec || (tok.Type() != "op" && tok.Type() != "kw" && tok.Type() != "concat") {
			return left, nil
		}
		s.Next()
		right, err := parsePratt(s, prec+1)
		if err != nil {
			return nil, err
		}
		left = &prattBin{tok.Value(), left, right}
	}
}

func bigExpr(n int) string {
	ops := []string{"+", "*", "-", "OR", "AND", "=", "<", "||", "/", ">"}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(" " + ops[i%len(ops)] + " ")
		}
		switch i % 3 {
		case 0:
			fmt.Fprintf(&sb, "a%d", i)
		case 1:
			fmt.Fprintf(&sb, "%d", i)
		default:
			fmt.Fprintf(&sb, "'s%d'", i)
		}
	}
	return sb.String()
}

func benchExpr(b *testing.B, dest func() any) {
	src := bigExpr(300)
	toks, err := Tokenise(src)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toks.Restore(0)
		if err := grammar.Parse(dest(), toks, grammar.Exhaust); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExprPrec(b *testing.B)  { benchExpr(b, func() any { return new(Expr) }) }
func BenchmarkExprPratt(b *testing.B) { benchExpr(b, func() any { return new(prattExpr) }) }
