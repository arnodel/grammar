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
	// List {
	//   Items: [
	//     Number {
	//       Tok: number = "1"
	//     }
	//     Number {
	//       Tok: number = "2"
	//     }
	//     Number {
	//       Tok: number = "3"
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
	grammar.PrettyWrite(os.Stdout, json)

	// Output:
	// Object {
	//   Items: [
	//     Pair {
	//       Key: string = "\"name\""
	//       Value: String {
	//         Tok: string = "\"Bob\""
	//       }
	//     }
	//     Pair {
	//       Key: string = "\"score\""
	//       Value: Number {
	//         Tok: number = "999"
	//       }
	//     }
	//     Pair {
	//       Key: string = "\"awards\""
	//       Value: List {
	//         Items: [
	//           String {
	//             Tok: string = "\"fast\""
	//           }
	//           String {
	//             Tok: string = "\"blob\""
	//           }
	//         ]
	//       }
	//     }
	//     Pair {
	//       Key: string = "\"penalties\""
	//       Value: List {
	//       }
	//     }
	//   ]
	// }
}
