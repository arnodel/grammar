package grammar

import (
	"fmt"
	"log"
	"strings"
)

type ParserState struct {
	TokenStream
	lastErr *ParseError
	depth   int
	logger  *log.Logger
}

func (s *ParserState) MergeError(err *ParseError) *ParseError {
	s.lastErr = s.lastErr.Merge(err)
	return s.lastErr
}

func (s *ParserState) Debug() bool {
	return s.logger != nil
}

func (s *ParserState) Logf(fstr string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Printf("% *d"+fstr, append([]interface{}{s.depth * 2, s.depth}, args...)...)
	}
}

// A Parser can parse a token stream to populate itself.  It must return an
// error if it fails to do it.
type Parser interface {
	Parse(r interface{}, s *ParserState, opts TokenOptions) *ParseError
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

type ParseOption func(s *ParserState)

func WithLogger(l *log.Logger) ParseOption {
	return func(s *ParserState) {
		s.logger = l
	}
}

var WithDefaultLogger = WithLogger(log.Default())

// Parse tries to interpret dest as a grammar rule and use it to parse the given
// token stream.  Parse can panic if dest is not a valid grammar rule.  It
// returns a non-nil *ParseError if the token stream does not match the rule.
func Parse(dest interface{}, s TokenStream, opts ...ParseOption) *ParseError {
	state := &ParserState{
		TokenStream: s,
	}
	for _, opt := range opts {
		opt(state)
	}
	err := ParseWithOptions(dest, state, TokenOptions{})
	if err != nil {
		return state.lastErr
	}
	return nil
}

// ParseWithOptions is the same as Parse but the ParseOptions are explicitely
// given (this is mostly used by the parser generator).
func ParseWithOptions(dest interface{}, s *ParserState, opts TokenOptions) *ParseError {
	switch p := dest.(type) {
	case Parser:
		if s.Debug() {
			s.Logf("===> %T, %v", p, opts)
		}
		s.depth++
		err := p.Parse(dest, s, opts)
		s.depth--
		if err != nil {
			s.MergeError(err)
		}
		if s.Debug() {
			s.Logf("<=== %s", err)
		}
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
		types, values := summariseOptions(e.TokenParseOptions)
		if len(types) > 0 {
			b.WriteString("type ")
			for i, t := range types {
				if i > 0 {
					b.WriteString(" or ")
				}
				b.WriteString(t)
			}
		}
		if len(values) > 0 {
			if len(types) > 0 {
				b.WriteString(", or ")
			}
			b.WriteString("value ")
			for i, v := range values {
				if i > 0 {
					b.WriteString(" or ")
				}
				fmt.Fprintf(&b, "%q", v)
			}
		}
		hint = b.String()
	}
	return fmt.Sprintf("token #%d %s with value %q: %s", e.Pos, e.Token.Type(), e.Token.Value(), hint)
}

func summariseOptions(opts []TokenParseOptions) ([]string, []string) {
	seenTypes := map[string]struct{}{}
	seenValues := map[string]struct{}{}
	for _, opt := range opts {
		if opt.TokenValue != "" {
			seenValues[opt.TokenValue] = struct{}{}
		} else if opt.TokenType != "" {
			seenTypes[opt.TokenType] = struct{}{}
		}
	}
	var types, values []string
	for t := range seenTypes {
		types = append(types, t)
	}
	for v := range seenValues {
		values = append(values, v)
	}
	return types, values
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
