package expr

import "github.com/arnodel/grammar"

//go:generate go run github.com/arnodel/grammar/cmd/genparse

type Token = grammar.SimpleToken

type Expr struct {
	grammar.OneOf
	Sum *Sum
	// Diff *Diff
	Num *Num
}

type Sum struct {
	grammar.Seq
	Lhs  Expr
	Plus grammar.Match `tok:"op,+"`
	Rhs  Num
}

// type Diff struct {
// 	grammar.Seq
// 	Lhs   Expr
// 	Minus grammar.Match `tok:"op,-"`
// 	Rhs   Num
// }

type Num struct {
	grammar.Tok `tok:"number"`
}

var tokenise = grammar.SimpleTokeniser([]grammar.TokenDef{
	{
		// If Name is empty, the token is skipped in the token stream
		Ptn: `\s+`,
	},
	{
		Name: "bkt",
		Ptn:  `[()]`,
	},
	{
		Name: "op",
		Ptn:  `[+-]`,
	},
	{
		Name: "number",
		Ptn:  `-?[0-9]+(?:\.[0-9]+)?`,
	},
})
