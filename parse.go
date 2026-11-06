package grammar

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
)

// memoKey identifies a memoised rule at a given stream position.
type memoKey struct {
	ruleType reflect.Type
	extra    any   // from Memoisable.MemoKey
	bound    Bound // precedence bound the rule was parsed under
}

type memoValue struct {
	key      memoKey
	next     *memoValue  // next entry at the same position
	pos      int         // where the parse ended, if ok
	val      any         // the parsed rule, if ok
	ok       bool        // val and pos hold a result (maybe a seed being grown)
	err      *ParseError // the error, if failed
	rec      bool        // being parsed: a reference to it is left recursive
	accessed bool        // referenced while being parsed
}

// memoTable holds the memo entries by stream position.  Positions are small
// dense integers and few rules are tried at each, so a slice of short lists
// beats a map, and entries are allocated in blocks rather than one by one.
type memoTable struct {
	heads   []*memoValue // by position
	entries blocks[memoValue]
	// seen has a bit per (position, rule type): set once the rule has been
	// parsed at the position.  A rule is only memoised when it is parsed
	// there a second time, so rules that are never retried cost no entry.
	seen   []uint64 // stride words per position
	stride int
}

// parsedBefore marks rule id as parsed at pos and reports whether it already
// was.
func (t *memoTable) parsedBefore(pos, id int) bool {
	w, bit := id/64, uint64(1)<<(id%64)
	if w >= t.stride {
		t.widen(w + 1)
	}
	if pos >= len(t.seen)/t.stride {
		seen := make([]uint64, max(pos+1, 2*len(t.seen)/t.stride, 64)*t.stride)
		copy(seen, t.seen)
		t.seen = seen
	}
	i := pos*t.stride + w
	if t.seen[i]&bit != 0 {
		return true
	}
	t.seen[i] |= bit
	return false
}

// widen lays seen out again with more words per position.
func (t *memoTable) widen(stride int) {
	n := 0
	if t.stride > 0 {
		n = len(t.seen) / t.stride
	}
	seen := make([]uint64, n*stride)
	for p := 0; p < n; p++ {
		copy(seen[p*stride:], t.seen[p*t.stride:(p+1)*t.stride])
	}
	t.seen, t.stride = seen, stride
}

// ruleIDs numbers the rule types, for memoTable.seen.
var (
	ruleIDs    atomicMap[reflect.Type, int]
	ruleIDMu   sync.Mutex
	nextRuleID int
)

func ruleID(t reflect.Type) int {
	if id, ok := ruleIDs.load(t); ok {
		return id
	}
	ruleIDMu.Lock()
	defer ruleIDMu.Unlock()
	if id, ok := ruleIDs.load(t); ok {
		return id
	}
	id := nextRuleID
	nextRuleID++
	ruleIDs.store(t, id)
	return id
}

func (t *memoTable) get(pos int, key memoKey) *memoValue {
	if pos >= len(t.heads) {
		return nil
	}
	for v := t.heads[pos]; v != nil; v = v.next {
		if v.key == key {
			return v
		}
	}
	return nil
}

// add inserts a new entry for key at pos and returns it.
func (t *memoTable) add(pos int, key memoKey) *memoValue {
	if pos >= len(t.heads) {
		heads := make([]*memoValue, max(pos+1, 2*len(t.heads), 64))
		copy(heads, t.heads)
		t.heads = heads
	}
	v := t.entries.new()
	*v = memoValue{key: key, next: t.heads[pos]}
	t.heads[pos] = v
	return v
}

func (t *memoTable) delete(pos int, key memoKey) {
	if pos >= len(t.heads) {
		return
	}
	for p := &t.heads[pos]; *p != nil; p = &(*p).next {
		if (*p).key == key {
			*p = (*p).next
			return
		}
	}
}

// stackEntry is a rule being parsed.
type stackEntry struct {
	pos   int
	key   memoKey
	entry *memoValue // created by a left-recursive reference to an unmemoised rule
}

