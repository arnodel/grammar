package grammar

import (
	"reflect"
	"sync"
)

// Memoisation (packrat parsing).
//
// The result of parsing a Memoisable rule at a position is kept in the memo
// table, so that parsing it there again is a lookup; a rule is only
// memoised once it is parsed at a position for the second time, as most
// rules never are.  The memo also makes left recursion work: a rule being
// parsed is on the stack (ParserState.keyStack), and a reference to it at
// the same position (a left-recursive one) is answered with its result so
// far, the seed, which is nothing at first.  If a parse made such a
// reference, the rule is parsed again with its result as the seed, as long
// as that gets further (growSeed).

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

// stackEntry is a memoised rule being parsed.
type stackEntry struct {
	pos   int
	key   memoKey
	entry *memoValue // created by a left-recursive reference to an unmemoised rule
	// forgotten is set when the rule's result depends on the seed of a
	// left-recursive rule below it on the stack: once the seed has grown
	// the result is stale, so the entry is dropped when the rule is done.
	forgotten bool
}

// reuse applies the memoised result v of the rule (pos, key) to dest.  A
// rule still being parsed (v.rec) is a left-recursive reference: it parses
// as its seed, the result so far, and is marked as accessed so that the
// seed is grown (see growSeed).
func (s *ParserState) reuse(v *memoValue, dest any, pos int, key memoKey) *ParseError {
	if s.Debug() {
		s.Logf("pos: %d, memoVal: %v", pos, *v)
	}
	if v.rec {
		v.accessed = true
		s.forgetAbove(pos, key)
		if !v.ok {
			return s.failRecursion(pos)
		}
	}
	if v.err != nil {
		return v.err
	}
	s.Restore(v.pos)
	reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(v.val).Elem())
	return nil
}

// forgetAbove marks the rules being parsed above the rule (pos, key) on the
// stack as forgotten: they depend on its seed.
func (s *ParserState) forgetAbove(pos int, key memoKey) {
	for i := len(s.keyStack) - 1; i >= 0; i-- {
		k := &s.keyStack[i]
		if k.pos == pos && k.key == key {
			return
		}
		k.forgotten = true
	}
	panic("grammar: left-recursive reference to a rule that is not being parsed")
}

// leftRecursive reports whether a reference to the rule (pos, key), which
// has no memo entry, is left recursive: the same rule is being parsed at
// the same position.  The rules being parsed at pos are at the top of the
// stack, so the caller need only call this when the top entry is at pos.
// If it is, an entry is created for the rule, as a failing seed, and the
// rules above it are forgotten (see forgetAbove).
func (s *ParserState) leftRecursive(pos int, key memoKey) bool {
	for i := len(s.keyStack) - 1; i >= 0 && s.keyStack[i].pos == pos; i-- {
		if s.keyStack[i].key != key {
			continue
		}
		v := s.memo.add(pos, key)
		v.rec, v.accessed = true, true
		s.keyStack[i].entry = v
		s.forgetAbove(pos, key)
		return true
	}
	return false
}

// growSeed parses the left-recursive rule dest at pos again with its result
// so far as the value of the references to itself (the seed), as long as
// that gets further; the last result that did stands.
func (s *ParserState) growSeed(p Parser, dest any, opts TokenOptions, bound Bound, pos int, v *memoValue) {
	for v.accessed {
		seed := reflect.New(reflect.TypeOf(dest).Elem())
		seed.Elem().Set(reflect.ValueOf(dest).Elem())
		v.val = seed.Interface()
		v.accessed = false
		s.Restore(pos)
		reflect.ValueOf(dest).Elem().SetZero()
		s.prec.bound = bound
		if s.fold(p.Parse(dest, s, opts)) != nil || s.Save() <= v.pos {
			reflect.ValueOf(dest).Elem().Set(seed.Elem())
			s.Restore(v.pos)
			break
		}
		v.pos = s.Save()
	}
	v.val = dest
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
