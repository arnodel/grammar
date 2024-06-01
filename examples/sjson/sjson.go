package sjson

import "github.com/arnodel/grammar"

type Token = grammar.SimpleToken

// SJSON stands for Simplified JSON as strings and numbers are simplified.
type SJSON struct {
	grammar.OneOf
	Number  *Token `tok:"number"`
	String  *Token `tok:"string"`
	Boolean *Token `tok:"bool"`
	*List
	*Object
}

type List struct {
	grammar.Seq
	Open  grammar.Match `tok:"op,["`
	Items []SJSON       `sep:"op,,"`
	Close grammar.Match `tok:"op,]"`
}

type Object struct {
	grammar.Seq
	Open  grammar.Match `tok:"op,{"`
	Items []Pair        `sep:"op,,"`
	Close grammar.Match `tok:"op,}"`
}

type Pair struct {
	grammar.Seq
	Key   Token         `tok:"string"`
	Colon grammar.Match `tok:"op,:"`
	Value SJSON
}

var tokenise = grammar.SimpleTokeniser([]grammar.TokenDef{
	{
		// If Name is empty, the token is skipped in the token stream
		Ptn: `\s+`,
	},
	{
		Name: "op",
		Ptn:  `[[\]{},:]`,
	},
	{
		Name: "string",
		Ptn:  `"[^"]*"`,
	},
	{
		Name: "number",
		Ptn:  `-?[0-9]+(?:\.[0-9]+)?`,
	},
	{
		Name: "bool",
		Ptn:  `true|false`,
	},
	{
		Name: "null",
		Ptn:  "null",
	},
})