type ParserState struct {
	TokenStream
	memo     memoTable
	keyStack []stackEntry
	fail     ParseError // the furthest failure so far, see recordFailure
	logger   *log.Logger
	exhaust  bool

	// Operator precedence (see the "prec" tag).  prec is the bound set for
	// the next rule parsed; ParseWithOptions moves it to bound for that rule
	// to read.  pending is the level of the OneOf alternative being tried,
	// for the Seq rule parsing it to apply to its fields.
	prec    Bound
	bound   Bound
	pending PendingPrec

	// Left recursion by request (see Alternatives): seedEnd is set for a
	// rule being re-parsed to grow its seed, growRequested by a rule that
	// skipped its left-recursive alternatives.
	seedEnd       int
	growRequested bool
}

// A Bound is a precedence bound: an operand is parsed under the level of the
// operator it belongs to, and only the sum rule of that operator's family
// (the type of its left operand) is constrained by it; a bound with no
// family constrains whichever sum rule it reaches.
type Bound struct {
	Level  int
	Family reflect.Type
}

// PendingPrec is the precedence of the OneOf alternative being parsed, for
// the Seq rule parsing it: its first field (the left operand) is parsed
// under the bound the OneOf was entered with, the others under the level.
type PendingPrec struct {
	level  int
	family reflect.Type
	outer  Bound
	set    bool
}

// Prec returns the bound for field i of the rule, none if there is none.
func (p PendingPrec) Prec(i int) Bound {
	switch {
	case !p.set:
		return Bound{}
	case i == 0:
		return p.outer
	default:
		return Bound{Level: p.level, Family: p.family}
	}
}

// TakePending returns the pending precedence for the Seq rule being parsed
// and clears it.
func (s *ParserState) TakePending() PendingPrec {
	p := s.pending
	s.pending.set = false
	return p
}

// TakePendingOr is TakePending for a rule with its own precedence (has): if
// no precedence is pending, the fields after the first are bound to level.
func (s *ParserState) TakePendingOr(level int, has bool) PendingPrec {
	p := s.TakePending()
	if !p.set && has {
		// Entered on its own: its operands belong to the same family as
		// the rule itself was parsed for.
		p = PendingPrec{level: level, family: s.bound.Family, outer: s.bound, set: true}
	}
	return p
}

// SetPrec sets the precedence bound for the next rule parsed.
func (s *ParserState) SetPrec(b Bound) { s.prec = b }

// TokenAt returns the token at position pos without moving the stream.
func (s *ParserState) TokenAt(pos int) Token {
	cur := s.Save()
	s.Restore(pos)
	tok := s.Next()
	s.Restore(cur)
	return tok
}

// Alternatives decides which alternatives of a OneOf rule to try: those whose
// precedence is above the bound; when the rule is being re-parsed to grow
// the seed of a left recursion, only the left-recursive ones, and only if
// the token after the seed can follow their left operand; otherwise only
// the others, the left-recursive ones being bound to fail on their own
// reference (Done then asks for the rule to be re-parsed).
type Alternatives struct {
	s                *ParserState
	t                reflect.Type // the sum rule type
	bound            Bound        // as entered with
	level            int          // the level that constrains t: the bound's if it is for t
	seedEnd          int
	afterSeed        Token
	growing, skipped bool
}

// Alternatives starts choosing the alternatives of the OneOf rule of type t
// being parsed; call it before anything else.
func (s *ParserState) Alternatives(t reflect.Type) Alternatives {
	a := Alternatives{s: s, t: t, bound: s.bound, seedEnd: s.seedEnd, growing: s.seedEnd >= 0}
	if a.bound.Family == nil || a.bound.Family == t {
		a.level = a.bound.Level
	}
	if a.growing {
		a.afterSeed = s.TokenAt(a.seedEnd)
	}
	return a
}

// AlternativesOf is Alternatives for the rule type T, for generated code.
func AlternativesOf[T any](s *ParserState) Alternatives {
	return s.Alternatives(reflect.TypeFor[T]())
}

// Level returns the precedence level that constrains the alternatives.
func (a *Alternatives) Level() int { return a.level }

