# Changelog

## Unreleased (v0.2.0)

This release reworks the parser.  It is not backward compatible with v0.1.0;
the breaking changes are listed last.

### Added

- Memoisation (packrat parsing), which makes parsing linear for grammars with
  shared prefixes and makes left-recursive rules work, including indirect
  left recursion.  Rules opt in by implementing `Memoisable`; `Seq`, `OneOf`
  and the generic one-of types do.
- Generic one-of rules `OneOf2` to `OneOf6`, to be embedded in a named struct.
- `Tok`, for single-token rule types.
- Line and column positions on tokens (`Position`), reported in errors and by
  `PrettyWrite`.  The EOF token is positioned after the last token.
- `Exhaust` parse option: fail unless all input is consumed.
  `ErrNotExhausted` identifies that error.
- `trail` tag on repeated fields with a separator: `optional` allows a
  trailing separator, `required` makes the separator a terminator.
- `drop` tag on a rule, to skip tokens of given types between fields.
- `PrettyWriter` interface, for types that want to control their pretty
  output.
- The `genparse` command works again and covers all current features;
  generated code shares its semantics with the reflection path through the
  helpers in `compiled.go`.

### Fixed

- A `OneOf` repeated field with a fixed size silently matched nothing.
- `OneOf` repeated fields ignored the `sep` tag.
- A trailing separator was accepted after the last item of any repeated
  field; now it is only accepted with `trail:"optional"`.
- When a repeated field reached its maximum size the following separator was
  consumed.
- Merging two errors at the same position dropped the error message; both
  the message and the expected tokens are now reported.
- Errors at end of input reported "Ln 0, Col 0".

### Breaking changes

- `Token` has a new method, `Position() Position`.  Custom token types must
  implement it.
- `SimpleToken` has a new field, `TokPos`; positional struct literals of it
  no longer compile.
- `PrettyWrite` requires a pointer to the rule.
- `PrettyWrite` output changed: `Match` fields are omitted and tokens are
  printed as `type = "value" Ln l, Col c`.
- `ParseError` messages start with the position instead of the token index.
- Memoisation changes which rules may be left recursive: only memoised ones.
- Requires Go 1.23.
