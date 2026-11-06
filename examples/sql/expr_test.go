package sql

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arnodel/grammar"
)

// TestExprPrecedence checks the shape of the expression trees by printing
// them as S-expressions.
func TestExprPrecedence(t *testing.T) {
	tests := []struct{ src, want string }{
		{`1 + 2 * 3`, `(+ 1 (* 2 3))`},
		{`1 - 2 - 3`, `(- (- 1 2) 3)`},
		{`a OR b AND c`, `(OR a (AND b c))`},
		{`a OR b OR c`, `(OR a b c)`},
		{`NOT a = b`, `(NOT (= a b))`},
		{`NOT NOT a`, `(NOT (NOT a))`},
		{`a = b < c`, `(= a (< b c))`},
		{`a || b * c`, `(* (|| a b) c)`},
		{`-a || b`, `(|| (- a) b)`},
		{`- - a`, `(- (- a))`},
		{`a COLLATE nocase = b`, `(= (COLLATE a nocase) b)`},
		{`x NOT BETWEEN 1 + 1 AND 10`, `(NOT BETWEEN x (+ 1 1) 10)`},
		{`x IN (1, 2) AND y NOT IN (SELECT z FROM t)`, `(AND (IN x (1 2)) (NOT IN y (SELECT)))`},
		{`x IN t`, `(IN x t)`},
		{`x IS NOT NULL OR x NOT NULL OR x ISNULL`, `(OR (IS NOT x NULL) (NOTNULL x) (ISNULL x))`},
		{`x IS DISTINCT FROM y`, `(IS DISTINCT FROM x y)`},
		{`a LIKE 'b%' ESCAPE '\'`, `(LIKE a 'b%' ESCAPE '\')`},
		{`a NOT GLOB 'b*'`, `(NOT GLOB a 'b*')`},
		{`x & 1 | 2 << 3`, `(<< (| (& x 1) 2) 3)`},
		{`CASE a WHEN 1 THEN 'x' ELSE 'y' END`, `(CASE a (WHEN 1 'x') (ELSE 'y'))`},
		{`CASE WHEN a THEN 1 END`, `(CASE (WHEN a 1))`},
		{`count(DISTINCT a) FILTER (WHERE b > 1) OVER w`, `(count DISTINCT a (FILTER (> b 1)) (OVER w))`},
		{`sum(x) OVER (PARTITION BY d, e ORDER BY s DESC ROWS UNBOUNDED PRECEDING)`, `(sum x (OVER (PARTITION d e) (ORDER (s DESC)) (ROWS ...)))`},
		{`sum(x) OVER (w ORDER BY s)`, `(sum x (OVER w (ORDER s)))`},
		{`count(*)`, `(count *)`},
		{`f()`, `(f)`},
		{`(1, 2) = (3, 4)`, `(= (ROW 1 2) (ROW 3 4))`},
		{`(1 + 2) * 3`, `(* (+ 1 2) 3)`},
		{`t.a.b`, `t.a.b`},
		{`CAST(x AS VARCHAR(10))`, `(CAST x VARCHAR(10))`},
		{`? + :name`, `(+ ? :name)`},
		{`a -> '$.b' ->> 'c'`, `(->> (-> a '$.b') 'c')`},
		{`NOT EXISTS (SELECT 1)`, `(NOT (EXISTS (SELECT)))`},
		{`first + last`, `(+ FIRST LAST)`}, // contextual keywords as columns
		{`TRUE AND NULL`, `(AND TRUE NULL)`},
	}
	for _, tt := range tests {
		toks, err := Tokenise(tt.src)
		if err != nil {
			t.Errorf("%s: %s", tt.src, err)
			continue
		}
		var expr Expr
		if err := grammar.Parse(&expr, toks, grammar.Exhaust); err != nil {
			t.Errorf("%s: %s", tt.src, err)
			continue
		}
		if got := lisp(&expr); got != tt.want {
			t.Errorf("%s:\n got: %s\nwant: %s", tt.src, got, tt.want)
		}
	}
}

