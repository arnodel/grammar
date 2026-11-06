package sql

import "github.com/arnodel/grammar"

//go:generate go run github.com/arnodel/grammar/cmd/genparse

// Expressions, from the lowest to the highest precedence.  Each binary level
// is a list "operand (op operand)*" rather than a left-recursive rule: that is
// the natural shape for the n-ary AND and OR, it is cheaper to parse, and a
// consumer folding the list from the left gets left associativity.
//
// The precedence levels are those of SQLite:
//
//	OR
//	AND
//	NOT (prefix)
//	= == != <> IS IS NOT BETWEEN IN LIKE GLOB REGEXP MATCH ISNULL NOTNULL NOT NULL
//	< > <= >=
//	& | << >>
//	+ -
//	* / %
//	|| -> ->>
//	- + ~ (prefix)
//	COLLATE (postfix)

// Expr ::= AndExpr (OR AndExpr)*
type Expr struct {
	grammar.Seq
	First AndExpr
	Rest  []OrTerm
}

type OrTerm struct {
	grammar.Seq
	Or   grammar.Match `tok:"kw,OR"`
	Expr AndExpr
}

// AndExpr ::= NotExpr (AND NotExpr)*
type AndExpr struct {
	grammar.Seq
	First NotExpr
	Rest  []AndTerm
}

type AndTerm struct {
	grammar.Seq
	And  grammar.Match `tok:"kw,AND"`
	Expr NotExpr
}

// NotExpr ::= NOT NotExpr | CmpExpr
type NotExpr struct {
	grammar.OneOf
	Not *Negation
	Cmp *CmpExpr
}

type Negation struct {
	grammar.Seq
	Not  grammar.Match `tok:"kw,NOT"`
	Expr NotExpr
}

// CmpExpr ::= RelExpr CmpTail*
//
// The tails are the comparison operators proper and the various predicates
// (IS, BETWEEN, IN, LIKE, ...), which all have the same precedence.
type CmpExpr struct {
	grammar.Seq
	First RelExpr
	Rest  []CmpTail
}

type CmpTail struct {
	grammar.OneOf
	Cmp     *CmpTerm
	Is      *IsTerm
	Null    *Token `tok:"kw,ISNULL|kw,NOTNULL"`
	NotNull *NotNullTerm
	Between *BetweenTerm
	In      *InTerm
	Like    *LikeTerm
}

type CmpTerm struct {
	grammar.Seq
	Op   Token `tok:"op,=|op,==|op,!=|op,<>"`
	Expr RelExpr
}

// IsTerm ::= IS [NOT] [DISTINCT FROM] RelExpr
type IsTerm struct {
	grammar.Seq
	Is       grammar.Match `tok:"kw,IS"`
	Not      *Token        `tok:"kw,NOT"`
	Distinct *DistinctFrom
	Expr     RelExpr
}

type DistinctFrom struct {
	grammar.Seq
	Distinct grammar.Match `tok:"kw,DISTINCT"`
	From     grammar.Match `tok:"kw,FROM"`
}

type NotNullTerm struct {
	grammar.Seq
	Not  grammar.Match `tok:"kw,NOT"`
	Null grammar.Match `tok:"kw,NULL"`
}

// BetweenTerm ::= [NOT] BETWEEN RelExpr AND RelExpr
type BetweenTerm struct {
	grammar.Seq
	Not     *Token        `tok:"kw,NOT"`
	Between grammar.Match `tok:"kw,BETWEEN"`
	Low     RelExpr
	And     grammar.Match `tok:"kw,AND"`
	High    RelExpr
}

// InTerm ::= [NOT] IN ( "(" [SelectStmt | Expr, ...] ")" | QualifiedName )
type InTerm struct {
	grammar.Seq
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

// LikeTerm ::= [NOT] (LIKE | GLOB | REGEXP | MATCH) RelExpr [ESCAPE RelExpr]
type LikeTerm struct {
	grammar.Seq
	Not     *Token `tok:"kw,NOT"`
	Op      Token  `tok:"kw,LIKE|kw,GLOB|kw,REGEXP|kw,MATCH"`
	Pattern RelExpr
	Escape  *EscapeClause
}

type EscapeClause struct {
	grammar.Seq
	Escape grammar.Match `tok:"kw,ESCAPE"`
	Expr   RelExpr
}

// RelExpr ::= BitExpr (("<" | ">" | "<=" | ">=") BitExpr)*
type RelExpr struct {
	grammar.Seq
	First BitExpr
	Rest  []RelTerm
}

type RelTerm struct {
	grammar.Seq
	Op   Token `tok:"op,<|op,>|op,<=|op,>="`
	Expr BitExpr
}

// BitExpr ::= AddExpr (("&" | "|" | "<<" | ">>") AddExpr)*
type BitExpr struct {
	grammar.Seq
	First AddExpr
	Rest  []BitTerm
}

type BitTerm struct {
	grammar.Seq
	Op   Token `tok:"op,&|bitor|op,<<|op,>>"`
	Expr AddExpr
}

// AddExpr ::= MulExpr (("+" | "-") MulExpr)*
type AddExpr struct {
	grammar.Seq
	First MulExpr
	Rest  []AddTerm
}

type AddTerm struct {
	grammar.Seq
	Op   Token `tok:"op,+|op,-"`
	Expr MulExpr
}

// MulExpr ::= ConcatExpr (("*" | "/" | "%") ConcatExpr)*
type MulExpr struct {
	grammar.Seq
	First ConcatExpr
	Rest  []MulTerm
}

type MulTerm struct {
	grammar.Seq
	Op   Token `tok:"op,*|op,/|op,%"`
	Expr ConcatExpr
}

// ConcatExpr ::= UnaryExpr (("||" | "->" | "->>") UnaryExpr)*
type ConcatExpr struct {
	grammar.Seq
	First UnaryExpr
	Rest  []ConcatTerm
}

type ConcatTerm struct {
	grammar.Seq
	Op   Token `tok:"concat|op,->|op,->>"`
	Expr UnaryExpr
}

// UnaryExpr ::= ("-" | "+" | "~") UnaryExpr | CollateExpr
type UnaryExpr struct {
	grammar.OneOf
	Op      *UnaryOp
	Collate *CollateExpr
}

type UnaryOp struct {
	grammar.Seq
	Op   Token `tok:"op,-|op,+|op,~"`
	Expr UnaryExpr
}

// CollateExpr ::= Atom (COLLATE Name)*
type CollateExpr struct {
	grammar.Seq
	Atom    Atom
	Collate []Collation
}

type Collation struct {
	grammar.Seq
	Collate grammar.Match `tok:"kw,COLLATE"`
	Name    Name
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
