package grammar

import (
	"fmt"
	"reflect"
	"strings"
)

// OneOf can be used as the first field of a Rule struct to signify that it
// should match exactly one of the fields
type OneOf struct{}

//
type Parser interface {
	Parse(t TokenStream, opts ParseOptions) error
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

func (opts ParseOptions) matchToken(tok Token) error {
	if opts.TokenType != "" && opts.TokenType != tok.Type() {
		return fmt.Errorf("expected token of type %s", opts.TokenType)
	}
	if opts.TokenValue != "" && opts.TokenValue != tok.Value() {
		return fmt.Errorf("expected token with value %q", opts.TokenValue)
	}
	return nil
}

func Parse(dest interface{}, t TokenStream) (err error) {
	// defer func() {
	// 	if r := recover(); r != nil {
	// 		var ok bool
	// 		err, ok = r.(error)
	// 		if !ok {
	// 			err = errors.New("unknown error")
	// 		}
	// 	}
	// }()
	tp := reflect.TypeOf(dest)
	if tp.Kind() != reflect.Ptr {
		panic("Parse must be given a pointer")
	}
	p := parser{tokenStream: t}
	v := p.parse(tp.Elem(), ParseOptions{})
	if !v.IsValid() {
		return p.error()
	}
	reflect.ValueOf(dest).Elem().Set(v.Elem())
	return nil
}

type parser struct {
	tokenStream TokenStream

	errPos int
	err    error
}

func (p *parser) updateError(err error) {
	if err == nil {
		return
	}
	errPos := p.tokenStream.Save()
	if errPos > p.errPos {
		p.errPos = errPos
		p.err = err
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

func (p *parser) error() error {
	if p.err != nil {
		p.tokenStream.Restore(p.errPos - 1)
		tok := p.tokenStream.Next()
		return &ParseError{
			Err:   p.err,
			Token: tok,
			Pos:   p.errPos - 1,
		}
	}
	return nil
}

func (p *parser) parse(tp reflect.Type, opts ParseOptions) reflect.Value {
	parserTp := reflect.TypeOf((*Parser)(nil)).Elem()

	ptr := reflect.New(tp)
	t := p.tokenStream
	start := t.Save()

	if ptr.Type().Implements(parserTp) {
		err := ptr.Interface().(Parser).Parse(t, opts)
		if err != nil {
			p.updateError(err)
			t.Restore(start)
			return reflect.Value{}
		}
		return ptr
	}

	// If the type doesn't implement Parser, it should be a struct.
	if tp.Kind() != reflect.Struct {
		panic("must be a struct")
	}

	elem := ptr.Elem()

	numField := tp.NumField()
	if numField == 0 {
		return ptr
	}
	firstFieldIndex := 0
	oneOf := tp.Field(0).Type == reflect.TypeOf(OneOf{})
	if oneOf {
		firstFieldIndex++
	}
	for fieldIndex := firstFieldIndex; fieldIndex < numField; fieldIndex++ {
		field := tp.Field(fieldIndex)
		fieldOpts := parseTagValue(field.Tag.Get("tok"))
		switch field.Type.Kind() {
		case reflect.Ptr:
			// Optional field
			startOpt := t.Save()
			v := p.parse(field.Type.Elem(), fieldOpts)
			if v.IsValid() {
				elem.Field(fieldIndex).Set(v)
				if oneOf {
					return ptr
				}
			} else {
				t.Restore(startOpt)
			}
		case reflect.Slice:
			// Repeated field
			vs := reflect.Zero(field.Type)
			for {
				startOpt := t.Save()
				v := p.parse(field.Type.Elem(), fieldOpts)
				if v.IsValid() {
					vs = reflect.Append(vs, v.Elem())
				} else {
					t.Restore(startOpt)
					break
				}
			}
			elem.Field(fieldIndex).Set(vs)
			if oneOf && vs.Len() > 0 {
				return ptr
			}
		default:
			// Compulsory field
			if oneOf {
				panic("oneOf fields must be pointers or slices")
			}
			v := p.parse(field.Type, fieldOpts)
			if v.IsValid() {
				elem.Field(fieldIndex).Set(v.Elem())
			} else {
				t.Restore(start)
				return reflect.Value{}
			}
		}
	}
	if oneOf {
		return reflect.Value{}
	}
	return ptr
}

func parseTagValue(v string) ParseOptions {
	if i := strings.IndexByte(v, ','); i >= 0 {
		return ParseOptions{
			TokenType:  v[:i],
			TokenValue: v[i+1:],
		}
	}
	return ParseOptions{TokenType: v}
}
