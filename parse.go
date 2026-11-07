package grammar

import (
	"errors"
	"fmt"
	"log"
	"reflect"
)

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

// ParserState is the state of a parse: the token stream and what the rules
// share while parsing it.  Rules get it as the argument of their Parse
// method.
type ParserState struct {
	TokenStream
	memo     memoTable    // results by position, see memo.go
	keyStack []stackEntry // the memoised rules being parsed, innermost last
	fail     ParseError   // the furthest failure so far, see failure.go
	prec     precState    // precedences passed between rules, see prec.go
	logger   *log.Logger
	exhaust  bool
}

// Peek returns the next token without consuming it.
func (s *ParserState) Peek() Token {
	pos := s.Save()
	tok := s.Next()
	s.Restore(pos)
	return tok
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
	state := &ParserState{TokenStream: s}
	for _, opt := range opts {
		opt(state)
	}
	err := ParseWithOptions(dest, state, TokenOptions{})
	if err == nil && state.exhaust {
		err = state.checkExhausted(dest)
	}
	if err == nil {
		return nil
	}
	if state.fail.Token == nil {
		// Cannot happen with the rules in this package: every failure is
		// recorded.  Be safe with a custom rule returning an unrecorded error.
		return err
	}
	return state.result()
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
	return s.recordFailure(pos, tok, nil, ErrNotExhausted)
}

// ParseWithOptions parses the rule dest at the current position, with the
// token options of the field it is parsed for.  It is what rules call to
// parse their fields (Parse calls it for the root rule): it takes the
// precedence bound set for the rule (see prec.go) and, if the rule is
// Memoisable, looks its result up in the memo or records it there (see
// memo.go).
func ParseWithOptions(dest any, s *ParserState, opts TokenOptions) *ParseError {
	p, ok := dest.(Parser)
	if !ok {
		panic(fmt.Sprintf("invalid type for rule %#v", dest))
	}
	bound := s.prec.take()
	m, ok := dest.(Memoisable)
	if !ok {
		s.prec.bound = bound
		return s.fold(p.Parse(dest, s, opts))
	}
	pos := s.Save()
	key := memoKey{ruleType: reflect.TypeOf(dest), extra: m.MemoKey(opts), bound: bound}
	if v := s.memo.get(pos, key); v != nil {
		return s.reuse(v, dest, pos, key)
	}
	if n := len(s.keyStack); n > 0 && s.keyStack[n-1].pos == pos && s.leftRecursive(pos, key) {
		return s.failRecursion(pos)
	}
	// Parse the rule and record the result if it is memoised here: a rule
	// gets an entry when it is parsed at a position for the second time (so
	// that rules never retried cost nothing), or when it turns out to be
	// left recursive, in which case the result is a seed to grow.
	if s.Debug() {
		s.Logf("===> %T, %v", p, opts)
	}
	s.keyStack = append(s.keyStack, stackEntry{pos: pos, key: key})
	top := len(s.keyStack) - 1
	var v *memoValue
	if key.extra != nil || s.memo.parsedBefore(pos, ruleID(key.ruleType)) {
		v = s.memo.add(pos, key)
		v.val = dest
		v.rec = true // until done, a reference to this rule is left recursive
	}
	s.prec.bound = bound
	err := s.fold(p.Parse(dest, s, opts))
	if v == nil {
		v = s.keyStack[top].entry // created by a left-recursive reference, if any
	}
	if v != nil {
		if err != nil {
			v.err = err
		} else {
			v.ok, v.pos, v.val = true, s.Save(), dest
			if v.accessed {
				s.growSeed(p, dest, opts, bound, pos, v)
			}
		}
		v.rec = false
		if s.keyStack[top].forgotten {
			s.memo.delete(pos, key) // stale once the seed below has grown
		}
	}
	s.keyStack = s.keyStack[:top] // after growing: forgetAbove needs the entry
	if s.Debug() {
		s.Logf("<=== %s", err)
	}
	return err
}
