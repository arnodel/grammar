package grammar

import "reflect"

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

// FirstSetOf returns the first set of a field of type T parsed with the given
// token options (see FirstSet).
func FirstSetOf[T any](tokOpts TokenOptions) FirstSet {
	return firstOfField(reflect.TypeFor[T](), tokOpts, firstOfType)
}

// RepeatOptionsFromTags parses the "size", "sep", "trail" and "tok" struct
// tags of a repeated field of items of type T and the "drop" tag of its rule.
func RepeatOptionsFromTags[T any](size, sep, trail, drop, tok string) (RepeatOptions, error) {
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
		Drop:        tokenOptionsFromTagValue(drop),
		First:       FirstSetOf[T](tokenOptionsFromTagValue(tok)),
	}
	if trailMode != TrailNone && len(opts.Sep.TokenParseOptions) == 0 {
		return RepeatOptions{}, errTrailWithoutSep
	}
	return opts, nil
}

// MustRepeatOptions is RepeatOptionsFromTags for package-level variables in
// generated code; it panics on an invalid tag.
func MustRepeatOptions[T any](size, sep, trail, drop, tok string) RepeatOptions {
	opts, err := RepeatOptionsFromTags[T](size, sep, trail, drop, tok)
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

// ParseOptional parses an optional (pointer) field of type T, unless its
// first set says it cannot start with the next token.  On failure it returns
// nil and the error, with the stream restored to where it was.
func ParseOptional[T any](s *ParserState, tokOpts TokenOptions, first FirstSet) (*T, *ParseError) {
	if err := first.check(s); err != nil {
		return nil, err
	}
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

// LeftRecursionOf returns the left recursion information of the alternatives
// of the OneOf rule type T, by field, for Alternatives.Try.
func LeftRecursionOf[T any]() []LeftRecInfo {
	def, err := getRuleDef(reflect.TypeFor[T]())
	if err != nil {
		panic(err)
	}
	if def.leftRec == nil {
		return make([]LeftRecInfo, len(def.Fields))
	}
	return def.leftRec
}

// ParseAlternative parses an alternative of a OneOf rule into a new T, after
// Alternatives.Try said to; on failure the stream is restored.
func ParseAlternative[T any](s *ParserState, a *Alternatives, lr LeftRecInfo, tokOpts TokenOptions) (*T, *ParseError) {
	start := s.Save()
	item := new(T)
	a.Enter(lr)
	err := ParseWithOptions(item, s, tokOpts)
	a.Leave()
	if err != nil {
		s.Restore(start)
		return nil, err
	}
	return item, nil
}

// DirectLeftRecursionOnly reports whether every left-recursive alternative
// of the OneOf rule T is a Seq rule starting with T, which generated code
// parses as a loop; otherwise it falls back to seed growing.
func DirectLeftRecursionOnly[T any]() bool {
	for _, lr := range LeftRecursionOf[T]() {
		if lr.kind == indirectLeftRec {
			return false
		}
	}
	return true
}

// A Dispatch says, for a token, which left-recursive alternatives of a OneOf
// rule may continue the value parsed so far with it, as a bit set by field
// index.
type Dispatch struct {
	byType  map[string]*dispatchEntry
	byValue map[string]uint64 // options with a value and no type
	always  uint64            // alternatives whose continuation may start with anything
	alts    []dispatchAlt
}

type dispatchEntry struct {
	anyValue uint64
	byValue  map[string]uint64
}

type dispatchAlt struct {
	prec    int
	hasPrec bool
	opts    []TokenParseOptions
}

// DispatchOf builds the Dispatch of the OneOf rule type T.
func DispatchOf[T any]() *Dispatch {
	return newDispatch(LeftRecursionOf[T]())
}

func newDispatch(lrs []LeftRecInfo) *Dispatch {
	if len(lrs) > 64 {
		panic("too many alternatives for a Dispatch")
	}
	d := &Dispatch{byType: map[string]*dispatchEntry{}, byValue: map[string]uint64{}}
	for i, lr := range lrs {
		if lr.kind != directLeftRec {
			d.alts = append(d.alts, dispatchAlt{})
			continue
		}
		bit := uint64(1) << i
		d.alts = append(d.alts, dispatchAlt{prec: lr.prec, hasPrec: lr.hasPrec, opts: lr.cont.opts})
		if lr.cont.any {
			d.always |= bit
			continue
		}
		for _, o := range append(append([]TokenParseOptions{}, lr.cont.opts...), lr.cont.drop...) {
			switch {
			case o.TokenType == "" && o.TokenValue == "":
				d.always |= bit
			case o.TokenType == "":
				d.byValue[o.TokenValue] |= bit
			default:
				e := d.byType[o.TokenType]
				if e == nil {
					e = &dispatchEntry{byValue: map[string]uint64{}}
					d.byType[o.TokenType] = e
				}
				if o.TokenValue == "" {
					e.anyValue |= bit
				} else {
					e.byValue[o.TokenValue] |= bit
				}
			}
		}
	}
	return d
}

// Lookup returns the alternatives that may continue with tok, those whose
// precedence is above level.
func (d *Dispatch) Lookup(tok Token, level int) uint64 {
	mask := d.always | d.byValue[tok.Value()]
	if e := d.byType[tok.Type()]; e != nil {
		mask |= e.anyValue | e.byValue[tok.Value()]
	}
	for i, a := range d.alts {
		if a.hasPrec && level >= a.prec {
			mask &^= uint64(1) << i
		}
	}
	return mask
}

// Expect records the continuations of the alternatives above level as
// expected at the current position, when none applied.
func (d *Dispatch) Expect(s *ParserState, level int) *ParseError {
	pos := s.Save()
	if s.beyondRecord(pos) {
		return &s.fail
	}
	tok := s.Next()
	s.Restore(pos)
	var err *ParseError
	for _, a := range d.alts {
		if a.opts == nil || a.hasPrec && level >= a.prec {
			continue
		}
		err = s.recordFailure(pos, tok, a.opts, nil)
	}
	return err
}

// RestParser is implemented by generated Seq rules: ParseRest parses the
// fields after the first, which is already set (the left operand), under
// the pending precedence.
type RestParser interface {
	ParseRest(s *ParserState, pending PendingPrec) *ParseError
}

// PlainAlternatives reports whether no alternative of the OneOf rule T is
// left recursive or has a precedence, so that generated code can try them
// without the Alternatives bookkeeping.
func PlainAlternatives[T any]() bool {
	for _, lr := range LeftRecursionOf[T]() {
		if lr.kind != notLeftRec || lr.hasPrec {
			return false
		}
	}
	return true
}
