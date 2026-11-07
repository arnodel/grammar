package grammar

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// A FirstSet describes the tokens a rule can start with.  A OneOf uses the
// first sets of its alternatives to skip, without allocating or parsing them,
// those that cannot start with the next token; Seq does the same with its
// optional and repeated fields.
//
// First sets are computed from the rule definitions (struct tags).  A rule
// that may match without consuming a token, or a custom rule (one with its
// own Parse method that is not a OneOfN), is never skipped.
type FirstSet struct {
	any      bool                // unknown: any token may start the rule
	nullable bool                // the rule may match without consuming a token
	opts     []TokenParseOptions // else the first token matches one of these...
	drop     []TokenParseOptions // ...or is one of these, which the rule drops
}

var (
	anyFirst      = FirstSet{any: true}
	nullableFirst = FirstSet{nullable: true}
)

// firstSetsEnabled turns the skipping off, for tests comparing parsing with
// and without it.
var firstSetsEnabled = true

// Accepts reports whether a rule with this first set may start with tok.
func (f FirstSet) Accepts(tok Token) bool {
	if f.any || f.nullable || !firstSetsEnabled {
		return true
	}
	return matchesAny(f.opts, tok) || matchesAny(f.drop, tok)
}

// check returns nil if a rule with this first set may start with the next
// token of s; otherwise it records that the tokens of the first set were
// expected there (as parsing the rule would have) and returns the error.
func (f FirstSet) check(s *ParserState) *ParseError {
	if f.any || f.nullable || !firstSetsEnabled {
		return nil
	}
	pos := s.Save()
	tok := s.Next()
	s.Restore(pos)
	if matchesAny(f.opts, tok) || matchesAny(f.drop, tok) {
		return nil
	}
	return s.recordFailure(pos, tok, f.opts, nil)
}

// checkAt is like check for the token tok at position pos, which the stream
// is not at: used for the continuation of a left-recursive alternative, which
// must start with the token after the seed.
func (f FirstSet) checkAt(s *ParserState, pos int, tok Token) *ParseError {
	if f.any || f.nullable || !firstSetsEnabled {
		return nil
	}
	if matchesAny(f.opts, tok) || matchesAny(f.drop, tok) {
		return nil
	}
	return s.recordFailure(pos, tok, f.opts, nil)
}

func matchesAny(opts []TokenParseOptions, tok Token) bool {
	for _, o := range opts {
		if o.matches(tok) {
			return true
		}
	}
	return false
}

func (f FirstSet) union(g FirstSet) FirstSet {
	return FirstSet{
		any:      f.any || g.any,
		nullable: f.nullable || g.nullable,
		opts:     unionOptions(f.opts, g.opts),
		drop:     unionOptions(f.drop, g.drop),
	}
}

func unionOptions(a, b []TokenParseOptions) []TokenParseOptions {
	if len(b) == 0 {
		return a
	}
	if len(a) == 0 {
		return b
	}
	res := make([]TokenParseOptions, 0, len(a)+len(b))
	res = append(res, a...)
	for _, o := range b {
		if !containsOption(res, o) {
			res = append(res, o)
		}
	}
	return res
}

// sameSize reports whether two first sets computed for the same type in
// successive fixpoint iterations are equal; as the sets only grow, comparing
// sizes is enough.
func (f FirstSet) sameSize(g FirstSet) bool {
	return f.any == g.any && f.nullable == g.nullable && len(f.opts) == len(g.opts) && len(f.drop) == len(g.drop)
}

func (f FirstSet) isEmpty() bool {
	return !f.any && !f.nullable && len(f.opts) == 0 && len(f.drop) == 0
}

// alternatives is implemented by the OneOfN rules (and promoted to the structs
// embedding them).
type alternatives interface {
	alternativeTypes() []reflect.Type
}

var (
	alternativesType = reflect.TypeFor[alternatives]()
	simpleTokenType  = reflect.TypeFor[SimpleToken]()
	matchType        = reflect.TypeFor[Match]()
	emptyType        = reflect.TypeFor[Empty]()
	tokType          = reflect.TypeFor[Tok]()
)

