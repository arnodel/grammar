package grammar

import "io"

type OneOfValue struct {
	Value any
}

func (o *OneOfValue) PrettyWrite(out io.Writer, r any, pfx string, indent int) (err error) {
	return prettyWrite(out, o.Value, pfx, indent)
}

type optionParser struct {
	*OneOfValue
	err *ParseError
}

func (o *optionParser) parseOption(r any, s *ParserState) bool {
	start := s.Save()
	optErr := ParseWithOptions(r, s, TokenOptions{})
	if optErr == nil {
		o.Value = r
		o.err = nil
		return true
	}
	s.Restore(start)
	o.err = o.err.Merge(optErr)
	return false
}

type OneOf2[T1, T2 Parser] struct {
	OneOfValue
}

var _ Parser = &OneOf2[Empty, Empty]{}
var _ PrettyWriter = &OneOf2[Empty, Empty]{}

func (o *OneOf2[T1, T2]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	p := optionParser{OneOfValue: &o.OneOfValue}
	_ = p.parseOption(new(T1), s) ||
		p.parseOption(new(T2), s)
	return p.err
}

type OneOf3[T1, T2, T3 Parser] struct {
	OneOfValue
}

var _ Parser = &OneOf3[Empty, Empty, Empty]{}
var _ PrettyWriter = &OneOf3[Empty, Empty, Empty]{}

func (o *OneOf3[T1, T2, T3]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	p := optionParser{OneOfValue: &o.OneOfValue}
	_ = p.parseOption(new(T1), s) ||
		p.parseOption(new(T2), s) ||
		p.parseOption(new(T3), s)
	return p.err
}

type OneOf4[T1, T2, T3, T4 Parser] struct {
	OneOfValue
}

var _ Parser = &OneOf4[Empty, Empty, Empty, Empty]{}
var _ PrettyWriter = &OneOf4[Empty, Empty, Empty, Empty]{}

func (o *OneOf4[T1, T2, T3, T4]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	p := optionParser{OneOfValue: &o.OneOfValue}
	_ = p.parseOption(new(T1), s) ||
		p.parseOption(new(T2), s) ||
		p.parseOption(new(T3), s) ||
		p.parseOption(new(T4), s)
	return p.err
}

type OneOf5[T1, T2, T3, T4, T5 any] struct {
	OneOfValue
}

var _ Parser = &OneOf5[any, any, any, any, any]{}
var _ PrettyWriter = &OneOf5[any, any, any, any, any]{}

func (o *OneOf5[T1, T2, T3, T4, T5]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	p := optionParser{OneOfValue: &o.OneOfValue}
	_ = p.parseOption(new(T1), s) ||
		p.parseOption(new(T2), s) ||
		p.parseOption(new(T3), s) ||
		p.parseOption(new(T4), s) ||
		p.parseOption(new(T5), s)
	return p.err
}
