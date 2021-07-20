package json

import (
	"github.com/arnodel/grammar"
)

var TokeniseJsonString = grammar.SimpleTokeniser([]grammar.TokenDef{
	{
		Ptn: `\s+`,
	},
	{
		Name: "null",
		Ptn:  `null`,
	},
	{
		Name: "bool",
		Ptn:  `true|false`,
	},
	{
		Name: "op",
		Ptn:  `[{},:[\]]`,
	},
	{
		Name: "string",
		Ptn:  `"[^"]*"`,
	},
	{
		Name: "number",
		Ptn:  `-?[0-9]+(?:\.[0-9]+)?`,
	},
})
