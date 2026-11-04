package expr

import (
	"fmt"
	"os"

	"github.com/arnodel/grammar"
)

func Example() {
	tokenStream, _ := tokenise(`1 + 2 + 3`)
	var expr Expr
	err := grammar.Parse(&expr, tokenStream, grammar.Exhaust)
	if err != nil {
		fmt.Println(err)
	}
	grammar.PrettyWrite(os.Stdout, &expr)

	// Output:
	// Expr {
	//   Sum: Sum {
	//     Lhs: Expr {
	//       Sum: Sum {
	//         Lhs: Expr {
	//           Num: Num {
	//             Tok: number = "1" Ln 1, Col 1
	//           }
	//         }
	//         Rhs: Num {
	//           Tok: number = "2" Ln 1, Col 5
	//         }
	//       }
	//     }
	//     Rhs: Num {
	//       Tok: number = "3" Ln 1, Col 9
	//     }
	//   }
	// }
}

func ExampleExhaust() {
	// Without Exhaust this would succeed, parsing just `1 + 2`.
	tokenStream, _ := tokenise(`1 + 2 3`)
	var expr Expr
	fmt.Println(grammar.Parse(&expr, tokenStream, grammar.Exhaust))

	// Here the parser got further than the leftover input while looking for
	// the right-hand side of the second `+`, so that is what it reports.
	tokenStream, _ = tokenise(`1 + 2 +`)
	fmt.Println(grammar.Parse(&expr, tokenStream, grammar.Exhaust))

	// Output:
	// Ln 1, Col 7: number with value "3": expected token with value "+"; expected end of input
	// Ln 1, Col 8: EOF with value "EOF": expected token with type number
}
