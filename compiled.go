package grammar

// This file holds the API used by the code that the genparse command
// generates (see cmd/genparse).  Generated Parse methods replace the
// reflection in Seq.Parse, OneOf.Parse and Tok.Parse with straight-line code
// but call back into the helpers below so that the semantics of repeated
// fields, errors and tags are defined in one place.

// TokenOptionsFromTag parses the value of a "tok", "sep" or "drop" struct tag.
func TokenOptionsFromTag(tag string) TokenOptions {
	return tokenOptionsFromTagValue(tag)
}

// SizeOptionsFromTag parses the value of a "size" struct tag.
func SizeOptionsFromTag(tag string) (SizeOptions, error) {
	return sizeOptionsFromTagValue(tag)
}

// TrailModeFromTag parses the value of a "trail" struct tag.
func TrailModeFromTag(tag string) (TrailMode, error) {
	return trailModeFromTagValue(tag)
}

// RepeatOptionsFromTags parses the "size", "sep" and "trail" struct tags of a
// repeated field.
func RepeatOptionsFromTags(size, sep, trail string) (RepeatOptions, error) {
	sizeOpts, err := sizeOptionsFromTagValue(size)
	if err != nil {
		return RepeatOptions{}, err
	}
	trailMode, err := trailModeFromTagValue(trail)
	if err != nil {
		return RepeatOptions{}, err
	}
	opts := RepeatOptions{
		SizeOptions: sizeOpts,
		Sep:         tokenOptionsFromTagValue(sep),
		Trail:       trailMode,
	}
	if trailMode != TrailNone && len(opts.Sep.TokenParseOptions) == 0 {
		return RepeatOptions{}, errTrailWithoutSep
	}
	return opts, nil
}

// MustRepeatOptions is RepeatOptionsFromTags for package-level variables in
// generated code; it panics on an invalid tag.
func MustRepeatOptions(size, sep, trail string) RepeatOptions {
	opts, err := RepeatOptionsFromTags(size, sep, trail)
	if err != nil {
		panic(err)
	}
	return opts
}

// ParseRepeated parses a repeated field of items of type T with the given
// token and repeat options.  See repeatCore for the meaning of the results.
func ParseRepeated[T any](s *ParserState, tokOpts TokenOptions, opts RepeatOptions) (items []T, n int, err *ParseError, ok bool) {
	var item *T
	n, err, ok = repeatCore(s, opts,
		func() *ParseError {
			item = new(T)
			return ParseWithOptions(item, s, tokOpts)
		},
		func() {
			items = append(items, *item)
		})
	return items, n, err, ok
}

// ParseOptional parses an optional (pointer) field of type T.  On failure it
// returns nil and the error, with the stream restored to where it was.
func ParseOptional[T any](s *ParserState, tokOpts TokenOptions) (*T, *ParseError) {
	start := s.Save()
	item := new(T)
	if err := ParseWithOptions(item, s, tokOpts); err != nil {
		s.Restore(start)
		return nil, err
	}
	return item, nil
}

// EmptyMatchError is the error a generated Seq rule returns when none of its
// fields matched anything.
func EmptyMatchError(s *ParserState, ruleName string) *ParseError {
	return emptyMatchError(s, ruleName)
}
