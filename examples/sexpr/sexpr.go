package sexpr

import "github.com/arnodel/grammar"

type Token = grammar.SimpleToken

type SExpr struct {
	grammar.OneOf
	Number *Token `tok:"number"`
	String *Token `tok:"string"`
	Atom   *Token `tok:"atom"`
	*List
}

type List struct {
	grammar.Seq
	OpenBkt  grammar.Match `tok:"bkt,("`
	Items    []SExpr
	CloseBkt grammar.Match `tok:"bkt,)"`
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
		Name: "string",
		Ptn:  `"[^"]*"`,
	},
	{
		Name: "number",
		Ptn:  `-?[0-9]+(?:\.[0-9]+)?`,
	},
	{
		Name: "atom",
		Ptn:  `[a-zA-Z_][a-zA-Z0-9_-]*`,
	},
})
