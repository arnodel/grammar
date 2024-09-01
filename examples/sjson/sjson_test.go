package sjson

import (
	"os"

	"github.com/arnodel/grammar"
)

func ExampleList() {
	tokenStream, _ := tokenise(`[1, 2, 3]`)
	var json SJSON
	grammar.Parse(&json, tokenStream) //, grammar.WithDefaultLogger)
	grammar.PrettyWrite(os.Stdout, &json)

	// Output:
	// List {
	//   Items: [
	//     Number {
	//       Tok: number = "1" Ln 1, Col 2
	//     }
	//     Number {
	//       Tok: number = "2" Ln 1, Col 5
	//     }
	//     Number {
	//       Tok: number = "3" Ln 1, Col 8
	//     }
	//   ]
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
	grammar.PrettyWrite(os.Stdout, &json)

	// Output:
	// Object {
	//   Items: [
	//     Pair {
	//       Key: string = "\"name\"" Ln 3, Col 9
	//       Value: String {
	//         Tok: string = "\"Bob\"" Ln 3, Col 17
	//       }
	//     }
	//     Pair {
	//       Key: string = "\"score\"" Ln 4, Col 9
	//       Value: Number {
	//         Tok: number = "999" Ln 4, Col 18
	//       }
	//     }
	//     Pair {
	//       Key: string = "\"awards\"" Ln 5, Col 9
	//       Value: List {
	//         Items: [
	//           String {
	//             Tok: string = "\"fast\"" Ln 5, Col 20
	//           }
	//           String {
	//             Tok: string = "\"blob\"" Ln 5, Col 28
	//           }
	//         ]
	//       }
	//     }
	//     Pair {
	//       Key: string = "\"penalties\"" Ln 6, Col 9
	//       Value: List {
	//       }
	//     }
	//   ]
	// }
}