// firstSets caches the first set of each rule type once computed.
var firstSets atomicMap[reflect.Type, FirstSet]

// oneOfFirsts caches, for each OneOfN type, the first sets of its
// alternatives.
var oneOfFirsts atomicMap[reflect.Type, []FirstSet]

// firstOfType returns the first set of the rule type t.
func firstOfType(t reflect.Type) FirstSet {
	if f, ok := firstSets.load(t); ok {
		return f
	}
	newRuleBuilder().build(t)
	f, _ := firstSets.load(t)
	return f
}

// firstOfField returns the first set of a field of type t parsed with the
// given token options: the token types in this package match a token
// against the options, other types ignore them.
func firstOfField(t reflect.Type, opts TokenOptions, firstOf func(reflect.Type) FirstSet) FirstSet {
	switch t {
	case simpleTokenType, matchType, tokType:
		if len(opts.TokenParseOptions) == 0 {
			return nullableFirst // matches without consuming
		}
		return FirstSet{opts: opts.TokenParseOptions}
	}
	return firstOf(t)
}

// alternativeFirsts returns the first sets of the alternatives of a OneOfN
// rule.
func alternativeFirsts(o alternatives) []FirstSet {
	t := reflect.TypeOf(o)
	if firsts, ok := oneOfFirsts.load(t); ok {
		return firsts
	}
	alts := o.alternativeTypes()
	firsts := make([]FirstSet, len(alts))
	for i, alt := range alts {
		firsts[i] = firstOfType(alt)
	}
	oneOfFirsts.store(t, firsts)
	return firsts
}

// alternativeTypesOf returns the alternatives of t if it is a OneOfN or Sum
// rule or embeds one.
func alternativeTypesOf(t reflect.Type) ([]reflect.Type, bool) {
	if t.Kind() != reflect.Struct {
		return nil, false
	}
	if reflect.PointerTo(t).Implements(alternativesType) {
		return reflect.New(t).Interface().(alternatives).alternativeTypes(), true
	}
	return nil, false
}

// A ruleBuilder builds the rule definitions and first sets of all the types
// reachable from a root type that are not cached yet, and publishes them
// together.  Until then they live in the builder, so a definition is never
// visible to other goroutines before it is complete.
type ruleBuilder struct {
	defs   map[reflect.Type]ruleDefOrError
	firsts map[reflect.Type]FirstSet
}

func newRuleBuilder() *ruleBuilder {
	return &ruleBuilder{defs: map[reflect.Type]ruleDefOrError{}, firsts: map[reflect.Type]FirstSet{}}
}

// def returns the definition of t, from the cache or built here.
func (b *ruleBuilder) def(t reflect.Type) (*RuleDef, error) {
	if d, ok := ruleDefs.load(t); ok {
		return d.ruleDef, d.err
	}
	d, ok := b.defs[t]
	if !ok {
		d.ruleDef, d.err = calcRuleDef(t)
		b.defs[t] = d
	}
	return d.ruleDef, d.err
}

// children returns the rule types the first set of t depends on.
func (b *ruleBuilder) children(t reflect.Type) []reflect.Type {
	if alts, ok := alternativeTypesOf(t); ok {
		return alts
	}
	def, err := b.def(t)
	if err != nil {
		return nil
	}
	var res []reflect.Type
	for _, f := range def.Fields {
		res = append(res, f.BaseType)
	}
	return res
}

// computeFirst computes the first set of t from those of its children.
func (b *ruleBuilder) computeFirst(t reflect.Type, firstOf func(reflect.Type) FirstSet) FirstSet {
	switch t {
	case simpleTokenType, matchType, tokType, emptyType:
		return nullableFirst // with no options to match, these consume nothing
	}
	if alts, opts, ok := sumAlternatives(t); ok {
		var res FirstSet
		if d, ok := reflect.New(t).Interface().(sumDeclarer); ok {
			res.drop = d.sumDecl().DropOptions.TokenParseOptions
		}
		for i, alt := range alts {
			res = res.union(firstOfField(alt, opts[i], firstOf))
		}
		return res
	}
	def, err := b.def(t)
	if err != nil {
		return anyFirst // a custom rule
	}
	res := FirstSet{drop: def.DropOptions.TokenParseOptions}
	if def.OneOf {
		for _, f := range def.Fields {
			res = res.union(firstOfField(f.BaseType, f.TokenOptions, firstOf))
		}
		return res
	}
	return seqFirst(def, 0, firstOf)
}

