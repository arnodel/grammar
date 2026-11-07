package sql

import "github.com/arnodel/grammar"

//go:generate go run github.com/arnodel/grammar/cmd/genparse

// Script ::= Stmt (";" Stmt)* [";"]
type Script struct {
	grammar.Seq
	Stmts []Stmt `sep:"op,;" trail:"optional"`
}

// Stmt is one of the statements supported.  Each starts with an optional
// WITH clause: the alternatives all parse it at the same position, and
// memoisation means it is only parsed once.
type Stmt struct {
	grammar.OneOf
	Select *SelectStmt
	Insert *InsertStmt
	Update *UpdateStmt
	Delete *DeleteStmt
}

// ---- SELECT ----

// SelectStmt ::= [WithClause] SelectCore (CompoundOp SelectCore)* [OrderBy] [Limit]
type SelectStmt struct {
	grammar.Seq
	With    *WithClause
	First   SelectCore
	Rest    []Compound
	OrderBy *OrderBy
	Limit   *Limit
}

type Compound struct {
	grammar.Seq
	Op     CompoundOp
	Select SelectCore
}

type CompoundOp struct {
	grammar.OneOf
	UnionAll  *UnionAll
	Union     *Token `tok:"kw,UNION"`
	Intersect *Token `tok:"kw,INTERSECT"`
	Except    *Token `tok:"kw,EXCEPT"`
}

type UnionAll struct {
	grammar.Seq
	Union grammar.Match `tok:"kw,UNION"`
	All   grammar.Match `tok:"kw,ALL"`
}

// WithClause ::= WITH [RECURSIVE] CTE, ...
type WithClause struct {
	grammar.Seq
	With      grammar.Match `tok:"kw,WITH"`
	Recursive *Token        `tok:"ckw,RECURSIVE"`
	CTEs      []CTE         `sep:"op,," size:"1-"`
}

// CTE ::= Name [ColumnList] AS [[NOT] MATERIALIZED] "(" SelectStmt ")"
type CTE struct {
	grammar.Seq
	Name         Name
	Columns      *ColumnList
	As           grammar.Match `tok:"kw,AS"`
	Materialized *Materialized
	Select       ParenSelect
}

type Materialized struct {
	grammar.Seq
	Not          *Token        `tok:"kw,NOT"`
	Materialized grammar.Match `tok:"ckw,MATERIALIZED"`
}

// ColumnList ::= "(" Name, ... ")"
type ColumnList struct {
	grammar.Seq
	Open  grammar.Match `tok:"op,("`
	Names []Name        `sep:"op,," size:"1-"`
	Close grammar.Match `tok:"op,)"`
}

type SelectCore struct {
	grammar.OneOf
	Select *Select
	Values *Values
}

// Select ::= SELECT [DISTINCT | ALL] ResultColumn, ... [From] [Where] [GroupBy] [WindowClause]
type Select struct {
	grammar.Seq
	Select  grammar.Match  `tok:"kw,SELECT"`
	Quant   *Token         `tok:"kw,DISTINCT|kw,ALL"`
	Columns []ResultColumn `sep:"op,," size:"1-"`
	From    *From
	Where   *Where
	GroupBy *GroupBy
	Window  *WindowClause
}

// Values ::= VALUES ParenExprs, ...
type Values struct {
	grammar.Seq
	Values grammar.Match `tok:"kw,VALUES"`
	Rows   []ParenExprs  `sep:"op,," size:"1-"`
}

// ResultColumn ::= "*" | Name "." "*" | Expr [Alias]
type ResultColumn struct {
	grammar.OneOf
	Star      *Token `tok:"op,*"`
	TableStar *TableStar
	Expr      *AliasedExpr
}

type TableStar struct {
	grammar.Seq
	Table Name
	Dot   grammar.Match `tok:"op,."`
	Star  grammar.Match `tok:"op,*"`
}

type AliasedExpr struct {
	grammar.Seq
	Expr  Expr
	Alias *Alias
}

// Alias ::= [AS] Name
type Alias struct {
	grammar.Seq
	As   *Token `tok:"kw,AS"`
	Name Name
}

// From ::= FROM JoinClause
type From struct {
	grammar.Seq
	From grammar.Match `tok:"kw,FROM"`
	Join JoinClause
}

