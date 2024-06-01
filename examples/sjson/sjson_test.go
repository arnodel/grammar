package sjson

import (
	"os"

	"github.com/arnodel/grammar"
)

func ExampleList() {
	tokenStream, _ := tokenise(`[1, 2, 3]`)
	var json SJSON
	grammar.Parse(&json, tokenStream)
	grammar.PrettyWrite(os.Stdout, json)

	// Output:
	// SJSON {
	//   List: List {
	//     Open: {}
	//     Items: [
	//       SJSON {
	//         Number: {number 1}
	//       }
	//       SJSON {
	//         Number: {number 2}
	//       }
	//       SJSON {
	//         Number: {number 3}
	//       }
	//     ]
	//     Close: {}
	//   }
	// }

}

func ExampleObject() {
	tokenStream, _ := tokenise(`
    {
        "name": "Bob",
        "score": 999,
        "awards": ["fast", "blob"],
        "penalties": []
    }
    `)
	var json SJSON
	grammar.Parse(&json, tokenStream)
	grammar.PrettyWrite(os.Stdout, json)

	// Output:
	// SJSON {
	//   Object: Object {
	//     Open: {}
	//     Items: [
	//       Pair {
	//         Key: {string "name"}
	//         Colon: {}
	//         Value: SJSON {
	//           String: {string "Bob"}
	//         }
	//       }
	//       Pair {
	//         Key: {string "score"}
	//         Colon: {}
	//         Value: SJSON {
	//           Number: {number 999}
	//         }
	//       }
	//       Pair {
	//         Key: {string "awards"}
	//         Colon: {}
	//         Value: SJSON {
	//           List: List {
	//             Open: {}
	//             Items: [
	//               SJSON {
	//                 String: {string "fast"}
	//               }
	//               SJSON {
	//                 String: {string "blob"}
	//               }
	//             ]
	//             Close: {}
	//           }
	//         }
	//       }
	//       Pair {
	//         Key: {string "penalties"}
	//         Colon: {}
	//         Value: SJSON {
	//           List: List {
	//             Open: {}
	//             Close: {}
	//           }
	//         }
	//       }
	//     ]
	//     Close: {}
	//   }
	// }

}
