package sjson

import "github.com/arnodel/grammar"

type Number struct {
	grammar.Tok `tok:"number"`
}

type String struct {
	grammar.Tok `tok:"string"`
}

type Boolean struct {
	grammar.Tok `tok:"bool"`
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
	Key   grammar.Tok   `tok:"string"`
	Colon grammar.Match `tok:"op,:"`
	Value SJSON
}

// SJSON stands for Simplified JSON as strings and numbers are simplified.
type SJSON struct {
	grammar.OneOf5[
		Number,
		String,
		Boolean,
		List,
		Object,
	]
}

var _ grammar.Parser = &SJSON{}

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
