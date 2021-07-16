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

// A Token has a Type() and Value() method. A TokenStream's Next() method must
// return a Token.
type Token interface {
	Type() string  // The type of the token (e.g. operator, number, string, identifier)
	Value() string // The value of the token(e.g. `+`, `123`, `"abc"`, `if`)
}

// A TokenStream represents a sequence of tokens that will be consumed by the
// Parse() function.  The SimpleTokenStream implementation in this package can
// be used, or users can provide their own implementation (e.g. with a richer
// data type for tokens, or "on demand" tokenisation).
//
// The Restore() method may panic if is not possible to return to the given
// position.
type TokenStream interface {
	Next() Token // Advance the stream by one token and return it.
	Save() int   // Return the current position in the token stream.
	Restore(int) // Return the stream to the given position.
}

// Parse tries to interpret dest as a grammar rule and use it to parse the given
// token stream.  Parse can panic if dest is not a valid grammar rule.  It
// returns a non-nil *ParseError if the token stream does not match the rule.
func Parse(dest interface{}, s TokenStream) *ParseError {
	return ParseWithOptions(dest, s, ParseOptions{})
}

// ParseWithOptions is the same as Parse but the ParseOptions are explicitely
// given (this is mostly used by the parser generator).
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
