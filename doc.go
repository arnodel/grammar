// Package grammar parses token streams with rules written as Go structs.
//
// The implementation, by concept and file:
//
//   - ruledef.go: a rule type and its struct tags, read into a RuleDef.
//   - first.go: the first set of a rule (the tokens it can start with),
//     computed from the definitions so that alternatives and optional fields
//     that cannot start with the next token are skipped without parsing.
//   - alternative.go: what is known about each alternative of a one-of rule
//     before parsing (first set, left recursion, precedence).
//   - rule.go: parsing a Seq rule, its fields and repetitions, and Tok rules.
//   - oneof.go: parsing a one-of rule, by precedence climbing when it can.
//   - sum.go, generic_oneof.go: Sum[T] and the OneOfN rules, one-of rules
//     whose value is a single pointer.
//   - prec.go: operator precedence, and how levels pass between rules.
//   - memo.go: memoisation (packrat parsing) and left recursion by seed
//     growing.
//   - failure.go: ParseError and how the furthest failure is recorded.
//   - parse.go: the entry points, ParserState and the interfaces rules and
//     tokens implement.
//   - compiled.go: the API of the code that cmd/genparse generates.
//   - lexer.go: a simple tokeniser.
package grammar
