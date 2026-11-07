package grammar

import (
	"strings"
	"testing"
)

// Two families of expressions with their own precedence tables: values,
// with an ascription operator whose right operand is a type, and types,
// with a union operator.  The levels of one table mean nothing to the other.

var pfTokenise = SimpleTokeniser([]TokenDef{
	{Ptn: `\s+`},
	{Name: "op", Ptn: `[:|+]`},
	{Name: "ident", Ptn: `[a-zA-Z]+`},
})

type pfExpr struct{ Sum[pfExprAlts] }

type pfExprAlts struct {
	OneOf
	Add  *pfAdd
	Asc  *pfAsc
	Name *pfName
}

type pfAdd struct {
	Seq `prec:"7"`
	Lhs pfExpr
	Op  Match `tok:"op,+"`
	Rhs pfExpr
}

// pfAsc ::= Expr ":" Type, binding looser than "+".
type pfAsc struct {
	Seq   `prec:"2"`
	Lhs   pfExpr
	Colon Match `tok:"op,:"`
	Type  pfType
}

type pfType struct{ Sum[pfTypeAlts] }

type pfTypeAlts struct {
	OneOf
	Union *pfUnion
	Name  *pfName
}

// pfUnion ::= Type "|" Type, at a level below that of pfAsc in the other
// table, which must not matter.
type pfUnion struct {
	Seq `prec:"1"`
	Lhs pfType
	Bar Match `tok:"op,|"`
	Rhs pfType
}

type pfName struct {
	Tok `tok:"ident"`
}

func pfLisp(e *pfExpr) string {
	switch v := e.Value.(type) {
	case *pfAdd:
		return "(+ " + pfLisp(&v.Lhs) + " " + pfLisp(&v.Rhs) + ")"
	case *pfAsc:
		return "(: " + pfLisp(&v.Lhs) + " " + pfLispType(&v.Type) + ")"
	case *pfName:
		return v.TokValue
	}
	panic("bad expr")
}

func pfLispType(t *pfType) string {
	switch v := t.Value.(type) {
	case *pfUnion:
		return "(| " + pfLispType(&v.Lhs) + " " + pfLispType(&v.Rhs) + ")"
	case *pfName:
		return v.TokValue
	}
	panic("bad type")
}

func TestPrecedenceFamilies(t *testing.T) {
	tests := []struct{ src, want string }{
		{`x : A | B`, `(: x (| A B))`}, // the type operand is not constrained by the level of ":"
		{`x + y : A`, `(: (+ x y) A)`},
		{`x : A | B | C`, `(: x (| (| A B) C))`},
		{`x + y + z`, `(+ (+ x y) z)`},
	}
	for _, tt := range tests {
		toks, _ := pfTokenise(tt.src)
		var e pfExpr
		if err := Parse(&e, toks, Exhaust); err != nil {
			t.Errorf("%s: %s", tt.src, err)
			continue
		}
		if got := pfLisp(&e); got != tt.want {
			t.Errorf("%s: got %s, want %s", tt.src, got, tt.want)
		}
	}
}

// An operator whose left operand is a group of the sum rather than the sum
// is a definition error.
type pgExpr struct{ Sum[pgExprAlts] }

type pgExprAlts struct {
	OneOf
	Ops  *pgOps
	Name *pfName
}

type pgOps struct{ OneOf2[pgAdd, pfName] }

type pgAdd struct {
	Seq `prec:"7"`
	Lhs pgOps // should be pgExpr
	Op  Match `tok:"op,+"`
	Rhs pgExpr
}

func TestPrecedenceGroupOperand(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "left operand of") {
			t.Errorf("expected a definition error, got %v", r)
		}
	}()
	toks, _ := pfTokenise(`x + y`)
	var e pgExpr
	Parse(&e, toks)
}

// Operators grouped in a OneOf that is an alternative of a plain OneOf
// rule: the group would be parsed without the bound, so it is a definition
// error; groups go in a Sum, which flattens them.
type pnExpr struct {
	OneOf
	Logical *pnLogical
	Name    *pfName
}

type pnLogical struct {
	OneOf
	Or *pnOr
}

type pnOr struct {
	Seq `prec:"1"`
	Lhs pnExpr
	Op  Match `tok:"op,|"`
	Rhs pnExpr
}

func TestPrecedenceGroupInOneOf(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "grammar.Sum") {
			t.Errorf("expected a definition error, got %v", r)
		}
	}()
	toks, _ := pfTokenise(`a | b`)
	var e pnExpr
	Parse(&e, toks)
}
