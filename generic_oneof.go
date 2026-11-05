package grammar

import (
	"io"
	"reflect"
)

// OneOfValue holds the result of parsing one of the generic OneOfN rules.
// Value is a pointer to the alternative that matched (e.g. *T2 for the second
// type parameter of OneOf3[T1, T2, T3]), so it is usually consumed with a type
// switch.
type OneOfValue struct {
	Value any
}

var _ Memoisable = OneOfValue{}

// MemoKey implements Memoisable: the OneOfN rules ignore the token options.
func (OneOfValue) MemoKey(opts TokenOptions) any {
	return nil
}

func (o *OneOfValue) PrettyWrite(out io.Writer, r any, pfx string, indent int) (err error) {
	return prettyWrite(out, o.Value, pfx, indent)
}

type optionParser struct {
	*OneOfValue
	s      *ParserState
	firsts []FirstSet // of the alternatives
	err    *ParseError
}

func newOptionParser(o *OneOfValue, alts alternatives, s *ParserState) optionParser {
	return optionParser{OneOfValue: o, s: s, firsts: alternativeFirsts(alts)}
}

// check reports whether alternative i may start with the next token; if not
// the failure is recorded.
func (o *optionParser) check(i int) bool {
	if err := o.firsts[i].check(o.s); err != nil {
		o.err = o.err.Merge(err)
		return false
	}
	return true
}

func (o *optionParser) parseOption(r any) bool {
	start := o.s.Save()
	optErr := ParseWithOptions(r, o.s, TokenOptions{})
	if optErr == nil {
		o.Value = r
		o.err = nil
		return true
	}
	o.s.Restore(start)
	o.err = o.err.Merge(optErr)
	return false
}

// OneOf2 is a rule that matches the first of T1, T2 that parses, in that
// order.  Each type parameter must be a rule type, i.e. a pointer to it must
// implement Parser; this is checked when the rule is first parsed, not at
// compile time, because Go cannot express "pointer to T implements Parser"
// without doubling the type parameters.
//
// OneOf3 to OneOf6 are the same for more alternatives.
//
// The recommended way to use these types is to embed them in a named struct
// rather than to define a type alias, so that methods can be defined on the
// rule and so that it has a short name in errors and debug output:
//
//	type Value struct {
//	    grammar.OneOf3[Number, String, List]
//	}
//
//	func (v *Value) Compile() any {
//	    switch x := v.Value.(type) {
//	    case *Number: ...
//	    case *String: ...
//	    case *List: ...
//	    }
//	}
type OneOf2[T1, T2 any] struct {
	OneOfValue
}

var _ Parser = &OneOf2[any, any]{}
var _ PrettyWriter = &OneOf2[any, any]{}

func (o *OneOf2[T1, T2]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	p := newOptionParser(&o.OneOfValue, o, s)
	_ = p.check(0) && p.parseOption(new(T1)) ||
		p.check(1) && p.parseOption(new(T2))
	return p.err
}

func (*OneOf2[T1, T2]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2]()}
}

type OneOf3[T1, T2, T3 any] struct {
	OneOfValue
}

var _ Parser = &OneOf3[any, any, any]{}
var _ PrettyWriter = &OneOf3[any, any, any]{}

func (o *OneOf3[T1, T2, T3]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	p := newOptionParser(&o.OneOfValue, o, s)
	_ = p.check(0) && p.parseOption(new(T1)) ||
		p.check(1) && p.parseOption(new(T2)) ||
		p.check(2) && p.parseOption(new(T3))
	return p.err
}

func (*OneOf3[T1, T2, T3]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2](), reflect.TypeFor[T3]()}
}

type OneOf4[T1, T2, T3, T4 any] struct {
	OneOfValue
}

var _ Parser = &OneOf4[any, any, any, any]{}
var _ PrettyWriter = &OneOf4[any, any, any, any]{}

func (o *OneOf4[T1, T2, T3, T4]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	p := newOptionParser(&o.OneOfValue, o, s)
	_ = p.check(0) && p.parseOption(new(T1)) ||
		p.check(1) && p.parseOption(new(T2)) ||
		p.check(2) && p.parseOption(new(T3)) ||
		p.check(3) && p.parseOption(new(T4))
	return p.err
}

func (*OneOf4[T1, T2, T3, T4]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2](), reflect.TypeFor[T3](), reflect.TypeFor[T4]()}
}

type OneOf5[T1, T2, T3, T4, T5 any] struct {
	OneOfValue
}

var _ Parser = &OneOf5[any, any, any, any, any]{}
var _ PrettyWriter = &OneOf5[any, any, any, any, any]{}

func (o *OneOf5[T1, T2, T3, T4, T5]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	p := newOptionParser(&o.OneOfValue, o, s)
	_ = p.check(0) && p.parseOption(new(T1)) ||
		p.check(1) && p.parseOption(new(T2)) ||
		p.check(2) && p.parseOption(new(T3)) ||
		p.check(3) && p.parseOption(new(T4)) ||
		p.check(4) && p.parseOption(new(T5))
	return p.err
}

func (*OneOf5[T1, T2, T3, T4, T5]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2](), reflect.TypeFor[T3](), reflect.TypeFor[T4](), reflect.TypeFor[T5]()}
}

type OneOf6[T1, T2, T3, T4, T5, T6 any] struct {
	OneOfValue
}

var _ Parser = &OneOf6[any, any, any, any, any, any]{}
var _ PrettyWriter = &OneOf6[any, any, any, any, any, any]{}

func (o *OneOf6[T1, T2, T3, T4, T5, T6]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	p := newOptionParser(&o.OneOfValue, o, s)
	_ = p.check(0) && p.parseOption(new(T1)) ||
		p.check(1) && p.parseOption(new(T2)) ||
		p.check(2) && p.parseOption(new(T3)) ||
		p.check(3) && p.parseOption(new(T4)) ||
		p.check(4) && p.parseOption(new(T5)) ||
		p.check(5) && p.parseOption(new(T6))
	return p.err
}

func (*OneOf6[T1, T2, T3, T4, T5, T6]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2](), reflect.TypeFor[T3](), reflect.TypeFor[T4](), reflect.TypeFor[T5](), reflect.TypeFor[T6]()}
}
