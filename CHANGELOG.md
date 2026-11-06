# Changelog

## Unreleased

### Performance

- A rule is memoised only once it is parsed a second time at the same
  position.  In most grammars few rules ever are (in a SQL grammar, 1.4% of
  memo entries were ever read), so this halves the memory allocated by a
  parse and speeds it up by 10 to 15%, without changing what is parsed;
  left recursion works as before.

## v0.2.0 - 2026-11-05

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
- `drop` tag on a rule, to skip tokens of given types before each field,
  item and separator.
- `PrettyWriter` interface, for types that want to control their pretty
  output.
- The `genparse` command works again and covers all current features;
  generated code shares its semantics with the reflection path through the
  helpers in `compiled.go`.
- `TokenDef.Scan`, a hand-written scanner for a token type, on its own or
  guarded by a pattern.
- `SimpleTokeniser` patterns may contain capturing groups.
- `ParserState.Fail`, for custom rules to report a failure that is not a
  token mismatch.
- `ParserState.Peek`, returning the next token without consuming it.
- First sets: a `OneOf` alternative, an optional field or a repeated item
  that cannot start with the next token is skipped without being allocated
  or parsed.  First sets are computed from the rule definitions; a custom
  rule or a rule that can match nothing is never skipped.  `FirstSet` and
  `FirstSetOf` expose them to generated code.

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
- A separator that failed to match after the last item of a repeated field
  is now listed among the expected tokens of the error.
- A `SimpleTokeniser` pattern matching the empty string looped forever.
- A `SimpleTokeniser` pattern containing `\Q` without `\E` broke the
  tokeniser.
- Parsing from several goroutines was not safe: the rule definition cache
  was a plain map.  Rule definitions and first sets are now built together
  and published to their caches complete.

### Performance

On the json example (116 KB, 2000 objects) tokenising went from 34 ms to
6 ms and parsing from 81 ms to 8 ms with the generated parser (17 ms with
reflection), through:

- `SimpleTokeniser` matching each definition separately, with literal and
  character class patterns matched without a regular expression, and
  allocating tokens in blocks.
- Failures recorded in one furthest-failure record on the parser state
  instead of a `ParseError` allocated and merged per failed alternative.
- A memo table indexed by stream position, with entries allocated in
  blocks, instead of a map.
- First sets, so that alternatives, optional fields and repeated items that
  cannot start with the next token are not allocated or parsed.

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
- `SimpleTokeniser` takes the longest match rather than the first listed
  (ties go to the first listed).  `let` listed before `[a-z]+` now tokenises
  `letter` as one identifier instead of `let` + `ter`.
- `TokenDef.Special func(string) string` is replaced by
  `Scan func(string) int`, returning the length of the token (0 for none).
- `SimpleTokeniser` streams hold `*SimpleToken` values, not `SimpleToken`.
- `SimpleTokeniser` panics when a definition has neither a pattern nor a
  `Scan`, or pushes an unknown mode.
- `TokenOptions.MatchNextToken` takes a `*ParserState` rather than a
  `TokenStream`, as it records the failure.
- `RepeatOptions` has a `First` field and `RuleField.RepeatOptions` takes
  it; `ParseOptional` takes a `FirstSet` and `RepeatOptionsFromTags` and
  `MustRepeatOptions` are generic in the item type and take the `tok` tag,
  to compute it.  Generated code must be regenerated.
- The `*ParseError` returned by a failing rule is shared by all failures of
  the parse and describes the furthest one, not necessarily the rule's own.
  `ParserState.MergeError` is gone; `ParseError.Merge` remains for custom
  rules that allocate their own errors.
- Requires Go 1.23.

## v0.1.0 - 2024-11-17

First tagged release.