// seqFirst is the first set of the fields of a Seq (or Tok rule) from index
// from: the fields that may consume nothing let the next one start the rule
// too; a lookahead (no-consume) field constrains the first token without
// consuming it, so it ends the walk as well.
func seqFirst(def *RuleDef, from int, firstOf func(reflect.Type) FirstSet) FirstSet {
	res := FirstSet{drop: def.DropOptions.TokenParseOptions}
	for _, f := range def.Fields[from:] {
		first := firstOfField(f.BaseType, f.TokenOptions, firstOf)
		res = res.union(FirstSet{any: first.any, opts: first.opts, drop: first.drop})
		switch {
		case f.Pointer, f.Array && f.Min == 0, first.nullable:
			continue // may match nothing
		}
		return res
	}
	res.nullable = true
	return res
}

// build builds the definitions and first sets of root and of all the rule
// types reachable from it that are not cached yet.  The first sets are the
// least fixpoint of computeFirst over those types (so recursion, left or
// not, is fine); they then fill in the field first sets of the definitions,
// and everything is published.
func (b *ruleBuilder) build(root reflect.Type) {
	env := map[reflect.Type]FirstSet{}
	var unknown []reflect.Type
	var visit func(t reflect.Type)
	visit = func(t reflect.Type) {
		if _, seen := env[t]; seen {
			return
		}
		if f, ok := firstSets.load(t); ok {
			env[t] = f
			return
		}
		env[t] = FirstSet{}
		unknown = append(unknown, t)
		for _, c := range b.children(t) {
			visit(c)
		}
	}
	visit(root)
	b.def(root) // so that it is cached even if it is not a struct rule
	lookup := func(t reflect.Type) FirstSet { return env[t] }
	for changed := true; changed; {
		changed = false
		for _, t := range unknown {
			f := b.computeFirst(t, lookup)
			if !f.sameSize(env[t]) {
				env[t] = f
				changed = true
			}
		}
	}
	for _, t := range unknown {
		f := env[t]
		if f.isEmpty() {
			f = anyFirst // cannot match anything, or only through itself
		}
		env[t] = f
		b.firsts[t] = f
	}
	for t, d := range b.defs {
		if d.err != nil {
			continue
		}
		d.ruleDef.firsts = make([]FirstSet, len(d.ruleDef.Fields))
		for i, f := range d.ruleDef.Fields {
			d.ruleDef.firsts[i] = firstOfField(f.BaseType, f.TokenOptions, lookup)
		}
		if d.ruleDef.OneOf {
			b.leftRecursion(t, d.ruleDef, lookup)
		}
	}
	// First sets before definitions: a definition found in the cache has its
	// type's first set there too.
	firstSets.storeAll(b.firsts)
	ruleDefs.storeAll(b.defs)
}

type leftRecKind uint8

const (
	notLeftRec      leftRecKind = iota
	directLeftRec               // a Seq rule whose first field is the OneOf rule
	indirectLeftRec             // reaches the OneOf rule first through other rules
)

// LeftRecInfo says whether an alternative of a OneOf rule is left recursive
// (may parse the OneOf rule itself first, at the same position) and, for a
// Seq alternative that starts with it, what may follow that first field.
type LeftRecInfo struct {
	kind    leftRecKind
	cont    FirstSet
	prec    int          // precedence of the alternative (from its rule's prec tag)
	hasPrec bool         // the alternative has a precedence
	family  reflect.Type // the type of its left operand, if direct
}