// JoinClause ::= TableOrSubquery (JoinOp TableOrSubquery [JoinConstraint])*
type JoinClause struct {
	grammar.Seq
	First TableOrSubquery
	Joins []Join
}

type Join struct {
	grammar.Seq
	Op         JoinOp
	Table      TableOrSubquery
	Constraint *JoinConstraint
}

// JoinOp ::= "," | [NATURAL] [LEFT | RIGHT | FULL [OUTER] | INNER | CROSS] JOIN
type JoinOp struct {
	grammar.OneOf
	Comma *Token `tok:"op,,"`
	Join  *JoinKw
}

type JoinKw struct {
	grammar.Seq
	Natural *Token `tok:"kw,NATURAL"`
	Type    *JoinType
	Join    grammar.Match `tok:"kw,JOIN"`
}

type JoinType struct {
	grammar.OneOf
	Outer *OuterJoin
	Inner *Token `tok:"kw,INNER"`
	Cross *Token `tok:"kw,CROSS"`
}

type OuterJoin struct {
	grammar.Seq
	Dir   Token  `tok:"kw,LEFT|kw,RIGHT|kw,FULL"`
	Outer *Token `tok:"kw,OUTER"`
}

type JoinConstraint struct {
	grammar.OneOf
	On    *OnClause
	Using *UsingClause
}

type OnClause struct {
	grammar.Seq
	On   grammar.Match `tok:"kw,ON"`
	Expr Expr
}

type UsingClause struct {
	grammar.Seq
	Using   grammar.Match `tok:"kw,USING"`
	Columns ColumnList
}

// TableOrSubquery ::= TableFunc | TableRef | Subquery | "(" JoinClause ")"
type TableOrSubquery struct {
	grammar.OneOf
	Func     *TableFunc
	Table    *TableRef
	Subquery *Subquery
	Join     *ParenJoin
}

// TableRef ::= QualifiedName [Alias] [IndexedBy]
type TableRef struct {
	grammar.Seq
	Name    QualifiedName
	Alias   *Alias
	Indexed *IndexedBy
}

type IndexedBy struct {
	grammar.OneOf
	By  *IndexedByName
	Not *NotIndexed
}

type IndexedByName struct {
	grammar.Seq
	Indexed grammar.Match `tok:"kw,INDEXED"`
	By      grammar.Match `tok:"kw,BY"`
	Name    Name
}

type NotIndexed struct {
	grammar.Seq
	Not     grammar.Match `tok:"kw,NOT"`
	Indexed grammar.Match `tok:"kw,INDEXED"`
}

// TableFunc ::= QualifiedName "(" [Expr, ...] ")" [Alias]
type TableFunc struct {
	grammar.Seq
	Name  QualifiedName
	Open  grammar.Match `tok:"op,("`
	Args  []Expr        `sep:"op,,"`
	Close grammar.Match `tok:"op,)"`
	Alias *Alias
}

// Subquery ::= "(" SelectStmt ")" [Alias]
type Subquery struct {
	grammar.Seq
	Select ParenSelect
	Alias  *Alias
}

type ParenJoin struct {
	grammar.Seq
	Open  grammar.Match `tok:"op,("`
	Join  JoinClause
	Close grammar.Match `tok:"op,)"`
}

type Where struct {
	grammar.Seq
	Where grammar.Match `tok:"kw,WHERE"`
	Expr  Expr
}

// GroupBy ::= GROUP BY Expr, ... [HAVING Expr]
type GroupBy struct {
	grammar.Seq
	Group  grammar.Match `tok:"kw,GROUP"`
	By     grammar.Match `tok:"kw,BY"`
	Exprs  []Expr        `sep:"op,," size:"1-"`
	Having *Having
}

type Having struct {
	grammar.Seq
	Having grammar.Match `tok:"kw,HAVING"`
	Expr   Expr
}

// OrderBy ::= ORDER BY OrderingTerm, ...
type OrderBy struct {
	grammar.Seq
	Order grammar.Match  `tok:"kw,ORDER"`
	By    grammar.Match  `tok:"kw,BY"`
	Terms []OrderingTerm `sep:"op,," size:"1-"`
}

// OrderingTerm ::= Expr [ASC | DESC] [NULLS (FIRST | LAST)]
type OrderingTerm struct {
	grammar.Seq
	Expr  Expr
	Dir   *Token `tok:"ckw,ASC|ckw,DESC"`
	Nulls *NullsOrder
}

