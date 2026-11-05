package grammar

import (
	"fmt"
	"io"
	"reflect"
)

type Match struct{}

var _ Parser = Match{}
var _ PrettyWriter = Match{}

func (Match) Parse(r interface{}, s *ParserState, opts TokenOptions) *ParseError {
	_, err := opts.MatchNextToken(s)
	return err
}

func (Match) PrettyWrite(out io.Writer, r any, pfx string, indent int) (err error) {
	return nil
}

type Empty struct{}

var _ Parser = Empty{}
var _ PrettyWriter = Empty{}

func (Empty) Parse(r interface{}, s *ParserState, opts TokenOptions) *ParseError {
	return nil
}

func (Empty) PrettyWrite(out io.Writer, r any, pfx string, indent int) (err error) {
	return nil
}

// OneOf should be used as the first field of a Rule struct to signify that it
// should match exactly one of the fields
type OneOf struct{}

var _ Parser = &OneOf{}
var _ Memoisable = OneOf{}

// MemoKey implements Memoisable: a OneOf rule ignores the token options.
func (OneOf) MemoKey(opts TokenOptions) any {
	return nil
}

func (OneOf) Parse(r interface{}, s *ParserState, opts TokenOptions) *ParseError {
	ruleDef, elem := getRuleDefAndValue(r)
	var err, fieldErr *ParseError
	ruleDef.DropOptions.DropMatchingNextTokens(s)
	for _, ruleField := range ruleDef.Fields {
		switch {
		case ruleField.Pointer:
			{
				start := s.Save()
				fieldPtrV := reflect.New(ruleField.BaseType)
				fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.TokenOptions)
				if fieldErr == nil {
					elem.Field(ruleField.Index).Set(fieldPtrV)
					return nil
				}
				s.Restore(start)
				err = err.Merge(fieldErr)
			}
		case ruleField.Array:
			itemsV, n, arrErr, ok := parseRepeated(ruleField, ruleDef.DropOptions, s)
			err = err.Merge(arrErr)
			if ok && n > 0 {
				elem.Field(ruleField.Index).Set(itemsV)
				return nil
			}
			// Nothing matched (or too few items): the stream is back where
			// it was, try the next alternative.
		default:
			panic("should not get here")
		}
	}
	return err
}

// Seq should be used as the first field of a Rule struct to signify that it
// should match all the fields in sequence.
type Seq struct{}

var _ Parser = &Seq{}
var _ Memoisable = Seq{}

// MemoKey implements Memoisable: a Seq rule ignores the token options.
func (Seq) MemoKey(opts TokenOptions) any {
	return nil
}

func (Seq) Parse(r interface{}, s *ParserState, opts TokenOptions) *ParseError {
	ruleDef, elem := getRuleDefAndValue(r)
	var err, fieldErr *ParseError
	itemCount := 0
	var fieldPtrV reflect.Value
	dropOptions := ruleDef.DropOptions
	for _, ruleField := range ruleDef.Fields {
		if s.Debug() {
			s.Logf("  .%s tok #%d", ruleField.Name, s.Save())
		}
		dropOptions.DropMatchingNextTokens(s)
		switch {
		case ruleField.Pointer:
			{
				start := s.Save()
				fieldPtrV = reflect.New(ruleField.BaseType)
				fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.TokenOptions)
				if fieldErr != nil {
					err = err.Merge(fieldErr)
					s.Restore(start)
				} else {
					elem.Field(ruleField.Index).Set(fieldPtrV)
					itemCount++
				}
			}
		case ruleField.Array:
			itemsV, n, arrErr, ok := parseRepeated(ruleField, ruleDef.DropOptions, s)
			err = err.Merge(arrErr)
			if !ok {
				return err
			}
			elem.Field(ruleField.Index).Set(itemsV)
			itemCount += n
		default:
			fieldPtrV = reflect.New(ruleField.BaseType)
			fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.TokenOptions)
			if fieldErr != nil {
				return err.Merge(fieldErr)
			}
			elem.Field(ruleField.Index).Set(fieldPtrV.Elem())
			itemCount++
		}
	}
	if itemCount == 0 {
		return emptyMatchError(s, ruleDef.Name)
	}
	return nil
}

