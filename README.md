# grammar

A parser generator where rules defined as go structs and code generation is
optional.  The concepts are introduced in the simple example below.  There are
also examples in the [examples/](./examples/) directory, the most interesting
being [examples/json](./examples/json/).

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
    grammar.Seq                           // This is a Sequence rule, all fields must match in a sequence
    OpenBkt  grammar.Match `tok:"bkt,("`  // This "tok" tag means only "bkt" tokens with value "(" will match
    Items    []SExpr
    CloseBkt grammar.Match `tok:"bkt,)`
}
```

The `grammar.Match` type above is an empty struct, so it takes no space in the
structure, but it only matches the token specification in the `tok` tag.

## Tokens

This is not quite complete as you need a `Token` type.  You can create your own
or if your needs are simple enough use `grammar.SimpleToken`:

```golang
type Token = grammar.SimpleToken
```

In order to parse your s-exprs into the data structures aboves you also need a
tokeniser.  You can make your own tokeniser or you can build one simply with the
`grammar.SimpleTokeniser` function:

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
    OpenBkt: {}
    Items: [
      SExpr {
        Atom: {atom cons}
      }
      SExpr {
        Atom: {atom a}
      }
      SExpr {
        List: List {
          OpenBkt: {}
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
          CloseBkt: {}
        }
      }
    ]
    CloseBkt: {}
  }
}
```

## Generating a parser

The above works using reflection.  It is also possible to generate, for each
rule type, a `Parse` method that does the same thing without reflection.
The generated methods call back into the grammar package for the logic of
repeated fields, errors and tags, so they behave exactly like the reflection
based ones; they are just a little faster (around 10-15% on the JSON example)
and allocate less.

Add this line to the file containing the grammar you want to compile:

```golang
//go:generate go run github.com/arnodel/grammar/cmd/genparse
```

Then run `go generate` in your package.  If your file is called say
`grammar.go`, this generates a file in the same package called
`grammar.compiled.go`, with a `Parse` method for every struct type whose first
field is `grammar.Seq`, `grammar.OneOf` or `grammar.Tok`.  You parse files the
same way as before with `grammar.Parse()`, which now runs the generated code.
The generic `OneOfN` types need nothing generated as they never used
reflection.

The generated file has the build constraint `!nocompiledgrammar`, so you can
always go back to reflection by building with `-tags nocompiledgrammar`.  This
is also a convenient way to check that the two agree: run your tests with and
without the tag.

The generator only looks at one file, so a rule whose fields refer to rule
types in other files of the package is fine, but each file holding rule types
needs its own `go:generate` line.
