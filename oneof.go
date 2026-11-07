package grammar

import (
	"fmt"
	"reflect"
)

// Parsing a one-of rule: a struct OneOf, a OneOfN or a Sum.
//
// What is known about the alternatives before parsing is gathered once per
// rule type in a oneOfInfo (nested sums flattened, see sum.go).  Parsing
// then goes:
//
//  1. A Choice filters the alternatives by precedence (Choice.Try) and
//     passes the precedence on to the operator alternatives (Choice.Enter).
//  2. Seed: the alternatives that do not start with the rule itself are
//     tried in order; the first that parses is the value.
//  3. Climb: while the next token can continue the value with one of the
//     alternatives that do start with the rule itself (Dispatch says which),
//     that alternative is parsed with the value as its left operand
//     (parseRest) and becomes the value.  This is precedence climbing.
//
// Climbing needs every left-recursive alternative to be direct (a Seq rule
// starting with the one-of rule, the usual shape of an infix or postfix
// operator).  Otherwise the alternatives are tried in order and left
// recursion is resolved by seed growing in the memo (see memo.go), which is
// general but slower.
//
// The code that cmd/genparse generates for a struct OneOf does the same
// with the same helpers (Choice, Dispatch, RestParser), see compiled.go.

// A Choice is the parsing of a one-of rule under a precedence bound (see
// prec.go): it says which alternatives to try and passes the precedence on
// to the operator alternatives.
type Choice struct {
	s     *ParserState
	t     reflect.Type // the one-of rule type
	bound Bound        // it is parsed under
	level int          // the level the bound imposes on it
}

// Choice starts parsing the one-of rule of type t; call it before anything
// else in the rule's Parse method.
func (s *ParserState) Choice(t reflect.Type) Choice {
	return Choice{s: s, t: t, bound: s.prec.bound, level: s.prec.bound.constrains(t)}
}

// ChoiceOf is Choice for the rule type T, for generated code.
func ChoiceOf[T any](s *ParserState) Choice {
	return s.Choice(reflect.TypeFor[T]())
}

// Level returns the precedence level that constrains the alternatives.
func (c *Choice) Level() int { return c.level }

// Try reports whether to try an alternative; when not, the error has the
// expected tokens recorded.
func (c *Choice) Try(alt Alternative) (bool, *ParseError) {
	if alt.leftRec != notLeftRec && alt.prec != 0 && c.level >= alt.prec {
		// An operator that does not bind tightly enough.  A prefix operator
		// (not left recursive) always applies: its level only binds its
		// operand.
		return false, nil
	}
	if err := alt.first.check(c.s); err != nil {
		return false, err
	}
	return true, nil
}

// Enter sets the precedence for the alternative about to be parsed: an
// operator gets the pending precedence and is parsed under the bound of
// the one-of rule.
func (c *Choice) Enter(alt Alternative) {
	if alt.prec != 0 {
		c.s.prec.pending = c.PendingFor(alt)
		c.s.prec.next = c.bound
	}
}

// Leave is called after an alternative was parsed, whatever the outcome.
func (c *Choice) Leave() { c.s.prec.pending = PendingPrec{} }

// PendingFor returns the pending precedence for parsing the fields of the
// operator alternative alt after its left operand.
func (c *Choice) PendingFor(alt Alternative) PendingPrec {
	return PendingPrec{level: alt.prec, family: c.t, outer: c.bound}
}

// A Dispatch says, for a token, which direct left-recursive alternatives of
// a one-of rule may continue the value parsed so far with it, as a bit set
// by alternative index.
type Dispatch struct {
	byType  map[string]*dispatchEntry
	byValue map[string]uint64 // options with a value and no type
	always  uint64            // alternatives whose continuation may start with anything
	alts    []dispatchAlt
}

type dispatchEntry struct {
	anyValue uint64
	byValue  map[string]uint64
}

type dispatchAlt struct {
	prec int
	opts []TokenParseOptions // of the continuation, nil if not direct
}

// DispatchOf builds the Dispatch of the OneOf rule type T, for generated
// code.
func DispatchOf[T any]() *Dispatch {
	return newDispatch(AlternativesOf[T]())
}

func newDispatch(alts []Alternative) *Dispatch {
	if len(alts) > 64 {
		panic("too many alternatives for a Dispatch")
	}
	d := &Dispatch{byType: map[string]*dispatchEntry{}, byValue: map[string]uint64{}}
	for i, a := range alts {
		if a.leftRec != directLeftRec {
			d.alts = append(d.alts, dispatchAlt{})
			continue
		}
		bit := uint64(1) << i
		d.alts = append(d.alts, dispatchAlt{prec: a.prec, opts: a.cont.opts})
		if a.cont.any {
			d.always |= bit
			continue
		}
		for _, o := range append(append([]TokenParseOptions{}, a.cont.opts...), a.cont.drop...) {
			switch {
			case o.TokenType == "" && o.TokenValue == "":
				d.always |= bit
			case o.TokenType == "":
				d.byValue[o.TokenValue] |= bit
			default:
				e := d.byType[o.TokenType]
				if e == nil {
					e = &dispatchEntry{byValue: map[string]uint64{}}
					d.byType[o.TokenType] = e
				}
				if o.TokenValue == "" {
					e.anyValue |= bit
				} else {
					e.byValue[o.TokenValue] |= bit
				}
			}
		}
	}
	return d
}

