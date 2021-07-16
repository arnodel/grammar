package grammar

import (
	"fmt"
	"reflect"
)

// A Parser can parse a token stream to populate itself.  It must return an
// error if it fails to do it.
type Parser interface {
	Parse(t TokenStream, opts ParseOptions) *ParseError
}

type Token interface {
	Type() string
	Value() string
}

type TokenStream interface {
	Next() Token
	Save() int
	Restore(int)
}

func Parse(dest interface{}, s TokenStream) *ParseError {
	return ParseWithOptions(dest, s, ParseOptions{})
}

func ParseWithOptions(dest interface{}, s TokenStream, opts ParseOptions) *ParseError {
	if parser, ok := dest.(Parser); ok {
		return parser.Parse(s, opts)
	}
	return parse(dest, s, opts)
}

func parse(dest interface{}, s TokenStream, opts ParseOptions) *ParseError {
	destV := reflect.ValueOf(dest)
	if destV.Kind() != reflect.Ptr {
		panic("dest must be a pointer")
	}
	elem := destV.Elem()
	tp := elem.Type()
	if tp.Kind() != reflect.Struct {
		panic("dest must point to a struct")
	}
	ruleDef, ruleErr := getRuleDef(tp)
	if ruleErr != nil {
		panic(ruleErr)
	}
	if ruleDef.OneOf {
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
					for {
						start := s.Save()
						itemPtrV := reflect.New(ruleField.BaseType)
						fieldErr = ParseWithOptions(itemPtrV.Interface(), s, ruleField.ParseOptions)
						if fieldErr != nil {
							s.Restore(start)
							err = err.Merge(fieldErr)
							break
						}
						itemsV = reflect.Append(itemsV, itemPtrV.Elem())
					}
					if itemsV.Len() > 0 {
						elem.Field(ruleField.Index).Set(itemsV)
						return nil
					}
				}
			default:
				panic("should not get here")
			}
		}
		return err
	} else {
		var fieldErr *ParseError
		for _, ruleField := range ruleDef.Fields {
			switch {
			case ruleField.Pointer:
				{
					start := s.Save()
					fieldPtrV := reflect.New(ruleField.BaseType)
					fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.ParseOptions)
					if fieldErr != nil {
						s.Restore(start)
					} else {
						elem.Field(ruleField.Index).Set(fieldPtrV)
					}
				}
			case ruleField.Array:
				{
					itemsV := reflect.Zero(reflect.SliceOf(ruleField.BaseType))
					for {
						start := s.Save()
						itemPtrV := reflect.New(ruleField.BaseType)
						fieldErr = ParseWithOptions(itemPtrV.Interface(), s, ruleField.ParseOptions)
						if fieldErr != nil {
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
					return fieldErr
				}
				elem.Field(ruleField.Index).Set(fieldPtrV.Elem())
			}
		}
		return nil
	}
}

type ParseError struct {
	Err error
	Token
	Pos int
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("token #%d %s with value %q: %s", e.Pos, e.Token.Type(), e.Token.Value(), e.Err)
}

func (e *ParseError) Merge(e2 *ParseError) *ParseError {
	if e == nil {
		return e2
	}
	if e2 == nil {
		return e
	}
	if e.Pos >= e2.Pos {
		return e
	}
	return e2
}
