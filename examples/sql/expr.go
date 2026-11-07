package sql

import "github.com/arnodel/grammar"

//go:generate go run github.com/arnodel/grammar/cmd/genparse

// Expressions.  Expr is a sum of the operator rules, in semantic groups,
// and the atoms.  An operator rule carries its precedence on its Seq field,
// or gets it from its group: it only applies when the bound in effect is
// lower, and its operands after the first are parsed under its own
// precedence, which is what gives operators their binding power; the first
// operand, when it is an Expr, is the left recursive reference.  Rules
// without a precedence (atoms) always apply and reset the bound.  A prefix
// operator is not left recursive, so its precedence only binds its operand.
//
// The precedence levels are those of SQLite:
//
//	1  OR
//	2  AND
//	3  NOT (prefix)
//	4  = == != <> IS IS NOT BETWEEN IN LIKE GLOB REGEXP MATCH ISNULL NOTNULL NOT NULL
//	5  < > <= >=
//	6  & | << >>
//	7  + -
//	8  * / %
//	9  || -> ->>
//	10 - + ~ (prefix)
//	11 COLLATE (postfix)
//
// Expr is an expression: Value holds the alternative that matched.
type Expr struct {
	grammar.Sum[struct {
		grammar.OneOf
		Logical       *Logical
		Comparison    *Comparison
		Bitwise       *Bitwise
		Arithmetic    *Arithmetic
		Concatenation *Concatenation
		Collate       *Collate
		Atom          *Atom
	}]
}

// ---- Logical operators: OR, AND, NOT ----

type Logical struct {
	grammar.Sum[struct {
		grammar.OneOf
		Or  *Or
		And *And
		Not *Not
	}]
}

// Or ::= Expr (OR Expr)+, flat.
type Or struct {
	grammar.Seq `prec:"1"`
	First       Expr
	Rest        []OrTerm `size:"1-"`
}

type OrTerm struct {
	grammar.Seq `prec:"1"`
	Or          grammar.Match `tok:"kw,OR"`
	X           Expr
}

// And ::= Expr (AND Expr)+, flat.
type And struct {
	grammar.Seq `prec:"2"`
	First       Expr
	Rest        []AndTerm `size:"1-"`
}

type AndTerm struct {
	grammar.Seq `prec:"2"`
	And         grammar.Match `tok:"kw,AND"`
	X           Expr
}

// Not ::= NOT Expr
type Not struct {
	grammar.Seq `prec:"3"`
	Not         grammar.Match `tok:"kw,NOT"`
	X           Expr
}

// ---- Comparisons and predicates, all at level 4 except Ordering ----

// The order only matters between alternatives that can both apply, like
// NOT NULL, NOT BETWEEN, NOT IN and NOT LIKE.
type Comparison struct {
	grammar.Sum[struct {
		grammar.OneOf `prec:"4"`
		Equality      *Equality
		Ordering      *Ordering
		Is            *Is
		NullTest      *NullTest
		NotNull       *NotNull
		Between       *Between
		In            *In
		PatternMatch  *PatternMatch
	}]
}

// Equality ::= Expr ("=" | "==" | "!=" | "<>") Expr
type Equality struct {
	grammar.Seq
	Lhs Expr
	Op  Token `tok:"op,=|op,==|op,!=|op,<>"`
	Rhs Expr
}

// Ordering ::= Expr ("<" | ">" | "<=" | ">=") Expr, binding tighter than
// equality.
type Ordering struct {
	grammar.Seq `prec:"5"`
	Lhs         Expr
	Op          Token `tok:"op,<|op,>|op,<=|op,>="`
	Rhs         Expr
}

// Is ::= Expr IS [NOT] [DISTINCT FROM] Expr
type Is struct {
	grammar.Seq
	Lhs      Expr
	Is       grammar.Match `tok:"kw,IS"`
	Not      *Token        `tok:"kw,NOT"`
	Distinct *DistinctFrom
	Rhs      Expr
}

type DistinctFrom struct {
	grammar.Seq
	Distinct grammar.Match `tok:"kw,DISTINCT"`
	From     grammar.Match `tok:"kw,FROM"`
}

// NullTest ::= Expr (ISNULL | NOTNULL)
type NullTest struct {
	grammar.Seq
	Lhs Expr
	Op  Token `tok:"kw,ISNULL|kw,NOTNULL"`
}

// NotNull ::= Expr NOT NULL
type NotNull struct {
	grammar.Seq
	Lhs  Expr
	Not  grammar.Match `tok:"kw,NOT"`
	Null grammar.Match `tok:"kw,NULL"`
}

