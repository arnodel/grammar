package grammar

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
)

type memoKey struct {
	pos      int
	ruleType reflect.Type
}

type memoValue struct {
	pos      int
	val      any
	err      *ParseError
	rec      bool
	accessed bool
}

type memoTable map[memoKey]*memoValue

func (m memoTable) Get(key memoKey, dest any) (int, bool, *ParseError) {
	val, ok := m[key]
	if !ok {
		return 0, false, nil
	}
	val.accessed = true
	if val.val != nil {
		// log.Printf("before: %#v", rule)
		reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(val.val).Elem())
		// log.Printf("after: %#v", rule)
	}
	return val.pos, val.rec, val.err
}

func (m memoTable) Set(key memoKey, rule any, newPos int, err *ParseError, rec bool) {
	switch rule.(type) {
	case *SimpleToken, *Tok, *Match:
		return
	default:
		m[key] = &memoValue{
			pos: newPos,
			val: rule,
			err: err,
			rec: rec,
		}
	}
}

type ParserState struct {
	TokenStream
	memo     memoTable
	keyStack []memoKey
	lastErr  *ParseError
	logger   *log.Logger
}

func (s *ParserState) MergeError(err *ParseError) *ParseError {
	s.lastErr = s.lastErr.Merge(err)
	return s.lastErr
}

func (s *ParserState) Debug() bool {
	return s.logger != nil
}

func (s *ParserState) Logf(fstr string, args ...any) {
	if s.logger != nil {
		depth := len(s.keyStack)
		s.logger.Printf("% *d"+fstr, append([]any{depth * 2, depth}, args...)...)
	}
}

// A Parser can parse a token stream to populate itself.  It must return an
// error if it fails to do it.
type Parser interface {
	Parse(r any, s *ParserState, opts TokenOptions) *ParseError
}

// A Token has a Type() and Value() method. A TokenStream's Next() method must
// return a Token.
type Token interface {
	Type() string       // The type of the token (e.g. operator, number, string, identifier)
	Value() string      // The value of the token(e.g. `+`, `123`, `"abc"`, `if`)
	Position() Position // The position of the token in the source text
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
func Parse(dest any, s TokenStream, opts ...ParseOption) *ParseError {
	state := &ParserState{
		TokenStream: s,
		memo:        memoTable{},
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
func ParseWithOptions(dest any, s *ParserState, opts TokenOptions) *ParseError {
	pos := s.Save()
	key := memoKey{pos: pos, ruleType: reflect.TypeOf(dest)}
	memoVal := s.memo[key]
	if memoVal != nil {
		s.Logf("pos: %d, memoVal: %v", pos, *memoVal)
		if memoVal.rec {
			memoVal.accessed = true
			i := len(s.keyStack) - 1
			for {
				k := s.keyStack[i]
				if k == key {
					break
				}
				s.Logf("Delete %d, %v", i, k)
				delete(s.memo, k)
				i--
			}
		}
		if memoVal.err != nil {
			return memoVal.err
		}
		if memoVal.pos > 0 {
			s.Restore(memoVal.pos)
			s.Logf("dest: %v, memoVal: %v", dest, *memoVal)
			reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(memoVal.val).Elem())
			return nil
		}
	}

	switch p := dest.(type) {
	case Parser:
		if s.Debug() {
			s.Logf("===> %T, %v", p, opts)
		}
		s.keyStack = append(s.keyStack, key)
		memoVal := memoValue{
			err: &ParseError{Err: errRecursion, Token: SimpleToken{}, Pos: pos},
			val: dest,
			rec: true,
		}
		s.memo[key] = &memoVal
		err := p.Parse(dest, s, opts)
		s.Logf("XX %T, memoVal: %v", dest, memoVal)
		if err == nil {
			memoVal.err = nil
			memoVal.pos = s.Save()
			i := 0
			for memoVal.accessed {
				i++
				memoVal.accessed = false
				s.Restore(pos)
				if p.Parse(dest, s, opts) != nil {
					s.Restore(memoVal.pos)
					break
				}
				memoVal.pos = s.Save()
			}
		} else {
			memoVal.err = err
			s.MergeError(err)
		}
		memoVal.rec = false
		s.keyStack = s.keyStack[:len(s.keyStack)-1]
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

var errRecursion = errors.New("recursion")

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
	return fmt.Sprintf("%s: %s with value %q: %s", e.Token.Position(), e.Token.Type(), e.Token.Value(), hint)
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
