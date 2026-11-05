package grammar

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type RuleDef struct {
	Name        string
	OneOf       bool
	DropOptions TokenOptions
	Fields      []RuleField
	firsts      []FirstSet // of the fields, filled by computeFirstSets
}

type RuleField struct {
	FieldType
	TokenOptions
	SizeOptions
	SepOptions TokenOptions
	Trail      TrailMode
	Name       string
	Index      int
}

type FieldType struct {
	BaseType reflect.Type
	Pointer  bool
	Array    bool
}

type TokenParseOptions struct {
	TokenType    string
	TokenValue   string
	DoNotConsume bool
}

func (o TokenParseOptions) String() string {
	var parts []string
	if o.TokenType != "" {
		parts = append(parts, fmt.Sprintf("type %s", o.TokenType))
	}
	if o.TokenValue != "" {
		parts = append(parts, fmt.Sprintf("value %q", o.TokenValue))
	}
	if len(parts) == 0 {
		return "any type"
	}
	return strings.Join(parts, ",")
}

type TokenOptions struct {
	TokenParseOptions []TokenParseOptions
}

// RepeatOptions gathers the options of a repeated field, the drop options of
// its rule and the first set of its items.
type RepeatOptions struct {
	SizeOptions
	Sep   TokenOptions
	Trail TrailMode
	Drop  TokenOptions // tokens to skip before each item and separator
	First FirstSet     // of an item
}

// RepeatOptions returns the repeat options of the field, with the given drop
// options and item first set.
func (f RuleField) RepeatOptions(drop TokenOptions, first FirstSet) RepeatOptions {
	return RepeatOptions{SizeOptions: f.SizeOptions, Sep: f.SepOptions, Trail: f.Trail, Drop: drop, First: first}
}

// Key returns a comparable value identifying the options, for use by
// Memoisable rules whose Parse result depends on them.
func (o TokenOptions) Key() any {
	switch len(o.TokenParseOptions) {
	case 0:
		return nil
	case 1:
		return o.TokenParseOptions[0]
	}
	var b strings.Builder
	for i, opt := range o.TokenParseOptions {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(opt.TokenType)
		if opt.DoNotConsume {
			b.WriteByte('*')
		}
		b.WriteByte(',')
		b.WriteString(opt.TokenValue)
	}
	return b.String()
}

type SizeOptions struct {
	Min, Max int
}

// TrailMode says what a repeated field with a separator does about a
// separator after the last item.  It is set with the "trail" struct tag.
type TrailMode int

const (
	// TrailNone (the default): items are separated, a trailing separator is
	// not part of the field, e.g. `1, 2`.
	TrailNone TrailMode = iota
	// TrailOptional: a trailing separator is accepted but not required, e.g.
	// `1, 2` or `1, 2,`.
	TrailOptional
	// TrailRequired: every item, including the last, must be followed by the
	// separator (it acts as a terminator), e.g. `1; 2;`.
	TrailRequired
)

var errTrailWithoutSep = errors.New("trail tag without a sep tag")

func trailModeFromTagValue(v string) (TrailMode, error) {
	switch v {
	case "":
		return TrailNone, nil
	case "optional":
		return TrailOptional, nil
	case "required":
		return TrailRequired, nil
	default:
		return TrailNone, fmt.Errorf("invalid trail tag value %q (want optional or required)", v)
	}
}

// MatchNextToken consumes the next token of the stream if it matches one of
// the options and returns it.  If it does not, the failure is recorded (see
// ParserState.Fail) and the error returned with the token.
func (o TokenOptions) MatchNextToken(s *ParserState) (Token, *ParseError) {
	if len(o.TokenParseOptions) == 0 {
		return SimpleToken{}, nil
	}
	pos := s.Save()
	tok := s.Next()

	for _, opts := range o.TokenParseOptions {
		if !opts.matches(tok) {
			continue
		}
		if opts.DoNotConsume {
			s.Restore(pos)
		}
		return tok, nil
	}
	return tok, s.recordFailure(pos, tok, o.TokenParseOptions, nil)
}

// matches reports whether tok has the type and value required by o.
func (o TokenParseOptions) matches(tok Token) bool {
	return (o.TokenType == "" || o.TokenType == tok.Type()) &&
		(o.TokenValue == "" || o.TokenValue == tok.Value())
}

func (o TokenOptions) DropMatchingNextTokens(s TokenStream) {
	if len(o.TokenParseOptions) == 0 {
		return
	}

	for {
		pos := s.Save()
		tok := s.Next()
		if !matchesAny(o.TokenParseOptions, tok) {
			s.Restore(pos)
			return
		}
	}
}

func getRuleDefAndValue(r any) (*RuleDef, reflect.Value) {
	rV := reflect.ValueOf(r)
	if rV.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("dest must be a pointer, it is %T", r))
	}
	elem := rV.Elem()
	tp := elem.Type()
	if tp.Kind() != reflect.Struct {
		panic(fmt.Sprintf("dest must point to a struct, it is %T", r))
	}
	ruleDef, ruleErr := getRuleDef(tp)
	if ruleErr != nil {
		panic(ruleErr)
	}
	return ruleDef, elem
}