// parseRepeated parses the items of a repeated (slice) field by reflection,
// using repeatCore for the logic.  It returns the slice of items, how many
// there are, the error that stopped the repetition (nil if it stopped because
// Max was reached) and whether at least Min items were found.
func parseRepeated(ruleField RuleField, drop TokenOptions, s *ParserState) (itemsV reflect.Value, n int, err *ParseError, ok bool) {
	itemsV = reflect.Zero(reflect.SliceOf(ruleField.BaseType))
	var itemPtrV reflect.Value
	n, err, ok = repeatCore(s, ruleField.RepeatOptions(drop),
		func() *ParseError {
			itemPtrV = reflect.New(ruleField.BaseType)
			return ParseWithOptions(itemPtrV.Interface(), s, ruleField.TokenOptions)
		},
		func() {
			itemsV = reflect.Append(itemsV, itemPtrV.Elem())
		})
	return itemsV, n, err, ok
}

// repeatCore drives the parsing of a repeated field, honouring its size, sep,
// trail and drop options.  parseItem must parse one item at the current
// position and keep is called for each item that is retained.  It returns how many
// items were kept, the error that stopped the repetition (nil if it stopped
// because Max was reached) and whether at least Min items were found.  When
// ok is false the stream is restored to where the field started; otherwise it
// is left after the last item (and after its separator, if the trail mode
// allows one).
func repeatCore(s *ParserState, opts RepeatOptions, parseItem func() *ParseError, keep func()) (n int, err *ParseError, ok bool) {
	fieldStart := s.Save()
	hasSep := len(opts.Sep.TokenParseOptions) > 0
	for opts.Max == 0 || n < opts.Max {
		itemStart := s.Save()
		opts.Drop.DropMatchingNextTokens(s)
		if n > 0 && hasSep && opts.Trail != TrailRequired {
			// A separator before every item but the first.
			if _, sepErr := opts.Sep.MatchNextToken(s); sepErr != nil {
				err = err.Merge(sepErr)
				s.Restore(itemStart)
				break
			}
			opts.Drop.DropMatchingNextTokens(s)
		}
		afterSep := s.Save()
		if itemErr := parseItem(); itemErr != nil {
			err = err.Merge(itemErr)
			if opts.Trail == TrailOptional {
				// Keep the separator that preceded the failed item.
				s.Restore(afterSep)
			} else {
				s.Restore(itemStart)
			}
			break
		}
		if hasSep && opts.Trail == TrailRequired {
			// A separator after every item, including the last.
			opts.Drop.DropMatchingNextTokens(s)
			if _, sepErr := opts.Sep.MatchNextToken(s); sepErr != nil {
				err = err.Merge(sepErr)
				s.Restore(itemStart)
				break
			}
		}
		keep()
		n++
	}
	if n < opts.Min {
		s.Restore(fieldStart)
		return n, err, false
	}
	return n, err, true
}

// emptyMatchError is the error for a Seq rule none of whose fields matched
// anything.
func emptyMatchError(s *ParserState, ruleName string) *ParseError {
	pos := s.Save()
	if s.beyondRecord(pos) {
		return &s.fail
	}
	tok := s.Next()
	s.Restore(pos)
	return s.Fail(pos, tok, fmt.Errorf("empty match for rule %s", ruleName))
}

type Tok struct {
	SimpleToken
}

var _ Parser = &Tok{}
var _ PrettyWriter = &Tok{}

func (t *Tok) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	if r == t {
		return t.SimpleToken.Parse(r, s, opts)
	}
	ruleDef, _ := getRuleDefAndValue(r)
	ruleField := ruleDef.Fields[0]
	return t.SimpleToken.Parse(r, s, ruleField.TokenOptions)
}

func (t *Tok) PrettyWrite(out io.Writer, r any, pfx string, indent int) error {
	if r == t {
		return t.SimpleToken.PrettyWrite(out, t, pfx, indent)
	}
	return prettyWriteStruct(out, r, pfx, indent)
}
