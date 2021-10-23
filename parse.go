package grammar

import (
	"fmt"
	"strings"
)

type ParserState struct {
	TokenStream
	lastErr *ParseError
	depth   int
}

func (s *ParserState) MergeError(err *ParseError) *ParseError {
	s.lastErr = s.lastErr.Merge(err)
	return s.lastErr
}

// A Parser can parse a token stream to populate itself.  It must return an
// error if it fails to do it.
type Parser interface {
	Parse(r interface{}, s *ParserState, opts ParseOptions) *ParseError
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
	state := &ParserState{
		TokenStream: s,
	}
	err := ParseWithOptions(dest, state, ParseOptions{})
	if err != nil {
		return state.lastErr
	}
	return nil
}

// ParseWithOptions is the same as Parse but the ParseOptions are explicitely
// given (this is mostly used by the parser generator).
func ParseWithOptions(dest interface{}, s *ParserState, opts ParseOptions) *ParseError {
	switch p := dest.(type) {
	case Parser:
		// fmt.Printf("% *d===> %T, %v\n", s.depth*2, s.depth, p, opts)
		s.depth++
		err := p.Parse(dest, s, opts)
		s.depth--
		if err != nil {
			s.MergeError(err)
		}
		// fmt.Printf("% *d<=== %s\n", s.depth*2, s.depth, err)
		return err
	default:
		panic(fmt.Sprintf("invalid type for rule %#v", dest))
	}
}

type ParseError struct {
	Err error
	Token
	TokenParseOptions []TokenParseOptions
	Pos               int
}

func (e *ParseError) Error() string {
	var hint string
	if e.Err != nil {
		hint = e.Err.Error()
	} else if len(e.TokenParseOptions) != 0 {
		var b strings.Builder
		b.WriteString("expected token with ")
		for i, opts := range e.TokenParseOptions {
			if i > 0 {
				b.WriteString(" or ")
			}
			b.WriteString(opts.String())
		}
		hint = b.String()
	}
	return fmt.Sprintf("token #%d %s with value %q: %s", e.Pos, e.Token.Type(), e.Token.Value(), hint)
}

func (e *ParseError) Merge(e2 *ParseError) *ParseError {
	if e == nil {
		return e2
	}
	if e2 == nil {
		return e
	}
	if e.Pos > e2.Pos {
		return e
	}
	if e.Pos < e2.Pos {
		return e2
	}
	return &ParseError{
		Token:             e.Token,
		TokenParseOptions: append(e.TokenParseOptions, e2.TokenParseOptions...),
		Pos:               e.Pos,
	}
}