type NullsOrder struct {
	grammar.Seq
	Nulls grammar.Match `tok:"ckw,NULLS"`
	Order Token         `tok:"ckw,FIRST|ckw,LAST"`
}

// Limit ::= LIMIT Expr [(OFFSET | ",") Expr]
type Limit struct {
	grammar.Seq
	Limit  grammar.Match `tok:"kw,LIMIT"`
	Count  Expr
	Offset *Offset
}

type Offset struct {
	grammar.Seq
	Sep  Token `tok:"ckw,OFFSET|op,,"`
	Expr Expr
}

// ---- Windows ----

// WindowClause ::= WINDOW Name AS WindowDef, ...
type WindowClause struct {
	grammar.Seq
	Window grammar.Match `tok:"kw,WINDOW"`
	Defs   []NamedWindow `sep:"op,," size:"1-"`
}

type NamedWindow struct {
	grammar.Seq
	Name Name
	As   grammar.Match `tok:"kw,AS"`
	Def  WindowDef
}

// WindowDef ::= "(" [base-window-name] [PARTITION BY Expr, ...] [OrderBy] [FrameSpec] ")"
//
// The base window name is restricted to plain identifiers: a contextual
// keyword here would be the start of a PARTITION BY or frame clause, and
// ordered choice does not come back to try that once a name has matched.
type WindowDef struct {
	grammar.Seq
	Open      grammar.Match `tok:"op,("`
	Base      *Token        `tok:"ident|qident"`
	Partition *PartitionBy
	OrderBy   *OrderBy
	Frame     *FrameSpec
	Close     grammar.Match `tok:"op,)"`
}

type PartitionBy struct {
	grammar.Seq
	Partition grammar.Match `tok:"ckw,PARTITION"`
	By        grammar.Match `tok:"kw,BY"`
	Exprs     []Expr        `sep:"op,," size:"1-"`
}

// FrameSpec ::= (RANGE | ROWS | GROUPS) (BETWEEN FrameBound AND FrameBound | FrameBound) [EXCLUDE ...]
type FrameSpec struct {
	grammar.Seq
	Mode    Token `tok:"ckw,RANGE|ckw,ROWS|ckw,GROUPS"`
	Bounds  FrameBounds
	Exclude *FrameExclude
}

type FrameBounds struct {
	grammar.OneOf
	Between *FrameBetween
	Start   *FrameBound
}

type FrameBetween struct {
	grammar.Seq
	Between grammar.Match `tok:"kw,BETWEEN"`
	Low     FrameBound
	And     grammar.Match `tok:"kw,AND"`
	High    FrameBound
}

// FrameBound ::= UNBOUNDED (PRECEDING | FOLLOWING) | CURRENT ROW | Expr (PRECEDING | FOLLOWING)
//
// UNBOUNDED and CURRENT are contextual keywords, so they would parse as column
// references if the Expr alternative came first.
type FrameBound struct {
	grammar.OneOf
	Unbounded *UnboundedBound
	Current   *CurrentRow
	Expr      *ExprBound
}

type UnboundedBound struct {
	grammar.Seq
	Unbounded grammar.Match `tok:"ckw,UNBOUNDED"`
	Dir       Token         `tok:"ckw,PRECEDING|ckw,FOLLOWING"`
}

type CurrentRow struct {
	grammar.Seq
	Current grammar.Match `tok:"ckw,CURRENT"`
	Row     grammar.Match `tok:"ckw,ROW"`
}

type ExprBound struct {
	grammar.Seq
	Expr Expr
	Dir  Token `tok:"ckw,PRECEDING|ckw,FOLLOWING"`
}

// FrameExclude ::= EXCLUDE (NO OTHERS | CURRENT ROW | GROUP | TIES)
type FrameExclude struct {
	grammar.Seq
	Exclude grammar.Match `tok:"ckw,EXCLUDE"`
	What    ExcludeWhat
}

type ExcludeWhat struct {
	grammar.OneOf
	NoOthers   *NoOthers
	CurrentRow *CurrentRow
	Other      *Token `tok:"kw,GROUP|ckw,TIES"`
}

