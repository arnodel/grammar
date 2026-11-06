package grammar

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
)

// OneOfValue holds the result of parsing one of the generic OneOfN rules.
// Value is a pointer to the alternative that matched (e.g. *T2 for the second
// type parameter of OneOf3[T1, T2, T3]), so it is usually consumed with a type
// switch.
type OneOfValue struct {
	Value any
}

var _ Memoisable = OneOfValue{}

// MemoKey implements Memoisable: the OneOfN rules ignore the token options.
func (OneOfValue) MemoKey(opts TokenOptions) any {
	return nil
}

func (o *OneOfValue) PrettyWrite(out io.Writer, r any, pfx string, indent int) (err error) {
	return prettyWrite(out, o.Value, pfx, indent)
}

func (o *OneOfValue) oneOfValue() *OneOfValue { return o }

type oneOfValuer interface{ oneOfValue() *OneOfValue }

// Sum is a memory-efficient form of a OneOf rule: T is a rule type whose
// first field is OneOf, and whose other fields, which must be pointers, list
// the alternatives.  Where a value of T has a field per alternative, a Sum
// value is just Value, a pointer to the alternative that matched, like the
// OneOfN rules.  T is only a declaration: it is not parsed itself unless
// used as a rule in its own right.
//
//	type Expr struct {
//	    grammar.Sum[ExprAlts]
//	}
//
//	type ExprAlts struct {
//	    grammar.OneOf
//	    Or   *Or
//	    And  *And
//	    Atom *Atom
//	}
type Sum[T any] struct {
	OneOfValue
}

var _ Parser = &Sum[any]{}

func (o *Sum[T]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	return parseSum(&o.OneOfValue, r, s)
}

func (*Sum[T]) alternativeTypes() []reflect.Type {
	def := sumDef[T]()
	var alts []reflect.Type
	for _, f := range def.Fields {
		if !f.Pointer {
			panic(fmt.Sprintf("Sum[%s]: alternative %s must be a pointer", def.Name, f.Name))
		}
		alts = append(alts, f.BaseType)
	}
	return alts
}

func (*Sum[T]) sumDecl() *RuleDef {
	return sumDef[T]()
}

// sumDef returns the definition of the declaration type T.  It is computed
// here rather than taken from the cache, as this may run while the rules
// reachable from T are being built, T among them.
func sumDef[T any]() *RuleDef {
	def, err := calcRuleDef(reflect.TypeFor[T]())
	if err != nil {
		panic(fmt.Sprintf("Sum[%s]: %s", reflect.TypeFor[T](), err))
	}
	if !def.OneOf {
		panic(fmt.Sprintf("Sum[%s]: not a OneOf rule type", def.Name))
	}
	return def
}

// sumDeclarer is implemented by Sum: the definition of the declaration type,
// for its drop tag and default precedence.
type sumDeclarer interface{ sumDecl() *RuleDef }

// groupPrec returns the precedence a group (a OneOfN or Sum type, or a type
// embedding one) gives its alternatives by default: the prec tag of the
// embedded field, else that of a Sum's declaration.
func groupPrec(u reflect.Type) (int, bool) {
	if u.Kind() == reflect.Struct && u.NumField() > 0 {
		if tag, ok := u.Field(0).Tag.Lookup("prec"); ok {
			p, err := strconv.Atoi(tag)
			if err != nil || p < 0 {
				panic(fmt.Sprintf("rule %s: invalid prec tag %q", u, tag))
			}
			return p, true
		}
	}
	if d, ok := reflect.New(u).Interface().(sumDeclarer); ok {
		if def := d.sumDecl(); def.HasPrec {
			return def.Prec, true
		}
	}
	return 0, false
}

// sumAlt is one alternative of a sum rule, after flattening nested sums.
type sumAlt struct {
	typ       reflect.Type
	opts      TokenOptions   // the tok tag of the alternative, in a Sum declaration
	path      []reflect.Type // the nested sum types between the rule and typ
	first     FirstSet
	lr        LeftRecInfo
	leftField int // struct field index of the left operand, if direct
}

// sumAlternatives returns the alternatives of the sum type u with their
// token options: the fields of a Sum declaration, else the types of a OneOfN.
func sumAlternatives(u reflect.Type) ([]reflect.Type, []TokenOptions, bool) {
	if d, ok := reflect.New(u).Interface().(sumDeclarer); ok {
		def := d.sumDecl()
		types := make([]reflect.Type, len(def.Fields))
		opts := make([]TokenOptions, len(def.Fields))
		for i, f := range def.Fields {
			types[i], opts[i] = f.BaseType, f.TokenOptions
		}
		return types, opts, true
	}
	types, ok := alternativeTypesOf(u)
	return types, make([]TokenOptions, len(types)), ok
}

