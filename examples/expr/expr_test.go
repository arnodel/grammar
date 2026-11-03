package expr

import (
	"fmt"
	"os"

	"github.com/arnodel/grammar"
)

func Example() {
	tokenStream, _ := tokenise(`1 + 2 + 3`)
	var expr Expr
	err := grammar.Parse(&expr, tokenStream)
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
