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
}

type RuleField struct {
	FieldType
	TokenOptions
	SizeOptions
	SepOptions TokenOptions
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

type SizeOptions struct {
	Min, Max int
}

func (o TokenOptions) MatchNextToken(s TokenStream) (Token, *ParseError) {
	if len(o.TokenParseOptions) == 0 {
		return SimpleToken{}, nil
	}
	pos := s.Save()
	tok := s.Next()

	for _, opts := range o.TokenParseOptions {
		if opts.TokenType != "" && opts.TokenType != tok.Type() {
			continue
			// return fmt.Errorf("expected token of type %s", opts.TokenType)
		}
		if opts.TokenValue != "" && opts.TokenValue != tok.Value() {
			continue
			// return fmt.Errorf("expected token with value %q", opts.TokenValue)
		}
		if opts.DoNotConsume {
			s.Restore(pos)
		}
		return tok, nil
	}
	return tok, &ParseError{
		Token:             tok,
		TokenParseOptions: o.TokenParseOptions,
		Pos:               pos,
	}
}

func (o TokenOptions) DropMatchingNextTokens(s TokenStream) {
	if len(o.TokenParseOptions) == 0 {
		return
	}

outerLoop:
	for {
		pos := s.Save()
		tok := s.Next()

		for _, opts := range o.TokenParseOptions {
			if opts.TokenType != "" && opts.TokenType != tok.Type() {
				continue
			}
			if opts.TokenValue != "" && opts.TokenValue != tok.Value() {
				continue
			}
			continue outerLoop
		}
		s.Restore(pos)
		return
	}
}

func getRuleDefAndValue(r interface{}) (*RuleDef, reflect.Value) {
	rV := reflect.ValueOf(r)
	if rV.Kind() != reflect.Ptr {
		panic("dest must be a pointer")
	}
	elem := rV.Elem()
	tp := elem.Type()
	if tp.Kind() != reflect.Struct {
		panic("dest must point to a struct")
	}
	ruleDef, ruleErr := getRuleDef(tp)
	if ruleErr != nil {
		panic(ruleErr)
	}
	return ruleDef, elem
}

func getRuleDef(tp reflect.Type) (*RuleDef, error) {
	cached, ok := ruleDefCache[tp]
	if ok {
		return cached.ruleDef, cached.err
	}
	ruleDef, err := calcRuleDef(tp)
	ruleDefCache[tp] = ruleDefCacheValue{
		ruleDef: ruleDef,
		err:     err,
	}
	return ruleDef, err
}

type ruleDefCacheValue struct {
	ruleDef *RuleDef
	err     error
}

var ruleDefCache = map[reflect.Type]ruleDefCacheValue{}

func calcRuleDef(tp reflect.Type) (*RuleDef, error) {
	if tp.Kind() != reflect.Struct {
		return nil, errors.New("type should be a struct")
	}
	numField := tp.NumField()
	if numField == 0 {
		return nil, errors.New("type should have at least one field")
	}
	firstFieldIndex := 0
	field0 := tp.Field(0)
	oneOf := field0.Type == reflect.TypeOf(OneOf{})
	seq := field0.Type == reflect.TypeOf(Seq{})
	var dropOptions TokenOptions
	if oneOf || seq {
		firstFieldIndex++
		dropOptions = tokenOptionsFromTagValue(field0.Tag.Get("drop"))
	} else {
		return nil, errors.New("first rule field should be OneOf or Seq")
	}

	var ruleFields []RuleField
	for fieldIndex := firstFieldIndex; fieldIndex < numField; fieldIndex++ {
		field := tp.Field(fieldIndex)
		sizeOpts, err := sizeOptionsFromTagValue(field.Tag.Get("size"))
		if err != nil {
			return nil, err
		}
		ruleField := RuleField{
			TokenOptions: tokenOptionsFromTagValue(field.Tag.Get("tok")),
			SepOptions:   tokenOptionsFromTagValue(field.Tag.Get("sep")),
			SizeOptions:  sizeOpts,
			Name:         field.Name,
			Index:        fieldIndex,
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
