# grammar

A parser generator where rules defined as go structs and code generation is
optional.  The concepts are introduced in the simple example below.

## Rules

Rules can be written as Go structs.  Here is a definition for s-exprs:

```golang

// SExpr ::= Number | String | Atom | List
type SExpr struct {
    grammar.OneOf                // This rule is a "one-of": one of the rules below has to match
    Number *Token `tok:"number"` // A pointer field means an optional match
    String *Token `tok:"string"` // This "tok" tag means only tokens of type "string" will match
    Atom   *Token `tok:"atom"`
    *List                        // All field in a one-of rule must be optional
}

// List ::= "(" [Item] ")"
type List struct {
    grammar.Seq             // This is a Sequence rule, all fields must match in a sequence
    OpenBkt  `tok:"bkt,("`  // This "tok" tag means only "bkt" tokens with value "(" will match
    Items []SExpr
    CloseBkt `tok:"bkt,)`
}
```

## Tokens

This is not quite complete as you need a `Token` type.  You can create your own or if your needs are simple enough use `grammar.SimpleToken`:

```golang
type Token = grammar.SimpleToken
```

In order to parse your s-exprs into the data structures aboves you also need a tokeniser.  You can make your own tokeniser or you can build one simply with the `grammar.SimpleTokeniser` function:

```golang
var tokenise = grammar.SimpleTokeniser([]grammar.TokenDef{
    {
        // If Name is empty, the token is skipped in the token stream
        Ptn: `\s+` // This is a regular expression (must not contain groups)
    },
    {
        Name: "bkt", // This is the type of the token seen in the "tok" struct tag
        Ptn: `[()]`,
    },
    {
        Name: "string",
        Ptn: `"[^"]*"`,
    },
    {
        Name: "number",
        Ptn: `-?[0-9]+(?:\.[0-9]+)?`,
    },
    {
        Name: "atom",
        Ptn: `[a-zA-Z_][a-zA-Z0-9_-]*`,
    },
})
```

## Parsing

Now putting all this together you can parse an s-expr of your choice:

```golang

tokenStream, _ := tokenise(`(cons a (list 123 "c")))`)
var sexpr SExpr
err := grammar.Parse(&sexpr, tokenStream)
```

Now `sexpr`'s fields have been filled and you explore the syntax tree by
traversing the fields in `sexpr`, e.g. `sexpr.List.Items` is now a slice of
`SExprs`, `sexpr.List.Items[0].Atom` is a `Token` with Value `"cons"` (and type
`atom`).

There is a convenient function to output a rule struct:

```golang
grammar.PrettyWrite(sexpr, os.Stdout)
```

This will output a pretty representation of `sexpr`:
```
SExpr {
  List: List {
    OpenBkt: {bkt (}
    Items: [
      SExpr {
        Atom: {atom cons}
      }
      SExpr {
        Atom: {atom a}
      }
      SExpr {
        List: List {
          OpenBkt: {bkt (}
          Items: [
            SExpr {
              Atom: {atom list}
            }
            SExpr {
              Number: {number 123}
            }
            SExpr {
              String: {string "c"}
            }
          ]
          CloseBkt: {bkt )}
        }
      }
    ]
    CloseBkt: {bkt )}
  }
}
```

## Generating a parser

The above works using reflection, which is fine but can be a little slow if you
are parsing very big files.  It is also possible to compile a parser.

First you need the command that generates the parser.

```sh
go install github.com/arnodel/grammar/cmd/genparse
```

Then the simplest is to add this line to the file containing the grammar you
want to compile:

```golang
//go:generate genparse
```

You can now generate the compiled parser by running `go generate` in your
package.  If your file is called say `grammar.go`, it will generated a file in
the same package called `grammar.compiled.go`.  You can still parse files the
same way as before using `grammar.Parse()` but this will no longer use
reflection!  Note that you can always force reflection to be used by compiling your
program with the `nocompiledgrammar` go compiler tag.