// PendingFor returns the pending precedence for parsing the fields of the
// alternative lr after its left operand.
func (a *Alternatives) PendingFor(lr LeftRecInfo) PendingPrec {
	family := lr.family
	if family == nil {
		family = a.t // a prefix operator binds an operand of this rule's family
	}
	return PendingPrec{level: lr.prec, family: family, outer: a.bound, set: true}
}

// Try reports whether to try an alternative, given its left recursion info,
// precedence and first set; when not, the error has the expected tokens
// recorded.
func (a *Alternatives) Try(lr LeftRecInfo, first FirstSet) (bool, *ParseError) {
	if lr.kind != notLeftRec && lr.hasPrec && a.level >= lr.prec {
		// An operator that does not bind tightly enough.  A prefix operator
		// (not left recursive) always applies: its precedence only binds
		// its operand.
		return false, nil
	}
	switch lr.kind {
	case directLeftRec:
		if !a.growing {
			a.skipped = true
			return false, nil
		}
		if err := lr.cont.checkAt(a.s, a.seedEnd, a.afterSeed); err != nil {
			return false, err
		}
		return true, nil
	case notLeftRec:
		if a.growing {
			return false, nil // would give the seed again at best
		}
	}
	if err := first.check(a.s); err != nil {
		return false, err
	}
	return true, nil
}

// Enter sets the precedence for the alternative about to be parsed.
func (a *Alternatives) Enter(lr LeftRecInfo) {
	if lr.hasPrec {
		a.s.pending = a.PendingFor(lr)
		a.s.prec = a.bound
	}
}

// Leave is called after an alternative was parsed, whatever the outcome.
func (a *Alternatives) Leave() { a.s.pending.set = false }

// Done is called when the OneOf rule is done, whatever the outcome.
func (a *Alternatives) Done() {
	if a.skipped {
		a.s.growRequested = true
	}
}

// Fail records that parsing failed at stream position pos, on token tok, for
// the given reason, and returns the error for the Parse method to return.  A
// custom rule uses it to report a failure other than a token mismatch (which
// TokenOptions.MatchNextToken records itself):
//
//	pos := s.Save()
//	tok, err := opts.MatchNextToken(s)
//	if err != nil {
//		return err
//	}
//	if !valid(tok) {
//		return s.Fail(pos, tok, errors.New("invalid literal"))
//	}
//
// Only the furthest failure of a parse is recorded (with every reason given
// for it), as it is where the input actually went wrong; earlier ones are
// dropped.  The *ParseError returned is the same for all failures of a parse
// and describes the furthest one, so a rule should not expect it to describe
// its own failure; it only means "failed".
func (s *ParserState) Fail(pos int, tok Token, err error) *ParseError {
	return s.recordFailure(pos, tok, nil, err)
}

// recordFailure merges a failure into s.fail, the furthest failure record,
// and returns a pointer to the record, which is what every failing rule
// returns.  This way no error is allocated during parsing; Parse returns a
// copy of the record at the end.
func (s *ParserState) recordFailure(pos int, tok Token, opts []TokenParseOptions, err error) *ParseError {
	f := &s.fail
	switch {
	case f.Token == nil || pos > f.Pos:
		f.Pos = pos
		f.Token = tok
		f.TokenParseOptions = append(f.TokenParseOptions[:0], opts...)
		f.Err = err
	case pos == f.Pos:
		// Duplicates are removed when the result is read, not here: dozens
		// of options may be recorded at a position, and checking each new
		// one against them was the biggest cost of a failed match.
		f.TokenParseOptions = append(f.TokenParseOptions, opts...)
		if len(f.TokenParseOptions) > maxRecordedOptions {
			f.TokenParseOptions = uniqueOptions(f.TokenParseOptions, f.TokenParseOptions[:0])
		}
		f.Err = mergeErrs(f.Err, err)
	default:
		return f // Not as far as the record: dropped.
	}
	if f.Err == errRecursion && len(f.TokenParseOptions) > 0 {
		// The expected tokens are the useful message; the marker is not.
		f.Err = nil
	}
	return f
}

// beyondRecord reports whether a failure at pos would be dropped by
// recordFailure, so that callers can skip computing its details.
func (s *ParserState) beyondRecord(pos int) bool {
	return s.fail.Token != nil && pos < s.fail.Pos
}