// Lookup returns the alternatives that may continue with tok, those whose
// precedence is above level.
func (d *Dispatch) Lookup(tok Token, level int) uint64 {
	mask := d.always | d.byValue[tok.Value()]
	if e := d.byType[tok.Type()]; e != nil {
		mask |= e.anyValue | e.byValue[tok.Value()]
	}
	for i, a := range d.alts {
		if a.prec != 0 && level >= a.prec {
			mask &^= uint64(1) << i
		}
	}
	return mask
}

// Expect records the continuations of the alternatives above level as
// expected at the current position, when none applied.
func (d *Dispatch) Expect(s *ParserState, level int) *ParseError {
	pos := s.Save()
	if s.beyondRecord(pos) {
		return &s.fail
	}
	tok := s.Next()
	s.Restore(pos)
	var err *ParseError
	for _, a := range d.alts {
		if a.opts == nil || a.prec != 0 && level >= a.prec {
			continue
		}
		err = s.recordFailure(pos, tok, a.opts, nil)
	}
	return err
}

// RestParser is implemented by generated Seq rules: ParseRest parses the
// fields after the first, which is already set (the left operand), under
// the pending precedence.
type RestParser interface {
	ParseRest(s *ParserState, pending PendingPrec) *ParseError
}

// parseRest parses the fields of the Seq rule item after the first, which is
// set, under the pending precedence: with the generated method if there is
// one, else by reflection.
func parseRest(item any, s *ParserState, pending PendingPrec) *ParseError {
	if rp, ok := item.(RestParser); ok {
		return s.fold(rp.ParseRest(s, pending))
	}
	ruleDef, elem := getRuleDefAndValue(item)
	return s.fold(seqParse(ruleDef, elem, s, 1, pending))
}

// oneOfInfo is what is known before parsing about the alternatives of a
// one-of rule type.
type oneOfInfo struct {
	alts     []oneOfAlt
	climb    bool      // every left-recursive alternative is direct: parse by climbing
	dispatch *Dispatch // the direct alternatives by the token that continues the value
	drop     TokenOptions
}

// oneOfAlt is an alternative of a one-of rule and where its value goes.
type oneOfAlt struct {
	Alternative
	typ       reflect.Type
	opts      TokenOptions   // its tok tag
	field     *RuleField     // of a struct OneOf: the field holding it
	path      []reflect.Type // of a sum: the nested sum types between the rule and typ
	leftField int            // of a direct one: the struct field index of its left operand
}

// store sets item (a pointer to typ, or a slice of it for a repeated field)
// as the value of the one-of rule dst: in the field holding it for a struct
// OneOf, as the Value of a sum, wrapped in the nested sums of its path.
func (a *oneOfAlt) store(dst reflect.Value, item reflect.Value) {
	if a.field != nil {
		dst.SetZero()
		dst.Field(a.field.Index).Set(item)
		return
	}
	v := item.Interface()
	for j := len(a.path) - 1; j >= 0; j-- {
		w := reflect.New(a.path[j]).Interface()
		w.(oneOfValuer).oneOfValue().Value = v
		v = w
	}
	dst.Addr().Interface().(oneOfValuer).oneOfValue().Value = v
}

var oneOfInfos atomicMap[reflect.Type, *oneOfInfo]

