package sql

import (
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
		{`count(DISTINCT a) FILTER (WHERE b) OVER w`, `(count DISTINCT a FILTER OVER)`},
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

// lisp writes an expression tree as an S-expression.  Binary levels are
// folded from the left, as SQL operators are left associative.
func lisp(e *Expr) string {
	parts := []string{lispAnd(e.First)}
	for _, t := range e.Rest {
		parts = append(parts, lispAnd(t.Expr))
	}
	return nary("OR", parts)
}

func nary(op string, parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + op + " " + strings.Join(parts, " ") + ")"
}

func lispAnd(e AndExpr) string {
	parts := []string{lispNot(e.First)}
	for _, t := range e.Rest {
		parts = append(parts, lispNot(t.Expr))
	}
	return nary("AND", parts)
}

func lispNot(e NotExpr) string {
	if e.Not != nil {
		return "(NOT " + lispNot(e.Not.Expr) + ")"
	}
	return lispCmp(*e.Cmp)
}

func not(tok *Token) string {
	if tok != nil {
		return "NOT "
	}
	return ""
}

func lispCmp(e CmpExpr) string {
	s := lispRel(e.First)
	for _, tail := range e.Rest {
		switch {
		case tail.Cmp != nil:
			s = "(" + tail.Cmp.Op.TokValue + " " + s + " " + lispRel(tail.Cmp.Expr) + ")"
		case tail.Is != nil:
			op := "IS " + not(tail.Is.Not)
			if tail.Is.Distinct != nil {
				op += "DISTINCT FROM "
			}
			s = "(" + op + s + " " + lispRel(tail.Is.Expr) + ")"
		case tail.Null != nil:
			s = "(" + tail.Null.TokValue + " " + s + ")"
		case tail.NotNull != nil:
			s = "(NOTNULL " + s + ")"
		case tail.Between != nil:
			b := tail.Between
			s = "(" + not(b.Not) + "BETWEEN " + s + " " + lispRel(b.Low) + " " + lispRel(b.High) + ")"
		case tail.In != nil:
			in := tail.In
			var target string
			switch {
			case in.Target.Subquery != nil:
				target = "(SELECT)"
			case in.Target.List != nil:
				var items []string
				for _, item := range in.Target.List.Exprs {
					items = append(items, lisp(&item))
				}
				target = "(" + strings.Join(items, " ") + ")"
			default:
				target = lispNames(in.Target.Table.Parts)
			}
			s = "(" + not(in.Not) + "IN " + s + " " + target + ")"
		case tail.Like != nil:
			like := tail.Like
			s = "(" + not(like.Not) + like.Op.TokValue + " " + s + " " + lispRel(like.Pattern)
			if like.Escape != nil {
				s += " ESCAPE " + lispRel(like.Escape.Expr)
			}
			s += ")"
		}
	}
	return s
}

func lispRel(e RelExpr) string {
	s := lispBit(e.First)
	for _, t := range e.Rest {
		s = "(" + t.Op.TokValue + " " + s + " " + lispBit(t.Expr) + ")"
	}
	return s
}

func lispBit(e BitExpr) string {
	s := lispAdd(e.First)
	for _, t := range e.Rest {
		s = "(" + t.Op.TokValue + " " + s + " " + lispAdd(t.Expr) + ")"
	}
	return s
}

func lispAdd(e AddExpr) string {
	s := lispMul(e.First)
	for _, t := range e.Rest {
		s = "(" + t.Op.TokValue + " " + s + " " + lispMul(t.Expr) + ")"
	}
	return s
}

func lispMul(e MulExpr) string {
	s := lispConcat(e.First)
	for _, t := range e.Rest {
		s = "(" + t.Op.TokValue + " " + s + " " + lispConcat(t.Expr) + ")"
	}
	return s
}

func lispConcat(e ConcatExpr) string {
	s := lispUnary(e.First)
	for _, t := range e.Rest {
		s = "(" + t.Op.TokValue + " " + s + " " + lispUnary(t.Expr) + ")"
	}
	return s
}

func lispUnary(e UnaryExpr) string {
	if e.Op != nil {
		return "(" + e.Op.Op.TokValue + " " + lispUnary(e.Op.Expr) + ")"
	}
	s := lispAtom(e.Collate.Atom)
	for _, c := range e.Collate.Collate {
		s = "(COLLATE " + s + " " + c.Name.TokValue + ")"
	}
	return s
}

func lispNames(names []Name) string {
	var parts []string
	for _, n := range names {
		parts = append(parts, n.TokValue)
	}
	return strings.Join(parts, ".")
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
			parts = append(parts, "FILTER")
		}
		if f.Over != nil {
			parts = append(parts, "OVER")
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