// Level returns the precedence of the alternative (0 if it has none).
func (lr LeftRecInfo) Level() int { return lr.prec }

// leftRecursion classifies the alternatives of the OneOf rule t.
func (b *ruleBuilder) leftRecursion(t reflect.Type, def *RuleDef, lookup func(reflect.Type) FirstSet) {
	def.leftRec = make([]LeftRecInfo, len(def.Fields))
	for i, f := range def.Fields {
		def.leftRec[i] = b.classify(t, f.BaseType, f.Pointer, lookup)
		if !def.leftRec[i].hasPrec && def.HasPrec {
			def.leftRec[i].prec, def.leftRec[i].hasPrec = def.Prec, true
		}
	}
}

// classify returns the left recursion info of alt as an alternative of the
// OneOf (or Sum) rule t, with the precedence of alt's own rule.
func (b *ruleBuilder) classify(t, alt reflect.Type, pointer bool, lookup func(reflect.Type) FirstSet) LeftRecInfo {
	var info LeftRecInfo
	altDef, err := b.def(alt)
	if err == nil {
		info.prec, info.hasPrec = altDef.Prec, altDef.HasPrec
	}
	if !b.reachesFirst(alt, t, lookup, map[reflect.Type]bool{}) {
		return info
	}
	info.kind = indirectLeftRec
	if err == nil && pointer && !altDef.OneOf && len(altDef.Fields) > 0 {
		if first := altDef.Fields[0]; first.BaseType == t && !first.Pointer && !first.Array {
			info.kind, info.cont, info.family = directLeftRec, seqFirst(altDef, 1, lookup), t
		}
	}
	return info
}

// reachesFirst reports whether parsing u may parse t first, at the same
// position.
func (b *ruleBuilder) reachesFirst(u, t reflect.Type, lookup func(reflect.Type) FirstSet, seen map[reflect.Type]bool) bool {
	if u == t {
		return true
	}
	if seen[u] {
		return false
	}
	seen[u] = true
	for _, c := range b.leftmost(u, lookup) {
		if b.reachesFirst(c, t, lookup, seen) {
			return true
		}
	}
	return false
}

// leftmost returns the rule types that u may parse first.
func (b *ruleBuilder) leftmost(u reflect.Type, lookup func(reflect.Type) FirstSet) []reflect.Type {
	if alts, ok := alternativeTypesOf(u); ok {
		return alts
	}
	def, err := b.def(u)
	if err != nil {
		return nil // a token or custom rule
	}
	var res []reflect.Type
	for _, f := range def.Fields {
		res = append(res, f.BaseType)
		if def.OneOf {
			continue
		}
		first := firstOfField(f.BaseType, f.TokenOptions, lookup)
		if f.Pointer || f.Array && f.Min == 0 || first.nullable {
			continue
		}
		break
	}
	return res
}

// atomicMap is a map that is safe for concurrent use and as cheap to read as
// a plain map, for caches that are written to a few times and read a lot: a
// write copies the map.
type atomicMap[K comparable, V any] struct {
	mu sync.Mutex
	m  atomic.Pointer[map[K]V]
}

func (a *atomicMap[K, V]) load(k K) (V, bool) {
	m := a.m.Load()
	if m == nil {
		var zero V
		return zero, false
	}
	v, ok := (*m)[k]
	return v, ok
}

func (a *atomicMap[K, V]) store(k K, v V) {
	a.storeAll(map[K]V{k: v})
}

// storeAll adds the entries of kvs, in one copy of the map.  An entry already
// present is kept, so that concurrent builders of the same (immutable) values
// agree on which one is published.
func (a *atomicMap[K, V]) storeAll(kvs map[K]V) {
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.m.Load()
	var m map[K]V
	if old == nil {
		m = make(map[K]V, len(kvs))
	} else {
		m = make(map[K]V, len(*old)+len(kvs))
		for k, v := range *old {
			m[k] = v
		}
	}
	for k, v := range kvs {
		if _, present := m[k]; !present {
			m[k] = v
		}
	}
	a.m.Store(&m)
}