// lisp writes an expression tree as an S-expression.  Flat lists are
// folded from the left, as SQL operators are left associative.
func lisp(e *Expr) string {
	switch v := e.Value.(type) {
	case *Logical:
		return lispLogical(v)
	case *Comparison:
		return lispComparison(v)
	case *Bitwise:
		return lispBitwise(v)
	case *Arithmetic:
		return lispArithmetic(v)
	case *Concatenation:
		return lispTerms(lisp(&v.First), v.Rest, func(t ConcatenationTerm) (string, *Expr) { return t.Op.TokValue, &t.X })
	case *Collate:
		return "(COLLATE " + lisp(&v.Lhs) + " " + v.Name.TokValue + ")"
	case *Atom:
		return lispAtom(*v)
	}
	panic(fmt.Sprintf("unexpected expression %T", e.Value))
}

func lispLogical(l *Logical) string {
	switch v := l.Value.(type) {
	case *Or:
		return nary("OR", v.First, orOperands(v.Rest))
	case *And:
		return nary("AND", v.First, andOperands(v.Rest))
	case *Not:
		return "(NOT " + lisp(&v.X) + ")"
	}
	panic(fmt.Sprintf("unexpected logical operator %T", l.Value))
}

func lispComparison(c *Comparison) string {
	switch v := c.Value.(type) {
	case *Equality:
		return "(" + v.Op.TokValue + " " + lisp(&v.Lhs) + " " + lisp(&v.Rhs) + ")"
	case *Ordering:
		return "(" + v.Op.TokValue + " " + lisp(&v.Lhs) + " " + lisp(&v.Rhs) + ")"
	case *Is:
		op := "IS " + not(v.Not)
		if v.Distinct != nil {
			op += "DISTINCT FROM "
		}
		return "(" + op + lisp(&v.Lhs) + " " + lisp(&v.Rhs) + ")"
	case *NullTest:
		return "(" + v.Op.TokValue + " " + lisp(&v.Lhs) + ")"
	case *NotNull:
		return "(NOTNULL " + lisp(&v.Lhs) + ")"
	case *Between:
		return "(" + not(v.Not) + "BETWEEN " + lisp(&v.Lhs) + " " + lisp(&v.Low) + " " + lisp(&v.High) + ")"
	case *In:
		var target string
		switch {
		case v.Target.Subquery != nil:
			target = "(SELECT)"
		case v.Target.List != nil:
			var items []string
			for _, item := range v.Target.List.Exprs {
				items = append(items, lisp(&item))
			}
			target = "(" + strings.Join(items, " ") + ")"
		default:
			target = lispNames(v.Target.Table.Parts)
		}
		return "(" + not(v.Not) + "IN " + lisp(&v.Lhs) + " " + target + ")"
	case *PatternMatch:
		s := "(" + not(v.Not) + v.Op.TokValue + " " + lisp(&v.Lhs) + " " + lisp(&v.Pattern)
		if v.Escape != nil {
			s += " ESCAPE " + lisp(&v.Escape.X)
		}
		return s + ")"
	}
	panic(fmt.Sprintf("unexpected comparison %T", c.Value))
}

func lispBitwise(b *Bitwise) string {
	switch v := b.Value.(type) {
	case *BitwiseBinary:
		return lispTerms(lisp(&v.First), v.Rest, func(t BitwiseTerm) (string, *Expr) { return t.Op.TokValue, &t.X })
	case *BitwiseComplement:
		return "(~ " + lisp(&v.X) + ")"
	}
	panic(fmt.Sprintf("unexpected bitwise operator %T", b.Value))
}

func lispArithmetic(a *Arithmetic) string {
	switch v := a.Value.(type) {
	case *Additive:
		return lispTerms(lisp(&v.First), v.Rest, func(t AdditiveTerm) (string, *Expr) { return t.Op.TokValue, &t.X })
	case *Multiplicative:
		return lispTerms(lisp(&v.First), v.Rest, func(t MultiplicativeTerm) (string, *Expr) { return t.Op.TokValue, &t.X })
	case *Sign:
		return "(" + v.Op.TokValue + " " + lisp(&v.X) + ")"
	}
	panic(fmt.Sprintf("unexpected arithmetic %T", a.Value))
}

func nary(op string, first Expr, rest []Expr) string {
	parts := []string{lisp(&first)}
	for i := range rest {
		parts = append(parts, lisp(&rest[i]))
	}
	return "(" + op + " " + strings.Join(parts, " ") + ")"
}