// fold records an error allocated by a custom rule (recognised by not being
// the shared record) and returns the shared record.
func (s *ParserState) fold(err *ParseError) *ParseError {
	if err == nil || err == &s.fail {
		return err
	}
	return s.recordFailure(err.Pos, err.Token, err.TokenParseOptions, err.Err)
}

// result returns the error for Parse to return: a copy of the record, so
// that it does not keep the parser state alive.
func (s *ParserState) result() *ParseError {
	result := s.fail
	if len(result.TokenParseOptions) > 0 {
		result.TokenParseOptions = uniqueOptions(result.TokenParseOptions, nil)
	}
	return &result
}

// maxRecordedOptions is the number of options recorded at the furthest
// position beyond which they are deduplicated, bounding the record.
const maxRecordedOptions = 128

// uniqueOptions appends the options to dst without duplicates, in order.
// dst may be opts[:0] to compact in place.
func uniqueOptions(opts, dst []TokenParseOptions) []TokenParseOptions {
	for i, o := range opts {
		if !containsOption(opts[:i], o) {
			dst = append(dst, o)
		}
	}
	return dst
}

func containsOption(opts []TokenParseOptions, o TokenParseOptions) bool {
	for _, p := range opts {
		if p == o {
			return true
		}
	}
	return false
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

// Peek returns the next token without consuming it.
func (s *ParserState) Peek() Token {
	pos := s.Save()
	tok := s.Next()
	s.Restore(pos)
	return tok
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

// ParseWithOptions is the same as Parse but the ParseOptions are explicitely
// given (this is mostly used by the parser generator).
func ParseWithOptions(dest any, s *ParserState, opts TokenOptions) *ParseError {
	m, ok := dest.(Memoisable)
	if !ok {
		return parseUnmemoized(dest, s, opts)
	}
	p, ok := dest.(Parser)
	if !ok {
		panic(fmt.Sprintf("invalid type for rule %#v", dest))
	}
	pos := s.Save()
	bound := s.prec
	s.prec = Bound{}
	key := memoKey{ruleType: reflect.TypeOf(dest), extra: m.MemoKey(opts), bound: bound}
	memoVal := s.memo.get(pos, key)
	if memoVal != nil {
		if s.Debug() {
			s.Logf("pos: %d, memoVal: %v", pos, *memoVal)
		}
		if memoVal.rec {
			memoVal.accessed = true
			// The rules being parsed above this one depend on its seed, so
			// their entries must not be reused once the seed has grown.
			for i := len(s.keyStack) - 1; ; i-- {
				k := s.keyStack[i]
				if k.pos == pos && k.key == key {
					break
				}
				if s.Debug() {
					s.Logf("Delete %d, %v", i, k)
				}
				s.memo.delete(k.pos, k.key)
			}
			if !memoVal.ok {
				return s.failRecursion(pos)
			}
		}
		if memoVal.err != nil {
			return memoVal.err
		}
		if memoVal.ok {
			s.Restore(memoVal.pos)
			if s.Debug() {
				s.Logf("dest: %v, memoVal: %v", dest, *memoVal)
			}
			reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(memoVal.val).Elem())
			return nil
		}
	} else {
		// Not memoised.  The rules being parsed at this very position are
		// at the top of the stack; if this one is among them, the reference
		// is left recursive: its entry is created now, as a failing seed.
		for i := len(s.keyStack) - 1; i >= 0 && s.keyStack[i].pos == pos; i-- {
			if s.keyStack[i].key != key {
				continue
			}
			for j := len(s.keyStack) - 1; j > i; j-- {
				s.memo.delete(s.keyStack[j].pos, s.keyStack[j].key)
			}
			memoVal = s.memo.add(pos, key)
			memoVal.rec = true
			memoVal.accessed = true
			s.keyStack[i].entry = memoVal
			return s.failRecursion(pos)
		}
	}

	if s.Debug() {
		s.Logf("===> %T, %v", p, opts)
	}
	s.keyStack = append(s.keyStack, stackEntry{pos: pos, key: key})
	// A rule parsed for the first time at pos is not memoised unless it
	// turns out to be left recursive; parsed there again, it is.
	if key.extra != nil || s.memo.parsedBefore(pos, ruleID(key.ruleType)) {
		memoVal = s.memo.add(pos, key)
		memoVal.val = dest
		memoVal.rec = true // Until done, a reference to this rule is left recursive.
	}
	s.bound = bound
	s.seedEnd = -1
	s.growRequested = false
	err := s.fold(p.Parse(dest, s, opts))
	top := len(s.keyStack) - 1
	if memoVal == nil && s.keyStack[top].entry != nil {
		memoVal = s.keyStack[top].entry // created by a left-recursive reference
		memoVal.val = dest
	}
	if s.growRequested {
		s.growRequested = false
		if err == nil {
			if memoVal == nil {
				memoVal = s.memo.add(pos, key)
				memoVal.rec = true
			}
			memoVal.val = dest
			memoVal.accessed = true
		}
	}
	if memoVal == nil {
		s.keyStack = s.keyStack[:top]
		if s.Debug() {
			s.Logf("<=== %s (not memoised)", err)
		}
		return err
	}
	if s.Debug() {
		s.Logf("XX %T, memoVal: %v", dest, *memoVal)
	}
	if err == nil {
		memoVal.ok = true
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
			s.bound = bound
			s.seedEnd = int(memoVal.pos)
			s.growRequested = false
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
	}
	memoVal.rec = false
	s.keyStack = s.keyStack[:top]
	if s.Debug() {
		s.Logf("<=== %s", err)
	}
	return err
}

// failRecursion is the failure of a left-recursive reference to a rule
// being parsed at pos (see Memoisable); the reference parses as its seed,
// which is nothing to start with.
func (s *ParserState) failRecursion(pos int) *ParseError {
	if s.beyondRecord(pos) {
		return &s.fail
	}
	tok := s.Next()
	s.Restore(pos)
	return s.recordFailure(pos, tok, nil, errRecursion)
}

// A ParseError describes where a parse failed: the position and token, the
// tokens that were expected there (TokenParseOptions, from "tok" tags) and
// any other reason (Err).  Parse returns the furthest failure of the parse,
// which is where the input went wrong.
//
// During a parse, the *ParseError returned by a failing rule is shared by all
// failures and describes the furthest one so far, not necessarily the rule's
// own failure (see ParserState.Fail).
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

// summariseOptions returns the token types and the token values of the
// options, without duplicates and in the order they were given, which is
// the order the grammar tried them in.
func summariseOptions(opts []TokenParseOptions) (types, values []string) {
	for _, opt := range opts {
		switch {
		case opt.TokenValue != "":
			values = appendUnique(values, opt.TokenValue)
		case opt.TokenType != "":
			types = appendUnique(types, opt.TokenType)
		}
	}
	return types, values
}

func appendUnique(ss []string, s string) []string {
	for _, x := range ss {
		if x == s {
			return ss
		}
	}
	return append(ss, s)
}

// Merge returns the error describing the furthest of the two failures, or
// both if they are at the same position.  Errors returned by the rules of
// this package are all the shared record of the furthest failure (see
// ParserState.Fail), so merging them is a no-op; a custom rule combining
// errors it allocated itself may need it.
func (e *ParseError) Merge(e2 *ParseError) *ParseError {
	if e == nil {
		return e2
	}
	if e2 == nil || e == e2 {
		return e
	}
	if e.Pos > e2.Pos {
		return e
	}
	if e.Pos < e2.Pos {
		return e2
	}
	// A fresh slice (append could write into e's array, shared with other
	// errors), each option once (merging errors that already share options
	// would otherwise multiply them with every level of nesting).
	opts := make([]TokenParseOptions, 0, len(e.TokenParseOptions)+len(e2.TokenParseOptions))
	for _, list := range [][]TokenParseOptions{e.TokenParseOptions, e2.TokenParseOptions} {
		for _, o := range list {
			if !containsOption(opts, o) {
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
	s.bound, s.prec = s.prec, Bound{}
	s.seedEnd = -1
	err := s.fold(p.Parse(dest, s, opts))
	s.growRequested = false
	return err
}
