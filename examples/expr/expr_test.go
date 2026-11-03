package expr

import (
	"fmt"
	"os"

	"github.com/arnodel/grammar"
)

func Example() {
	tokenStream, _ := tokenise(`1 + 2 + 3`)
	var expr Expr
	err := grammar.Parse(&expr, tokenStream, grammar.WithDefaultLogger)
	if err != nil {
		fmt.Println(err)
	}
	grammar.PrettyWrite(os.Stdout, &expr)

	// Output:
	// SExpr {
	//   List: List {
	//     Items: [
	//       SExpr {
	//         Atom: atom = "cons" Ln 1, Col 2
	//       }
	//       SExpr {
	//         Atom: atom = "a" Ln 1, Col 7
	//       }
	//       SExpr {
	//         List: List {
	//           Items: [
	//             SExpr {
	//               Atom: atom = "list" Ln 1, Col 10
	//             }
	//             SExpr {
	//               Number: number = "123" Ln 1, Col 15
	//             }
	//             SExpr {
	//               String: string = "\"c\"" Ln 1, Col 19
	//             }
	//           ]
	//         }
	//       }
	//     ]
	//   }
	// }
}
