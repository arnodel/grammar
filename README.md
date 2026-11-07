# grammar

[![CI](https://github.com/arnodel/grammar/actions/workflows/ci.yml/badge.svg)](https://github.com/arnodel/grammar/actions/workflows/ci.yml)

A parser library for Go where the rules are Go structs and the parse tree is
those same structs, filled in.  There is no separate grammar file or DSL: the
type you want to get out of the parser *is* the grammar.  Parsing uses
reflection by default; a generator can produce equivalent reflection-free
code if you want it.

The parser is a memoising ("packrat") recursive descent parser with
[PEG](https://en.wikipedia.org/wiki/Parsing_expression_grammar) semantics:
ordered choice, greedy repetition, and left recursion supported.

The concepts are introduced in the example below.  There are more examples in
[examples/](./examples/): [json](./examples/json/) is a good first one,
[sjson](./examples/sjson/) shows the generic one-of types,
[expr](./examples/expr/) shows a left-recursive expression grammar and
[sql](./examples/sql/) is a parser for a SQLite-like dialect of SQL, large
enough to show how the pieces fit together on a real language and how to
deal with a language that has several dialects.

## Rules

Rules are Go structs whose first field says what kind of rule it is.  Here is
a definition for s-expressions:

```golang
import "github.com/arnodel/grammar"

type Token = grammar.SimpleToken

// SExpr ::= Number | String | Atom | List
type SExpr struct {
    grammar.OneOf                // A "one-of" rule: exactly one of the fields below matches
    Number *Token `tok:"number"` // A pointer field is an alternative; this one matches a "number" token
    String *Token `tok:"string"`
    Atom   *Token `tok:"atom"`
    *List                        // An embedded pointer works too, the field is named List
}

// List ::= "(" SExpr* ")"
type List struct {
    grammar.Seq                           // A sequence rule: all fields match in order
    OpenBkt  grammar.Match `tok:"bkt,("`  // Matches a "bkt" token with value "(" (and stores nothing)
    Items    []SExpr                      // A slice field matches zero or more items
    CloseBkt grammar.Match `tok:"bkt,)"`
}
```

There are three kinds of rule, chosen by the first (embedded) field:

- `grammar.Seq`: every field must match, in order.
- `grammar.OneOf`: the first field that matches wins (ordered choice).  All
  fields must be pointers or slices.
- `grammar.Tok`: the rule matches a single token; the struct has no other
  field and the `tok` tag goes on the `grammar.Tok` field itself.  See
  [Tokens](#tokens) below.

Within a rule, the type of a field says how it matches:

- A plain field (`Items SExpr`) must match.
- A pointer field (`Number *Token`) is optional in a `Seq`, and one
  alternative in a `OneOf`.
- A slice field (`Items []SExpr`) matches a run of items.  In a `OneOf`, the
  alternative matches if there is at least one item.

Field types can be other rule types, `grammar.Match`, your token type, or
anything implementing `grammar.Parser`.  `grammar.Match` is an empty struct:
it matches the token given by its `tok` tag and takes no space in the result.
`grammar.Empty` always matches and consumes nothing.

### Tags

| tag | on | meaning |
|---|---|---|
| `tok:"type"` | a token field | match a token of this type |
| `tok:"type,value"` | a token field | match a token of this type with this value |
| `tok:"a\|b,x"` | a token field | alternatives, separated by `\|` |
| `tok:"type*"` | a token field | a `*` after the type matches without consuming the token (lookahead) |
| `sep:"type,value"` | a slice field | items are separated by this token |
| `trail:"optional"` | a slice field with `sep` | a separator after the last item is allowed (`1, 2,`) |
| `trail:"required"` | a slice field with `sep` | every item must be followed by the separator (`1; 2;`) |
| `size:"n"`, `size:"min-max"`, `size:"min-"`, `size:"-max"` | a slice field | bounds on the number of items |
| `drop:"type"` | the `Seq` or `OneOf` field | tokens of this type are skipped before each field, item and separator of the rule (e.g. whitespace or comments that the tokeniser keeps) |
| `prec:"n"` | the `Seq` field of an operator rule, or the `OneOf` field of a group of them | the precedence of the operator, see [Operator precedence](#operator-precedence) |

A slice with no `size` matches zero or more items.  A separator is matched
only between items unless a `trail` mode says otherwise.

### Generic one-of rules

For a rule that is purely a choice between other rules, the generic types
`grammar.OneOf2` to `grammar.OneOf6` avoid the wrapper struct.  Embed one in
a named struct (a type alias would not let you define methods on it):

```golang
type Number struct {
    grammar.Tok `tok:"number"`
}

type SJSON struct {
    grammar.OneOf5[Number, String, Boolean, List, Object]
}
```

After parsing, `Value` holds a pointer to the alternative that matched, so it
is consumed with a type switch:

```golang
switch v := sjson.Value.(type) {
case *Number:
    ...
case *List:
    ...
}
```

The type parameters are unconstrained (`any`) because Go cannot express
"pointer to T implements Parser" without doubling them; an argument that is
not a rule type is reported when the rule is first parsed.

`grammar.Sum[T]` gives the same compact value for a `OneOf` rule type `T`,
which then serves only as the declaration of the alternatives, with their
tags, in the usual form.  This suits a choice with many alternatives, or
one you want to group:

```golang
type Expr struct {
    grammar.Sum[struct {
        grammar.OneOf
        Sum  *Sum
        Neg  *Neg
        Atom *Atom
    }]
}
```

`expr.Value` is then a `*Sum`, a `*Neg` or an `*Atom`.  The declaration may
be a named type too, if the wide form is wanted somewhere.

## Tokens

The parser consumes a `grammar.TokenStream`, whose tokens implement
`grammar.Token`: a type, a value and a position.  You can write your own
token and stream types, or use `grammar.SimpleToken` and build a tokeniser
with `grammar.SimpleTokeniser`:

```golang
var tokenise = grammar.SimpleTokeniser([]grammar.TokenDef{
    {
        // If Name is empty, the token is skipped in the token stream
        Ptn: `\s+`, // A regular expression, matched at the start of the input
    },
    {
        Name: "bkt", // The token type, as seen in "tok" struct tags
        Ptn:  `[()]`,
    },
    {
        Name: "string",
        Ptn:  `"[^"]*"`,
    },
    {
        Name: "number",
        Ptn:  `-?[0-9]+(?:\.[0-9]+)?`,
    },
    {
        Name: "atom",
        Ptn:  `[a-zA-Z_][a-zA-Z0-9_-]*`,
    },
})
```

At each point in the input the definition with the longest match wins, and
among equal matches the first one listed, so keywords can be listed before
an identifier pattern without clashing with longer identifiers.  Literal
patterns and character classes are matched without running a regular
expression.

A `Scan func(string) int` on a definition takes over from the pattern (or
complements it: when both are given the pattern decides whether `Scan` is
tried, and `Scan` decides the length of the token).  It returns the length
of the token at the start of the remaining input, or 0, and covers what a
regular expression cannot, like nested comments or strings with escapes:

```golang
    {
        Name: "comment",
        Ptn:  `/\*`,
        Scan: scanNestedComment, // func(string) int
    },
```

Definitions can be grouped into modes (`Mode`), entered and left with
`PushMode` and `PopMode` on the tokens that switch context, e.g. for string
interpolation.

`grammar.SimpleToken` is also a `Parser`, so a field of that type matches a
token and stores it.  `grammar.Tok` wraps it for use as the first field of a
single-token rule type, which is handy with the generic one-of types.

## Parsing

```golang
tokenStream, _ := tokenise(`(cons a (list 123 "c"))`)
var sexpr SExpr
err := grammar.Parse(&sexpr, tokenStream, grammar.Exhaust)
```

`sexpr` is now filled in: `sexpr.List.Items` is a slice of `SExpr`, and
`sexpr.List.Items[0].Atom` is a `Token` of type `atom` with value `cons`.

`grammar.Exhaust` makes the parse fail unless all the input was consumed;
without it a rule may match just a prefix.  The error is a `*grammar.ParseError`
with the offending token, its position and what was expected there:

```
Ln 1, Col 7: number with value "3": expected token with value "+"; expected end of input
```

`grammar.WithLogger(logger)` traces the parse, which helps when a grammar does
not do what you expect.

To look at a parse tree, `grammar.PrettyWrite` writes a readable form of it:

```golang
grammar.PrettyWrite(os.Stdout, &sexpr)
```

```
SExpr {
  List: List {
    Items: [
      SExpr {
        Atom: atom = "cons" Ln 1, Col 2
      }
      SExpr {
        Atom: atom = "a" Ln 1, Col 7
      }
      SExpr {
        List: List {
          Items: [
            SExpr {
              Atom: atom = "list" Ln 1, Col 10
            }
            SExpr {
              Number: number = "123" Ln 1, Col 15
            }
            SExpr {
              String: string = "\"c\"" Ln 1, Col 19
            }
          ]
        }
      }
    ]
  }
}
```

Empty optional fields, empty slices and `Match` fields are omitted.

## Semantics

The parser works like a PEG parser, which differs from a context-free grammar
in two ways that matter when you write rules:

- **Choice is ordered.**  A `OneOf` takes the first alternative that matches,
  not the longest.  Put the more specific alternative first.
- **Repetition is greedy and does not backtrack.**  A slice field takes as
  many items as it can; if what follows then fails to match, the parser does
  not try again with fewer items.

Results are memoised by rule type and position, so parsing is linear in the
size of the input for grammars whose alternatives share prefixes.  A rule is
memoised once it is parsed a second time at the same place (most rules never
are, and memoising them would cost more than it saves), so the same sub-rule
is parsed at most twice at the same place.

A `OneOf` does not try an alternative that cannot start with the next token,
nor does a `Seq` try an optional field or a repeated item that cannot: the
tokens a rule can start with (its first set) are computed from the struct
tags.  This changes nothing to what is parsed or to error messages, it only
saves work.

### Left recursion

Thanks to memoisation, rules may be left recursive, which is the natural way
to write left-associative operators:

```golang
// Expr ::= Expr "+" Num | Num
type Expr struct {
    grammar.OneOf
    Sum *Sum
    Num *Num
}

type Sum struct {
    grammar.Seq
    Lhs  Expr
    Plus grammar.Match `tok:"op,+"`
    Rhs  Num
}

type Num struct {
    grammar.Tok `tok:"number"`
}
```

Parsing `1 + 2 + 3` gives `Sum{Sum{1, 2}, 3}`.  Indirect left recursion
(through several rules) works too.

### Operator precedence

With several operators, left recursion alone gives every operator the same
precedence.  A `prec` tag on the `Seq` field of an operator rule gives it a
level, a positive integer: among the alternatives of a `OneOf` (or `Sum`), a
rule with a level only applies when the level in effect is lower, and its
operands after the first are parsed under its own level.  Higher binds
tighter.

```golang
type Expr struct {
    grammar.Sum[struct {
        grammar.OneOf
        Add  *Add
        Mul  *Mul
        Neg  *Neg
        Atom *Atom
    }]
}

// Add ::= Expr ("+" | "-") Expr
type Add struct {
    grammar.Seq `prec:"1"`
    Lhs Expr
    Op  grammar.Tok `tok:"op,+|op,-"`
    Rhs Expr
}

// Mul ::= Expr ("*" | "/") Expr
type Mul struct {
    grammar.Seq `prec:"2"`
    Lhs Expr
    Op  grammar.Tok `tok:"op,*|op,/"`
    Rhs Expr
}

// Neg ::= "-" Expr, a prefix operator
type Neg struct {
    grammar.Seq `prec:"3"`
    Minus grammar.Match `tok:"op,-"`
    X     Expr
}

type Atom struct {
    grammar.OneOf
    Num   *Num
    Paren *Paren // "(" Expr ")"
}
```

`1 + 2 * 3` parses as `Add{1, Mul{2, 3}}`: the `Rhs` of `Add` is parsed under
level 1, where `Mul` (level 2) applies but another `Add` does not, which also
makes the operators left associative.  `-1 * 2` is `Mul{Neg{1}, 2}`: a prefix
operator is not left recursive, so its level only binds its operand.  The
level is consumed by the first rule entered, so the expression inside
`Paren` starts afresh, as do function arguments or anything else bracketed.

Operators may be grouped, as alternatives of a `Sum` within the `Sum` (the
groups are flattened into it for parsing and wrapped back in the tree), and
a level may be given to a group instead of each of its members, with a
`prec` tag on the `OneOf` field of the group's declaration (or on an
embedded `OneOfN`); a group member may still state its own.  A flat list
is written as usual, `First Expr` then `Rest []Term`, with the level on the
term rule too since it binds the term's operand.  A level set by an operator
of `Expr` only constrains `Expr`: a language with separate value and type
expressions can give each its own table.

An operator rule and the sum it belongs to are parsed by precedence climbing
rather than by seed growing, so this is also the fast way to write
expressions.  See [examples/sql](./examples/sql/expr.go) for a full
expression grammar in this style, with eleven levels, prefix and postfix
operators and mixfix predicates like `BETWEEN`.

## Custom rules

Any type with a `Parse(r any, s *grammar.ParserState, opts grammar.TokenOptions) *grammar.ParseError`
method is a rule.  `opts` holds the `tok` tag of the field being parsed.  If
the result of `Parse` depends only on the rule type, the position and the
options, the type can also implement `grammar.Memoisable` to have its results
cached:

```golang
func (w *Word) MemoKey(opts grammar.TokenOptions) any {
    return opts.Key()
}
```

Rules that do not implement it are parsed afresh every time, which is the
right default for cheap token-like rules.

A rule reports a token mismatch by returning the error from
`opts.MatchNextToken(s)`, and any other failure with `s.Fail`; `s.Peek()`
returns the next token without consuming it:

```golang
func (n *EvenNumber) Parse(r any, s *grammar.ParserState, opts grammar.TokenOptions) *grammar.ParseError {
    pos := s.Save()
    tok, err := opts.MatchNextToken(s)
    if err != nil {
        return err
    }
    if !isEven(tok.Value()) {
        return s.Fail(pos, tok, errors.New("odd number"))
    }
    n.Value = tok.Value()
    return nil
}
```

The parser state keeps a record of the furthest failure so far, with every
reason given for it, and that is what `grammar.Parse` reports: the furthest
point reached is where the input went wrong.  Both functions above record
into it and return a pointer to it, so no error is allocated while parsing,
and the error a rule gets back from a sub-rule only means "failed"; it does
not necessarily describe that sub-rule's own failure.

The first set of a custom rule is unknown, so it is always tried (see
[Semantics](#semantics)).  Putting a `Match` on a keyword or other token
before a custom rule in a `Seq` lets the sequence as a whole be skipped.

## Generating a parser

The above works using reflection.  It is also possible to generate, for each
rule type, a `Parse` method that does the same thing without reflection.
The generated methods call back into the grammar package for the logic of
repeated fields, errors and tags, so they behave exactly like the reflection
based ones; they are about twice as fast on the JSON example and allocate
half as much.

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
