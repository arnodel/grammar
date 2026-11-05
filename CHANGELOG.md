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
- `SimpleTokeniser` tokenised at about 3 MB/s, with several allocations per
  token; it is now several times faster and allocates once per regular
  expression match, none for literal and character class patterns.
- A `SimpleTokeniser` pattern matching the empty string looped forever.
- Failures are recorded in one furthest-failure record on the parser state
  instead of allocating and merging a `ParseError` per failed alternative,
  which halves the parsing time of the json example.
- A separator that failed to match after the last item of a repeated field
  is now listed among the expected tokens of the error.
- The memo table is a slice of short lists indexed by stream position, with
  entries allocated in blocks, instead of a map allocating one entry per
  memoised parse; together with the previous item this makes the json
  example parse about seven times faster than before the two changes.

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
- The `*ParseError` returned by a failing rule is shared by all failures of
  the parse and describes the furthest one, not necessarily the rule's own.
  `ParserState.MergeError` is gone; `ParseError.Merge` remains for custom
  rules that allocate their own errors.
- Requires Go 1.23.
