package grammar

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// OneOf should be used as the first field of a Rule struct to signify that it
// should match exactly one of the fields
type OneOf struct{}

var _ Parser = OneOf{}

func (OneOf) Parse(r interface{}, s *ParserState, opts ParseOptions) *ParseError {
	ruleDef, elem := getRuleDefAndValue(r)
	var err, fieldErr *ParseError
	for _, ruleField := range ruleDef.Fields {
		switch {
		case ruleField.Pointer:
			{
				start := s.Save()
				fieldPtrV := reflect.New(ruleField.BaseType)
				fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.ParseOptions)
				if fieldErr == nil {
					elem.Field(ruleField.Index).Set(fieldPtrV)
					return nil
				}
				s.Restore(start)
				err = err.Merge(fieldErr)
			}
		case ruleField.Array:
			{
				itemsV := reflect.Zero(reflect.SliceOf(ruleField.BaseType))
				var sz int
				arrStart := s.Save()
				for sz = 0; ruleField.Max == 0 || sz < ruleField.Max; sz++ {
					start := s.Save()
					itemPtrV := reflect.New(ruleField.BaseType)
					fieldErr = ParseWithOptions(itemPtrV.Interface(), s, ruleField.ParseOptions)
					if fieldErr != nil {
						if sz < ruleField.Min {
							s.Restore(arrStart)
							break
						}
						s.Restore(start)
						if sz > 0 {
							elem.Field(ruleField.Index).Set(itemsV)
							return nil
						}
						err = err.Merge(fieldErr)
						break
					}
					itemsV = reflect.Append(itemsV, itemPtrV.Elem())
				}
			}
		default:
			panic("should not get here")
		}
	}
	return err
}

// Seq should be used as the first field of a Rule struct to signify that it
// should match all the fields in sequence.
type Seq struct{}

var _ Parser = Seq{}

func (Seq) Parse(r interface{}, s *ParserState, opts ParseOptions) *ParseError {
	ruleDef, elem := getRuleDefAndValue(r)
	var err, fieldErr *ParseError
	for _, ruleField := range ruleDef.Fields {
		switch {
		case ruleField.Pointer:
			{
				start := s.Save()
				fieldPtrV := reflect.New(ruleField.BaseType)
				fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.ParseOptions)
				if fieldErr != nil {
					err = err.Merge(fieldErr)
					s.Restore(start)
				} else {
					elem.Field(ruleField.Index).Set(fieldPtrV)
				}
			}
		case ruleField.Array:
			{
				itemsV := reflect.Zero(reflect.SliceOf(ruleField.BaseType))
				var sz int
				for sz = 0; ruleField.Max == 0 || sz < ruleField.Max; sz++ {
					start := s.Save()
					itemPtrV := reflect.New(ruleField.BaseType)
					fieldErr = ParseWithOptions(itemPtrV.Interface(), s, ruleField.ParseOptions)
					if fieldErr != nil {
						err = err.Merge(fieldErr)
						if sz < ruleField.Min {
							return err
						}
						s.Restore(start)
						break
					}
					itemsV = reflect.Append(itemsV, itemPtrV.Elem())
				}
				elem.Field(ruleField.Index).Set(itemsV)
			}
		default:
			fieldPtrV := reflect.New(ruleField.BaseType)
			fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.ParseOptions)
			if fieldErr != nil {
				return err.Merge(fieldErr)
			}
			elem.Field(ruleField.Index).Set(fieldPtrV.Elem())
		}
	}
	return nil
}

type RuleDef struct {
	Name   string
	OneOf  bool
	Fields []RuleField
}

type RuleField struct {
	FieldType
	ParseOptions
	SizeOptions
	Name  string
	Index int
}

type FieldType struct {
	BaseType reflect.Type
	Pointer  bool
	Array    bool
}

type ParseOptions struct {
	TokenType  string
	TokenValue string
}

type SizeOptions struct {
	Min, Max int
}

func (opts ParseOptions) MatchToken(tok Token) error {
	if opts.TokenType != "" && opts.TokenType != tok.Type() {
		return fmt.Errorf("expected token of type %s", opts.TokenType)
	}
	if opts.TokenValue != "" && opts.TokenValue != tok.Value() {
		return fmt.Errorf("expected token with value %q", opts.TokenValue)
	}
	return nil
}

func parseOptionsFromTagValue(v string) ParseOptions {
	if v == "" {
		return ParseOptions{}
	}
	if i := strings.IndexByte(v, ','); i >= 0 {
		return ParseOptions{
			TokenType:  v[:i],
			TokenValue: v[i+1:],
		}
	}
	return ParseOptions{TokenType: v}
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

func calcRuleDef(tp reflect.Type) (*RuleDef, error) {
	if tp.Kind() != reflect.Struct {
		return nil, errors.New("type should be a struct")
	}
	numField := tp.NumField()
	if numField == 0 {
		return nil, errors.New("type should have at least one field")
	}
	firstFieldIndex := 0
	oneOf := tp.Field(0).Type == reflect.TypeOf(OneOf{})
	rule := tp.Field(0).Type == reflect.TypeOf(Seq{})
	if oneOf || rule {
		firstFieldIndex++
	} else {
		return nil, errors.New("first rule field should be OneOf or Rule")
	}
	var ruleFields []RuleField
	for fieldIndex := firstFieldIndex; fieldIndex < numField; fieldIndex++ {
		field := tp.Field(fieldIndex)
		sizeOpts, err := sizeOptionsFromTagValue(field.Tag.Get("size"))
		if err != nil {
			return nil, err
		}
		ruleField := RuleField{
			ParseOptions: parseOptionsFromTagValue(field.Tag.Get("tok")),
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
		Name:   tp.Name(),
		OneOf:  oneOf,
		Fields: ruleFields,
	}, nil
}

type ruleDefCacheValue struct {
	ruleDef *RuleDef
	err     error
}

var ruleDefCache = map[reflect.Type]ruleDefCacheValue{}

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