func orOperands(terms []OrTerm) (xs []Expr) {
	for _, t := range terms {
		xs = append(xs, t.X)
	}
	return xs
}

func andOperands(terms []AndTerm) (xs []Expr) {
	for _, t := range terms {
		xs = append(xs, t.X)
	}
	return xs
}

func not(tok *Token) string {
	if tok != nil {
		return "NOT "
	}
	return ""
}

func lispNames(names []Name) string {
	var parts []string
	for _, n := range names {
		parts = append(parts, n.TokValue)
	}
	return strings.Join(parts, ".")
}

// lispTerms folds a flat operator list from the left.
func lispTerms[T any](first string, rest []T, term func(T) (string, *Expr)) string {
	s := first
	for _, t := range rest {
		op, x := term(t)
		s = "(" + op + " " + s + " " + lisp(x) + ")"
	}
	return s
}

func lispOver(o *OverClause) string {
	if o.Window.Name != nil {
		return "(OVER " + o.Window.Name.TokValue + ")"
	}
	def := o.Window.Def
	parts := []string{"OVER"}
	if def.Base != nil {
		parts = append(parts, def.Base.TokValue)
	}
	if def.Partition != nil {
		var exprs []string
		for i := range def.Partition.Exprs {
			exprs = append(exprs, lisp(&def.Partition.Exprs[i]))
		}
		parts = append(parts, "(PARTITION "+strings.Join(exprs, " ")+")")
	}
	if def.OrderBy != nil {
		var terms []string
		for i := range def.OrderBy.Terms {
			term := lisp(&def.OrderBy.Terms[i].Expr)
			if dir := def.OrderBy.Terms[i].Dir; dir != nil {
				term = "(" + term + " " + dir.TokValue + ")"
			}
			terms = append(terms, term)
		}
		parts = append(parts, "(ORDER "+strings.Join(terms, " ")+")")
	}
	if def.Frame != nil {
		parts = append(parts, "("+def.Frame.Mode.TokValue+" ...)")
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func lispAtom(a Atom) string {
	switch {
	case a.Literal != nil:
		return a.Literal.TokValue
	case a.Param != nil:
		return a.Param.TokValue
	case a.Cast != nil:
		var names []string
		for _, n := range a.Cast.Type.Names {
			names = append(names, n.TokValue)
		}
		s := "(CAST " + lisp(&a.Cast.Expr) + " " + strings.Join(names, " ")
		if size := a.Cast.Type.Size; size != nil {
			var sizes []string
			for _, n := range size.Sizes {
				sizes = append(sizes, n.Number.TokValue)
			}
			s += "(" + strings.Join(sizes, ", ") + ")"
		}
		return s + ")"
	case a.Case != nil:
		s := "(CASE"
		if a.Case.Operand != nil {
			s += " " + lisp(a.Case.Operand)
		}
		for _, w := range a.Case.Whens {
			s += " (WHEN " + lisp(&w.Cond) + " " + lisp(&w.Expr) + ")"
		}
		if a.Case.Else != nil {
			s += " (ELSE " + lisp(&a.Case.Else.Expr) + ")"
		}
		return s + ")"
	case a.Exists != nil:
		return "(EXISTS (SELECT))"
	case a.Func != nil:
		f := a.Func
		parts := []string{f.Name.TokValue}
		if f.Star != nil {
			parts = append(parts, "*")
		}
		if f.Distinct != nil {
			parts = append(parts, "DISTINCT")
		}
		for _, arg := range f.Args {
			parts = append(parts, lisp(&arg))
		}
		if f.Filter != nil {
			parts = append(parts, "(FILTER "+lisp(&f.Filter.Where.Expr)+")")
		}
		if f.Over != nil {
			parts = append(parts, lispOver(f.Over))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case a.Column != nil:
		return lispNames(a.Column.Parts)
	case a.Subquery != nil:
		return "(SELECT)"
	case a.Paren != nil:
		var items []string
		for _, item := range a.Paren.Exprs {
			items = append(items, lisp(&item))
		}
		if len(items) == 1 {
			return items[0]
		}
		return "(ROW " + strings.Join(items, " ") + ")"
	}
	panic("empty atom")
}