// Between ::= Expr [NOT] BETWEEN Expr AND Expr
type Between struct {
	grammar.Seq
	Lhs     Expr
	Not     *Token        `tok:"kw,NOT"`
	Between grammar.Match `tok:"kw,BETWEEN"`
	Low     Expr
	And     grammar.Match `tok:"kw,AND"`
	High    Expr
}

// In ::= Expr [NOT] IN ( "(" [SelectStmt | Expr, ...] ")" | QualifiedName )
type In struct {
	grammar.Seq
	Lhs    Expr
	Not    *Token        `tok:"kw,NOT"`
	In     grammar.Match `tok:"kw,IN"`
	Target InTarget
}

type InTarget struct {
	grammar.OneOf
	Subquery *ParenSelect
	List     *InList
	Table    *QualifiedName
}

type InList struct {
	grammar.Seq
	Open  grammar.Match `tok:"op,("`
	Exprs []Expr        `sep:"op,,"`
	Close grammar.Match `tok:"op,)"`
}

// PatternMatch ::= Expr [NOT] (LIKE | GLOB | REGEXP | MATCH) Expr [ESCAPE Expr]
type PatternMatch struct {
	grammar.Seq
	Lhs     Expr
	Not     *Token `tok:"kw,NOT"`
	Op      Token  `tok:"kw,LIKE|kw,GLOB|kw,REGEXP|kw,MATCH"`
	Pattern Expr
	Escape  *EscapeClause
}

type EscapeClause struct {
	grammar.Seq `prec:"4"`
	Escape      grammar.Match `tok:"kw,ESCAPE"`
	X           Expr
}

// ---- Bitwise operators ----

type Bitwise struct {
	grammar.Sum[struct {
		grammar.OneOf
		Binary     *BitwiseBinary
		Complement *BitwiseComplement
	}]
}

// BitwiseBinary ::= Expr (("&" | "|" | "<<" | ">>") Expr)+, flat with the
// operator of each term.
type BitwiseBinary struct {
	grammar.Seq `prec:"6"`
	First       Expr
	Rest        []BitwiseTerm `size:"1-"`
}

type BitwiseTerm struct {
	grammar.Seq `prec:"6"`
	Op          Token `tok:"op,&|bitor|op,<<|op,>>"`
	X           Expr
}

// BitwiseComplement ::= "~" Expr
type BitwiseComplement struct {
	grammar.Seq `prec:"10"`
	Tilde       grammar.Match `tok:"op,~"`
	X           Expr
}

// ---- Arithmetic operators, the binary ones as flat lists with the
// operator of each term ----

type Arithmetic struct {
	grammar.Sum[struct {
		grammar.OneOf
		Additive       *Additive
		Multiplicative *Multiplicative
		Sign           *Sign
	}]
}

// Additive ::= Expr (("+" | "-") Expr)+
type Additive struct {
	grammar.Seq `prec:"7"`
	First       Expr
	Rest        []AdditiveTerm `size:"1-"`
}

type AdditiveTerm struct {
	grammar.Seq `prec:"7"`
	Op          Token `tok:"op,+|op,-"`
	X           Expr
}

// Multiplicative ::= Expr (("*" | "/" | "%") Expr)+
type Multiplicative struct {
	grammar.Seq `prec:"8"`
	First       Expr
	Rest        []MultiplicativeTerm `size:"1-"`
}

type MultiplicativeTerm struct {
	grammar.Seq `prec:"8"`
	Op          Token `tok:"op,*|op,/|op,%"`
	X           Expr
}

// ---- String operators ----

// Concatenation ::= Expr (("||" | "->" | "->>") Expr)+: string
// concatenation and the JSON extraction operators, which SQLite puts at the
// same level.
type Concatenation struct {
	grammar.Seq `prec:"9"`
	First       Expr
	Rest        []ConcatenationTerm `size:"1-"`
}

type ConcatenationTerm struct {
	grammar.Seq `prec:"9"`
	Op          Token `tok:"concat|op,->|op,->>"`
	X           Expr
}

// Sign ::= ("-" | "+") Expr
type Sign struct {
	grammar.Seq `prec:"10"`
	Op          Token `tok:"op,-|op,+"`
	X           Expr
}

// ---- Collation, a postfix operator ----

// Collate ::= Expr COLLATE Name
type Collate struct {
	grammar.Seq `prec:"11"`
	Lhs         Expr
	Collate     grammar.Match `tok:"kw,COLLATE"`
	Name        Name
}

// Atom is an expression that does not involve an operator.  A function call
// is tried before a column reference because both start with a name; a
// subquery before a parenthesised expression because both start with "(".
type Atom struct {
	grammar.OneOf
	Literal  *Literal
	Param    *Token `tok:"param"`
	Cast     *CastExpr
	Case     *CaseExpr
	Exists   *ExistsExpr
	Func     *FuncCall
	Column   *ColumnRef
	Subquery *ParenSelect
	Paren    *ParenExprs
}