// wrap returns item wrapped in the nested sum values of its path.
func (a *sumAlt) wrap(item any) any {
	for j := len(a.path) - 1; j >= 0; j-- {
		w := reflect.New(a.path[j]).Interface()
		w.(oneOfValuer).oneOfValue().Value = item
		item = w
	}
	return item
}

type sumInfo struct {
	alts     []sumAlt
	direct   bool // no indirect left recursion: the loop applies
	dispatch *Dispatch
	drop     TokenOptions
}

var sumInfos atomicMap[reflect.Type, *sumInfo]

// sumInfoOf returns the (cached) information on the sum rule type t.
func sumInfoOf(t reflect.Type) *sumInfo {
	if info, ok := sumInfos.load(t); ok {
		return info
	}
	firstOfType(t) // make sure the definitions reachable from t are built
	b := newRuleBuilder()
	info := &sumInfo{direct: true}
	if d, ok := reflect.New(t).Interface().(sumDeclarer); ok {
		info.drop = d.sumDecl().DropOptions
	}
	groups := map[reflect.Type]bool{}
	var flatten func(u reflect.Type, opts TokenOptions, path []reflect.Type, prec int, hasPrec bool)
	flatten = func(u reflect.Type, opts TokenOptions, path []reflect.Type, prec int, hasPrec bool) {
		if alts, altOpts, ok := sumAlternatives(u); ok && len(path) < 8 {
			groups[u] = true
			if p, ok := groupPrec(u); ok {
				prec, hasPrec = p, true // the group's level, for its members
			}
			for i, alt := range alts {
				flatten(alt, altOpts[i], append(path[:len(path):len(path)], u), prec, hasPrec)
			}
			return
		}
		a := sumAlt{typ: u, opts: opts, path: path, first: firstOfField(u, opts, firstOfType), lr: b.classify(t, u, true, firstOfType)}
		if !a.lr.hasPrec && hasPrec {
			a.lr.prec, a.lr.hasPrec = prec, true
		}
		if a.lr.kind == directLeftRec {
			def, _ := b.def(u)
			a.leftField = def.Fields[0].Index
		} else if a.lr.kind == indirectLeftRec {
			info.direct = false
		}
		info.alts = append(info.alts, a)
	}
	alts, altOpts, _ := sumAlternatives(t)
	prec, hasPrec := groupPrec(t)
	for i, alt := range alts {
		flatten(alt, altOpts[i], nil, prec, hasPrec)
	}
	if len(info.alts) > 64 {
		panic(fmt.Sprintf("rule %s has more than 64 alternatives", t))
	}
	for _, a := range info.alts {
		// An operator whose left operand is a group of the rule rather than
		// the rule itself would make the group a sum of its own.
		if def, err := b.def(a.typ); err == nil && !def.OneOf && len(def.Fields) > 0 && groups[def.Fields[0].BaseType] {
			panic(fmt.Sprintf("rule %s: the left operand of %s is %s, a group of alternatives of %s; it should be %s", t, a.typ, def.Fields[0].BaseType, t, t))
		}
	}
	lrs := make([]LeftRecInfo, len(info.alts))
	for i, a := range info.alts {
		lrs[i] = a.lr
	}
	info.dispatch = newDispatch(lrs)
	sumInfos.store(t, info)
	return info
}