type NoOthers struct {
	grammar.Seq
	No     grammar.Match `tok:"ckw,NO"`
	Others grammar.Match `tok:"ckw,OTHERS"`
}

// ---- INSERT ----

// InsertStmt ::= [WithClause] (REPLACE | INSERT [OR action]) INTO QualifiedName [AS Name] [ColumnList]
//
//	(SelectStmt [Upsert] | DEFAULT VALUES) [Returning]
type InsertStmt struct {
	grammar.Seq
	With      *WithClause
	Verb      InsertVerb
	Into      grammar.Match `tok:"kw,INTO"`
	Table     QualifiedName
	Alias     *AsAlias
	Columns   *ColumnList
	Source    InsertSource
	Returning *Returning
}

type InsertVerb struct {
	grammar.OneOf
	Replace *Token `tok:"ckw,REPLACE"`
	Insert  *Insert
}

type Insert struct {
	grammar.Seq
	Insert grammar.Match `tok:"kw,INSERT"`
	Or     *OrAction
}

// OrAction ::= OR (ABORT | FAIL | IGNORE | REPLACE | ROLLBACK)
type OrAction struct {
	grammar.Seq
	Or     grammar.Match `tok:"kw,OR"`
	Action Token         `tok:"ckw,ABORT|ckw,FAIL|ckw,IGNORE|ckw,REPLACE|ckw,ROLLBACK"`
}

type AsAlias struct {
	grammar.Seq
	As   grammar.Match `tok:"kw,AS"`
	Name Name
}

type InsertSource struct {
	grammar.OneOf
	Select  *InsertSelect
	Default *DefaultValues
}

type InsertSelect struct {
	grammar.Seq
	Select SelectStmt
	Upsert *Upsert
}

type DefaultValues struct {
	grammar.Seq
	Default grammar.Match `tok:"kw,DEFAULT"`
	Values  grammar.Match `tok:"kw,VALUES"`
}

// Upsert ::= ON CONFLICT [ColumnList [Where]] DO (NOTHING | UPDATE SetClause [Where])
type Upsert struct {
	grammar.Seq
	On       grammar.Match `tok:"kw,ON"`
	Conflict grammar.Match `tok:"ckw,CONFLICT"`
	Target   *ConflictTarget
	Do       grammar.Match `tok:"ckw,DO"`
	Action   UpsertAction
}

type ConflictTarget struct {
	grammar.Seq
	Columns ColumnList
	Where   *Where
}

type UpsertAction struct {
	grammar.OneOf
	Nothing *Token `tok:"ckw,NOTHING"`
	Update  *DoUpdate
}

type DoUpdate struct {
	grammar.Seq
	Update grammar.Match `tok:"kw,UPDATE"`
	Set    SetClause
	Where  *Where
}

// SetClause ::= SET Assignment, ...
type SetClause struct {
	grammar.Seq
	Set         grammar.Match `tok:"kw,SET"`
	Assignments []Assignment  `sep:"op,," size:"1-"`
}

// Assignment ::= (Name | ColumnList) "=" Expr
type Assignment struct {
	grammar.Seq
	Target AssignTarget
	Eq     grammar.Match `tok:"op,="`
	Expr   Expr
}

type AssignTarget struct {
	grammar.OneOf
	Columns *ColumnList
	Column  *Name
}

// Returning ::= RETURNING ResultColumn, ...
type Returning struct {
	grammar.Seq
	Returning grammar.Match  `tok:"kw,RETURNING"`
	Columns   []ResultColumn `sep:"op,," size:"1-"`
}

// ---- UPDATE ----

// UpdateStmt ::= [WithClause] UPDATE [OrAction] TableRef SetClause [From] [Where] [Returning]
type UpdateStmt struct {
	grammar.Seq
	With      *WithClause
	Update    grammar.Match `tok:"kw,UPDATE"`
	Or        *OrAction
	Table     TableRef
	Set       SetClause
	From      *From
	Where     *Where
	Returning *Returning
}

// ---- DELETE ----

// DeleteStmt ::= [WithClause] DELETE FROM TableRef [Where] [Returning]
type DeleteStmt struct {
	grammar.Seq
	With      *WithClause
	Delete    grammar.Match `tok:"kw,DELETE"`
	From      grammar.Match `tok:"kw,FROM"`
	Table     TableRef
	Where     *Where
	Returning *Returning
}
