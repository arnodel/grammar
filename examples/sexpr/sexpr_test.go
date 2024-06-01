package sexpr

import (
	"os"

	"github.com/arnodel/grammar"
)

func Example() {
	tokenStream, _ := tokenise(`(cons a (list 123 "c")))`)
	var sexpr SExpr
	grammar.Parse(&sexpr, tokenStream)
	grammar.PrettyWrite(os.Stdout, sexpr)

	// Output:
	// SExpr {
	//   List: List {
	//     OpenBkt: {}
	//     Items: [
	//       SExpr {
	//         Atom: {atom cons}
	//       }
	//       SExpr {
	//         Atom: {atom a}
	//       }
	//       SExpr {
	//         List: List {
	//           OpenBkt: {}
	//           Items: [
	//             SExpr {
	//               Atom: {atom list}
	//             }
	//             SExpr {
	//               Number: {number 123}
	//             }
	//             SExpr {
	//               String: {string "c"}
	//             }
	//           ]
	//           CloseBkt: {}
	//         }
	//       }
	//     ]
	//     CloseBkt: {}
	//   }
	// }
}
