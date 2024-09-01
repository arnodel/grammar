package sexpr

import (
	"os"

	"github.com/arnodel/grammar"
)

func Example() {
	tokenStream, _ := tokenise(`(cons a (list 123 "c")))`)
	var sexpr SExpr
	grammar.Parse(&sexpr, tokenStream)
	grammar.PrettyWrite(os.Stdout, &sexpr)

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
