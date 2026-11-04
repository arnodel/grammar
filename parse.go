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
	extra    any // from Memoisable.MemoKey
}

type memoValue struct {
	pos      int
	val      any
	err      *ParseError
	rec      bool
	accessed bool
}

type memoTable map[memoKey]*memoValue

type ParserState struct {
	TokenStream
	memo     memoTable
	keyStack []memoKey
	lastErr  *ParseError
	logger   *log.Logger
	exhaust  bool
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
//
// A Parser that also implements Memoisable has its results cached (see
// Memoisable); one that does not is parsed afresh every time.
type Parser interface {
	Parse(r any, s *ParserState, opts TokenOptions) *ParseError
}

// A Memoisable rule has the result of its Parse method cached by stream
// position (packrat parsing), which also makes left recursion through it
// work.  For the cache to be valid, the outcome of Parse must be fully
// determined by the rule type, the stream position and the key returned by
// MemoKey; a rule for which that is not true must not implement Memoisable
// and is then parsed afresh every time.
//
// Seq, OneOf and the OneOfN types implement it, returning a nil key, as they
// ignore the token options passed to Parse; a struct rule embedding one of
// them inherits the method, so if it overrides Parse the override must obey
// the same rule.  A custom rule whose result depends on the token options
// should return opts.Key().
//
// Note that only memoised rules can be the head of a left-recursive cycle.
type Memoisable interface {
	MemoKey(opts TokenOptions) any
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

// Exhaust makes Parse fail unless the whole token stream is consumed.  Without
// it, Parse succeeds as soon as the rule matches a prefix of the input, so
// e.g. an expression grammar happily parses `1 +` as `1`.  With it, leftover
// input yields a *ParseError whose Err is ErrNotExhausted, unless the parser
// got further while trying to match more, in which case that (usually more
// helpful) error is returned instead.
var Exhaust ParseOption = func(s *ParserState) {
	s.exhaust = true
}

// ErrNotExhausted is the Err of the *ParseError returned by Parse with the
// Exhaust option when some input is left after the rule matched.
var ErrNotExhausted = errors.New("expected end of input")

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
	if state.exhaust {
		return state.checkExhausted(dest)
	}
	return nil
}

// checkExhausted returns an error unless the next token is EOF.  Tokens the
// top-level rule drops (its "drop" tag) are skipped first, so trailing
// dropped tokens do not count as leftover input.
func (s *ParserState) checkExhausted(dest any) *ParseError {
	if tp := reflect.TypeOf(dest); tp != nil && tp.Kind() == reflect.Ptr {
		if ruleDef, err := getRuleDef(tp.Elem()); err == nil {
			ruleDef.DropOptions.DropMatchingNextTokens(s)
		}
	}
	pos := s.Save()
	tok := s.Next()
	if tok.Type() == EOF.TokType {
		return nil
	}
	s.Restore(pos)
	return s.lastErr.Merge(&ParseError{Err: ErrNotExhausted, Token: tok, Pos: pos})
}

// ParseWithOptions is the same as Parse but the ParseOptions are explicitely
// given (this is mostly used by the parser generator).
func ParseWithOptions(dest any, s *ParserState, opts TokenOptions) *ParseError {
	m, ok := dest.(Memoisable)
	if !ok {
		return parseUnmemoized(dest, s, opts)
	}
	pos := s.Save()
	key := memoKey{pos: pos, ruleType: reflect.TypeOf(dest), extra: m.MemoKey(opts)}
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
			for memoVal.accessed {
				// Grow the seed: re-parse with the last result as the
				// left-recursive reference, while that gets further.
				seed := reflect.New(reflect.TypeOf(dest).Elem())
				seed.Elem().Set(reflect.ValueOf(dest).Elem())
				memoVal.val = seed.Interface()
				memoVal.accessed = false
				s.Restore(pos)
				reflect.ValueOf(dest).Elem().SetZero()
				if p.Parse(dest, s, opts) != nil || s.Save() <= memoVal.pos {
					reflect.ValueOf(dest).Elem().Set(seed.Elem())
					s.Restore(memoVal.pos)
					break
				}
				memoVal.pos = s.Save()
			}
			memoVal.val = dest
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
	if len(e.TokenParseOptions) != 0 {
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
	if e.Err != nil {
		if hint != "" {
			hint += "; "
		}
		hint += e.Err.Error()
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
	if e == e2 {
		return e
	}
	// A fresh slice (append could write into e's array, shared with other
	// errors), each option once (merging errors that already share options
	// would otherwise multiply them with every level of nesting).
	opts := make([]TokenParseOptions, 0, len(e.TokenParseOptions)+len(e2.TokenParseOptions))
	seen := make(map[TokenParseOptions]bool, cap(opts))
	for _, list := range [][]TokenParseOptions{e.TokenParseOptions, e2.TokenParseOptions} {
		for _, o := range list {
			if !seen[o] {
				seen[o] = true
				opts = append(opts, o)
			}
		}
	}
	mergedErr := mergeErrs(e.Err, e2.Err)
	if mergedErr == errRecursion && len(opts) > 0 {
		// The expected tokens are the useful message; the marker is not.
		mergedErr = nil
	}
	return &ParseError{
		Err:               mergedErr,
		Token:             e.Token,
		TokenParseOptions: opts,
		Pos:               e.Pos,
	}
}

// mergeErrs picks the Err to keep when merging two ParseErrors at the same
// position: the first one that is not the internal left-recursion marker,
// which is only meaningful to the memoisation and should not reach users
// when there is anything better to say.
func mergeErrs(err1, err2 error) error {
	switch {
	case err1 != nil && err1 != errRecursion:
		return err1
	case err2 != nil && err2 != errRecursion:
		return err2
	case err1 != nil:
		return err1
	default:
		return err2
	}
}

func parseUnmemoized(dest any, s *ParserState, opts TokenOptions) *ParseError {
	p, ok := dest.(Parser)
	if !ok {
		panic(fmt.Sprintf("invalid type for rule %#v", dest))
	}
	err := p.Parse(dest, s, opts)
	if err != nil {
		s.MergeError(err)
	}
	return err
}