// Literal is a number, string or blob literal or one of the keyword literals.
// TRUE and FALSE are contextual keywords, so a column of that name is read as
// the literal, as SQLite does.
type Literal struct {
	grammar.Tok `tok:"number|string|blob|kw,NULL|ckw,TRUE|ckw,FALSE|kw,CURRENT_TIME|kw,CURRENT_DATE|kw,CURRENT_TIMESTAMP"`
}

// Name is an identifier, quoted or not; a contextual keyword is a valid name.
type Name struct {
	grammar.Tok `tok:"ident|ckw|qident"`
}

// QualifiedName ::= Name ["." Name]
type QualifiedName struct {
	grammar.Seq
	Parts []Name `sep:"op,." size:"1-2"`
}

// ColumnRef ::= Name ["." Name ["." Name]]
type ColumnRef struct {
	grammar.Seq
	Parts []Name `sep:"op,." size:"1-3"`
}

// CastExpr ::= CAST "(" Expr AS TypeName ")"
type CastExpr struct {
	grammar.Seq
	Cast  grammar.Match `tok:"kw,CAST"`
	Open  grammar.Match `tok:"op,("`
	Expr  Expr
	As    grammar.Match `tok:"kw,AS"`
	Type  TypeName
	Close grammar.Match `tok:"op,)"`
}

// TypeName ::= Name+ ["(" number ["," number] ")"]
type TypeName struct {
	grammar.Seq
	Names []Name `size:"1-"`
	Size  *TypeSize
}

type TypeSize struct {
	grammar.Seq
	Open  grammar.Match  `tok:"op,("`
	Sizes []SignedNumber `sep:"op,," size:"1-2"`
	Close grammar.Match  `tok:"op,)"`
}

type SignedNumber struct {
	grammar.Seq
	Sign   *Token `tok:"op,-|op,+"`
	Number Token  `tok:"number"`
}

// CaseExpr ::= CASE [Expr] (WHEN Expr THEN Expr)+ [ELSE Expr] END
type CaseExpr struct {
	grammar.Seq
	Case    grammar.Match `tok:"kw,CASE"`
	Operand *Expr
	Whens   []WhenClause `size:"1-"`
	Else    *ElseClause
	End     grammar.Match `tok:"kw,END"`
}

type WhenClause struct {
	grammar.Seq
	When grammar.Match `tok:"kw,WHEN"`
	Cond Expr
	Then grammar.Match `tok:"kw,THEN"`
	Expr Expr
}

type ElseClause struct {
	grammar.Seq
	Else grammar.Match `tok:"kw,ELSE"`
	Expr Expr
}

// ExistsExpr ::= EXISTS "(" SelectStmt ")"
type ExistsExpr struct {
	grammar.Seq
	Exists grammar.Match `tok:"kw,EXISTS"`
	Select ParenSelect
}

// FuncCall ::= Name "(" ["*"] [DISTINCT] [Expr, ...] [OrderBy] ")" [Filter] [Over]
type FuncCall struct {
	grammar.Seq
	Name     Name
	Open     grammar.Match `tok:"op,("`
	Star     *Token        `tok:"op,*"`
	Distinct *Token        `tok:"kw,DISTINCT"`
	Args     []Expr        `sep:"op,,"`
	OrderBy  *OrderBy
	Close    grammar.Match `tok:"op,)"`
	Filter   *FilterClause
	Over     *OverClause
}

// FilterClause ::= FILTER "(" WHERE Expr ")"
type FilterClause struct {
	grammar.Seq
	Filter grammar.Match `tok:"ckw,FILTER"`
	Open   grammar.Match `tok:"op,("`
	Where  Where
	Close  grammar.Match `tok:"op,)"`
}

// OverClause ::= OVER (WindowDef | Name)
type OverClause struct {
	grammar.Seq
	Over   grammar.Match `tok:"ckw,OVER"`
	Window WindowSpec
}

type WindowSpec struct {
	grammar.OneOf
	Def  *WindowDef
	Name *Name
}

// ParenSelect ::= "(" SelectStmt ")"
type ParenSelect struct {
	grammar.Seq
	Open   grammar.Match `tok:"op,("`
	Select SelectStmt
	Close  grammar.Match `tok:"op,)"`
}

// ParenExprs ::= "(" Expr, ... ")"
//
// With more than one expression this is a row value.
type ParenExprs struct {
	grammar.Seq
	Open  grammar.Match `tok:"op,("`
	Exprs []Expr        `sep:"op,," size:"1-"`
	Close grammar.Match `tok:"op,)"`
}