// oneOfInfoOf returns the (cached) information on the one-of rule type t.
func oneOfInfoOf(t reflect.Type) *oneOfInfo {
	if info, ok := oneOfInfos.load(t); ok {
		return info
	}
	firstOfType(t) // builds the definitions reachable from t
	b := newRuleBuilder()
	info := &oneOfInfo{climb: true}
	var groups map[reflect.Type]bool
	if def, err := b.def(t); err == nil && def.OneOf {
		// A struct OneOf: an alternative per field.
		info.drop = def.DropOptions
		for i := range def.Fields {
			f := &def.Fields[i]
			info.alts = append(info.alts, oneOfAlt{Alternative: def.alts[i], typ: f.BaseType, opts: f.TokenOptions, field: f})
		}
	} else {
		info.drop = sumDrop(t)
		info.alts, groups = b.flattenSum(t)
	}
	if len(info.alts) > 64 {
		panic(fmt.Sprintf("rule %s has more than 64 alternatives", t))
	}
	for i := range info.alts {
		a := &info.alts[i]
		switch a.leftRec {
		case directLeftRec:
			def, _ := b.def(a.typ)
			a.leftField = def.Fields[0].Index
		case indirectLeftRec:
			info.climb = false
		}
		// An operator whose left operand is a group of the rule rather than
		// the rule itself would make the group a sum of its own.
		if def, err := b.def(a.typ); err == nil && !def.OneOf && len(def.Fields) > 0 && groups[def.Fields[0].BaseType] {
			panic(fmt.Sprintf("rule %s: the left operand of %s is %s, a group of alternatives of %s; it should be %s", t, a.typ, def.Fields[0].BaseType, t, t))
		}
		// A group of left-recursive alternatives that is not flattened into
		// the rule (a struct OneOf, or any group in a struct OneOf) would be
		// parsed without the bound: the levels would not apply in it.
		if a.leftRec == indirectLeftRec && isOneOfType(b, a.typ) && b.hasOperators(t, map[reflect.Type]bool{}) {
			panic(fmt.Sprintf("rule %s: %s is a group of alternatives left recursive through %s; with precedence levels, declare %s with grammar.Sum so that the group is flattened into it", t, a.typ, t, t))
		}
	}
	alts := make([]Alternative, len(info.alts))
	for i := range info.alts {
		alts[i] = info.alts[i].Alternative
	}
	info.dispatch = newDispatch(alts)
	oneOfInfos.store(t, info)
	return info
}

// isOneOfType reports whether u is a one-of rule type: a struct OneOf, a
// OneOfN or a Sum.
func isOneOfType(b *ruleBuilder, u reflect.Type) bool {
	if _, _, ok := sumAlternatives(u); ok {
		return true
	}
	def, err := b.def(u)
	return err == nil && def.OneOf
}

// hasOperators reports whether the one-of rule type t has a rule with a
// level among its alternatives, at any depth.
func (b *ruleBuilder) hasOperators(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	if groupPrec(t) != 0 {
		return true
	}
	if types, _, ok := sumAlternatives(t); ok {
		for _, u := range types {
			if b.hasOperators(u, seen) {
				return true
			}
		}
		return false
	}
	def, err := b.def(t)
	if err != nil {
		return false
	}
	if def.Prec != 0 {
		return true
	}
	if def.OneOf {
		for _, f := range def.Fields {
			if b.hasOperators(f.BaseType, seen) {
				return true
			}
		}
	}
	return false
}

// parseOneOf parses the one-of rule that self points to, by reflection.
func parseOneOf(self any, s *ParserState) *ParseError {
	t := reflect.TypeOf(self).Elem()
	info := oneOfInfoOf(t)
	dst := reflect.ValueOf(self).Elem()
	c := s.Choice(t)
	info.drop.DropMatchingNextTokens(s)
	var err *ParseError
	// try parses alt at the current position into dst and reports whether
	// it matched; if not the stream is back where it was.
	try := func(alt *oneOfAlt) bool {
		ok, tryErr := c.Try(alt.Alternative)
		if !ok {
			err = err.Merge(tryErr)
			return false
		}
		if alt.field != nil && alt.field.Array {
			itemsV, n, arrErr, ok := parseRepeated(*alt.field, info.drop, alt.first, Bound{}, s)
			err = err.Merge(arrErr)
			if !ok || n == 0 {
				return false
			}
			alt.store(dst, itemsV)
			return true
		}
		start := s.Save()
		item := reflect.New(alt.typ)
		c.Enter(alt.Alternative)
		altErr := ParseWithOptions(item.Interface(), s, alt.opts)
		c.Leave()
		if altErr != nil {
			s.Restore(start)
			err = err.Merge(altErr)
			return false
		}
		alt.store(dst, item)
		return true
	}
	if !info.climb {
		// Left recursive through other rules: the general mechanism.
		for i := range info.alts {
			if try(&info.alts[i]) {
				return nil
			}
		}
		return err
	}
	// Seed with the alternatives that do not start with the rule itself.
	seeded := false
	for i := range info.alts {
		if info.alts[i].leftRec != directLeftRec && try(&info.alts[i]) {
			seeded = true
			break
		}
	}
	if !seeded {
		return err
	}
	// Climb: extend it with the ones that do, as long as one applies.
	for {
		mask := info.dispatch.Lookup(s.Peek(), c.Level())
		if mask == 0 {
			break
		}
		extended := false
		for i := range info.alts {
			if mask&(1<<i) == 0 {
				continue
			}
			alt := &info.alts[i]
			start := s.Save()
			item := reflect.New(alt.typ)
			item.Elem().Field(alt.leftField).Set(dst)
			if restErr := parseRest(item.Interface(), s, c.PendingFor(alt.Alternative)); restErr == nil && s.Save() > start {
				alt.store(dst, item)
				extended = true
				break
			} else {
				err = err.Merge(restErr)
				s.Restore(start)
			}
		}
		if !extended {
			break
		}
	}
	info.dispatch.Expect(s, c.Level())
	return nil
}