// getRuleDef returns the rule definition of tp (or the error if it is not a
// valid rule type), with the first sets of its fields.
//
// Definitions are built for all the types reachable from tp at once, as
// first sets depend on each other, and published to the cache complete, so
// that they are immutable from then on and safe to use concurrently.
func getRuleDef(tp reflect.Type) (*RuleDef, error) {
	cached, ok := ruleDefs.load(tp)
	if !ok {
		newRuleBuilder().build(tp)
		cached, _ = ruleDefs.load(tp)
	}
	return cached.ruleDef, cached.err
}

type ruleDefOrError struct {
	ruleDef *RuleDef
	err     error
}

// ruleDefs caches the rule definitions by type, complete with first sets.
var ruleDefs atomicMap[reflect.Type, ruleDefOrError]

func calcRuleDef(tp reflect.Type) (*RuleDef, error) {
	if tp.Kind() != reflect.Struct {
		return nil, fmt.Errorf("rule type %s should be a struct", tp)
	}
	numField := tp.NumField()
	if numField == 0 {
		return nil, fmt.Errorf("rule type %s should have at least one field", tp)
	}
	firstFieldIndex := 0
	field0 := tp.Field(0)
	oneOf := field0.Type == reflect.TypeOf(OneOf{})
	seq := field0.Type == reflect.TypeOf(Seq{})
	tok := field0.Type == reflect.TypeOf(Tok{})
	var dropOptions TokenOptions
	if oneOf || seq {
		firstFieldIndex++
		dropOptions = tokenOptionsFromTagValue(field0.Tag.Get("drop"))
	} else if tok {
		if numField > 1 {
			return nil, fmt.Errorf("Tok rule type %s must have exactly one field", tp)
		}
	} else {
		return nil, fmt.Errorf("first field of rule type %s should be OneOf or Seq or Tok", tp)
	}

	var ruleFields []RuleField
	for fieldIndex := firstFieldIndex; fieldIndex < numField; fieldIndex++ {
		field := tp.Field(fieldIndex)
		sizeOpts, err := sizeOptionsFromTagValue(field.Tag.Get("size"))
		if err != nil {
			return nil, err
		}
		trail, err := trailModeFromTagValue(field.Tag.Get("trail"))
		if err != nil {
			return nil, fmt.Errorf("field %s of rule type %s: %w", field.Name, tp, err)
		}
		ruleField := RuleField{
			TokenOptions: tokenOptionsFromTagValue(field.Tag.Get("tok")),
			SepOptions:   tokenOptionsFromTagValue(field.Tag.Get("sep")),
			SizeOptions:  sizeOpts,
			Trail:        trail,
			Name:         field.Name,
			Index:        fieldIndex,
		}
		if trail != TrailNone && len(ruleField.SepOptions.TokenParseOptions) == 0 {
			return nil, fmt.Errorf("field %s of rule type %s: %w", field.Name, tp, errTrailWithoutSep)
		}
		switch field.Type.Kind() {
		case reflect.Ptr:
			ruleField.FieldType = FieldType{
				Pointer:  true,
				BaseType: field.Type.Elem(),
			}
		case reflect.Slice:
			ruleField.FieldType = FieldType{
				Array:    true,
				BaseType: field.Type.Elem(),
			}
		default:
			if oneOf {
				return nil, errors.New("OneOf fields must be pointers or slices")
			}
			ruleField.FieldType = FieldType{
				BaseType: field.Type,
			}
		}
		ruleFields = append(ruleFields, ruleField)
	}
	return &RuleDef{
		Name:        tp.Name(),
		OneOf:       oneOf,
		Fields:      ruleFields,
		DropOptions: dropOptions,
	}, nil
}

func sizeOptionsFromTagValue(v string) (opts SizeOptions, err error) {
	if v == "" {
		return
	}
	var min, max uint64
	if i := strings.IndexByte(v, '-'); i >= 0 {
		if i > 0 {
			min, err = strconv.ParseUint(v[:i], 10, 64)
		}
		if err == nil && i+1 < len(v) {
			max, err = strconv.ParseUint(v[i+1:], 10, 64)
		}
	} else {
		min, err = strconv.ParseUint(v, 10, 64)
		max = min
	}
	opts.Min = int(min)
	opts.Max = int(max)
	return
}

func tokenOptionsFromTagValue(v string) TokenOptions {
	var opts []TokenParseOptions
	for _, optStr := range strings.Split(v, "|") {
		if optStr == "" {
			continue
		}
		var tt, tv string
		if i := strings.IndexByte(optStr, ','); i >= 0 {
			tt = optStr[:i]
			tv = optStr[i+1:]
		} else {
			tt = optStr
		}
		dnc := tt[len(tt)-1] == '*'
		if dnc {
			tt = tt[:len(tt)-1]
		}
		opts = append(opts, TokenParseOptions{
			TokenType:    tt,
			TokenValue:   tv,
			DoNotConsume: dnc,
		})
	}
	return TokenOptions{TokenParseOptions: opts}
}
