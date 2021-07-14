package grammar

import (
	"fmt"
	"reflect"
	"strings"
)

// OneOf can be used as the first field of a Rule struct to signify that it
// should match exactly one of the fields
type OneOf struct{}

type Rule struct{}

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

type ParseOptions struct {
	TokenType  string
	TokenValue string
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

func optionsFromTagValue(v string) ParseOptions {
	if i := strings.IndexByte(v, ','); i >= 0 {
		return ParseOptions{
			TokenType:  v[:i],
			TokenValue: v[i+1:],
		}
	}
	return ParseOptions{TokenType: v}
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
	numField := tp.NumField()
	if numField == 0 {
		return nil
	}
	firstFieldIndex := 0
	oneOf := tp.Field(0).Type == reflect.TypeOf(OneOf{})
	if oneOf {
		firstFieldIndex++
	}
	rule := tp.Field(0).Type == reflect.TypeOf(Rule{})
	if rule {
		firstFieldIndex++
	}
	var err, fieldErr *ParseError
	for fieldIndex := firstFieldIndex; fieldIndex < numField; fieldIndex++ {
		field := tp.Field(fieldIndex)
		fieldOpts := optionsFromTagValue(field.Tag.Get("tok"))
		switch field.Type.Kind() {
		case reflect.Ptr:
			// Optional field
			startOpt := s.Save()
			fieldVal := reflect.New(field.Type.Elem())
			if fieldErr = ParseWithOptions(fieldVal.Interface(), s, fieldOpts); fieldErr != nil {
				s.Restore(startOpt)
				if oneOf {
					err = err.Merge(fieldErr)
				}
			} else {
				elem.Field(fieldIndex).Set(fieldVal)
				if oneOf {
					return nil
				}
			}
		case reflect.Slice:
			// Repeated field
			vs := reflect.Zero(field.Type)
			for {
				startOpt := s.Save()
				fieldVal := reflect.New(field.Type.Elem())
				if fieldErr = ParseWithOptions(fieldVal.Interface(), s, fieldOpts); fieldErr != nil {
					s.Restore(startOpt)
					break
				}
				vs = reflect.Append(vs, fieldVal.Elem())
			}
			elem.Field(fieldIndex).Set(vs)
			if oneOf && vs.Len() > 0 {
				return nil
			}
			if oneOf {
				err = err.Merge(fieldErr)
			}
		default:
			// Compulsory field
			if oneOf {
				panic("oneOf fields must be pointers or slices")
			}
			fieldVal := reflect.New(field.Type)
			if fieldErr = ParseWithOptions(fieldVal.Interface(), s, fieldOpts); fieldErr != nil {
				return fieldErr
			}
			elem.Field(fieldIndex).Set(fieldVal.Elem())
		}
	}
	return err
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
