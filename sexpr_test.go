package grammar_test

import (
	"os"

	"github.com/arnodel/grammar"
)

type Token = grammar.SimpleToken

type SExpr struct {
	grammar.OneOf
	Number *Token `tok:"number"`
	String *Token `tok:"string"`
	Atom   *Token `tok:"atom"`
	*List
}

type List struct {
	grammar.Rule
	OpenBkt  Token `tok:"bkt,("`
	Items    []SExpr
	CloseBkt Token `tok:"bkt,)"`
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

func Example() {
	tokenStream, _ := tokenise(`(cons a (list 123 "c")))`)
	var sexpr SExpr
	grammar.Parse(&sexpr, tokenStream)
	grammar.PrettyWrite(os.Stdout, sexpr)

	// Output:
	// SExpr {
	//   List: List {
	//     OpenBkt: {bkt (}
	//     Items: SExpr {
	//       Atom: {atom cons}
	//     }
	//     Items: SExpr {
	//       Atom: {atom a}
	//     }
	//     Items: SExpr {
	//       List: List {
	//         OpenBkt: {bkt (}
	//         Items: SExpr {
	//           Atom: {atom list}
	//         }
	//         Items: SExpr {
	//           Number: {number 123}
	//         }
	//         Items: SExpr {
	//           String: {string "c"}
	//         }
	//         CloseBkt: {bkt )}
	//       }
	//     }
	//     CloseBkt: {bkt )}
	//   }
	// }
}