// parseSum parses a sum rule (OneOfN or Sum) into o; self is the rule value
// being parsed, whose type lists the alternatives.
func parseSum(o *OneOfValue, self any, s *ParserState) *ParseError {
	t := reflect.TypeOf(self).Elem()
	info := sumInfoOf(t)
	a := s.Alternatives(t)
	info.drop.DropMatchingNextTokens(s)
	var err *ParseError
	tryAlt := func(alt *sumAlt) bool {
		ok, tryErr := a.Try(alt.lr, alt.first)
		if !ok {
			err = err.Merge(tryErr)
			return false
		}
		start := s.Save()
		item := reflect.New(alt.typ).Interface()
		a.Enter(alt.lr)
		altErr := ParseWithOptions(item, s, alt.opts)
		a.Leave()
		if altErr != nil {
			s.Restore(start)
			err = err.Merge(altErr)
			return false
		}
		o.Value = alt.wrap(item)
		return true
	}
	if !info.direct {
		// Left recursive through other rules: the general mechanism.
		defer a.Done()
		for i := range info.alts {
			if tryAlt(&info.alts[i]) {
				return nil
			}
		}
		return err
	}
	// Seed with the alternatives that do not start with the rule itself.
	seeded := false
	for i := range info.alts {
		if info.alts[i].lr.kind != directLeftRec && tryAlt(&info.alts[i]) {
			seeded = true
			break
		}
	}
	if !seeded {
		return err
	}
	// Extend it with the ones that do, as long as one applies.
	level := a.Level()
	selfV := reflect.ValueOf(self).Elem()
	for {
		mask := info.dispatch.Lookup(s.Peek(), level)
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
			itemV := reflect.New(alt.typ)
			itemV.Elem().Field(alt.leftField).Set(selfV)
			item := itemV.Interface()
			if restErr := parseRest(item, s, a.PendingFor(alt.lr)); restErr == nil && s.Save() > start {
				o.Value = alt.wrap(item)
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
	info.dispatch.Expect(s, level)
	return nil
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

// OneOf2 is a rule that matches the first of T1, T2 that parses, in that
// order.  Each type parameter must be a rule type, i.e. a pointer to it must
// implement Parser; this is checked when the rule is first parsed, not at
// compile time, because Go cannot express "pointer to T implements Parser"
// without doubling the type parameters.
//
// OneOf3 to OneOf6 are the same for more alternatives.
//
// The recommended way to use these types is to embed them in a named struct
// rather than to define a type alias, so that methods can be defined on the
// rule and so that it has a short name in errors and debug output:
//
//	type Value struct {
//	    grammar.OneOf3[Number, String, List]
//	}
//
//	func (v *Value) Compile() any {
//	    switch x := v.Value.(type) {
//	    case *Number: ...
//	    case *String: ...
//	    case *List: ...
//	    }
//	}
type OneOf2[T1, T2 any] struct {
	OneOfValue
}

var _ Parser = &OneOf2[any, any]{}
var _ PrettyWriter = &OneOf2[any, any]{}

func (o *OneOf2[T1, T2]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	return parseSum(&o.OneOfValue, r, s)
}

func (*OneOf2[T1, T2]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2]()}
}

type OneOf3[T1, T2, T3 any] struct {
	OneOfValue
}

var _ Parser = &OneOf3[any, any, any]{}
var _ PrettyWriter = &OneOf3[any, any, any]{}

func (o *OneOf3[T1, T2, T3]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	return parseSum(&o.OneOfValue, r, s)
}

func (*OneOf3[T1, T2, T3]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2](), reflect.TypeFor[T3]()}
}

type OneOf4[T1, T2, T3, T4 any] struct {
	OneOfValue
}

var _ Parser = &OneOf4[any, any, any, any]{}
var _ PrettyWriter = &OneOf4[any, any, any, any]{}

func (o *OneOf4[T1, T2, T3, T4]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	return parseSum(&o.OneOfValue, r, s)
}

func (*OneOf4[T1, T2, T3, T4]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2](), reflect.TypeFor[T3](), reflect.TypeFor[T4]()}
}

type OneOf5[T1, T2, T3, T4, T5 any] struct {
	OneOfValue
}

var _ Parser = &OneOf5[any, any, any, any, any]{}
var _ PrettyWriter = &OneOf5[any, any, any, any, any]{}

func (o *OneOf5[T1, T2, T3, T4, T5]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	return parseSum(&o.OneOfValue, r, s)
}

func (*OneOf5[T1, T2, T3, T4, T5]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2](), reflect.TypeFor[T3](), reflect.TypeFor[T4](), reflect.TypeFor[T5]()}
}

type OneOf6[T1, T2, T3, T4, T5, T6 any] struct {
	OneOfValue
}

var _ Parser = &OneOf6[any, any, any, any, any, any]{}
var _ PrettyWriter = &OneOf6[any, any, any, any, any, any]{}

func (o *OneOf6[T1, T2, T3, T4, T5, T6]) Parse(r any, s *ParserState, opts TokenOptions) *ParseError {
	return parseSum(&o.OneOfValue, r, s)
}

func (*OneOf6[T1, T2, T3, T4, T5, T6]) alternativeTypes() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[T1](), reflect.TypeFor[T2](), reflect.TypeFor[T3](), reflect.TypeFor[T4](), reflect.TypeFor[T5](), reflect.TypeFor[T6]()}
}
