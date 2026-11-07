package grammar

import "reflect"

// An Alternative is what is known before parsing about an alternative of a
// one-of rule, from the rule definitions: the tokens it can start with,
// whether it is left recursive and its precedence.  A Choice uses it to
// decide whether to try the alternative (see oneof.go).
type Alternative struct {
	first   FirstSet
	leftRec leftRecKind
	cont    FirstSet // of a direct one: what may follow its left operand
	prec    int      // its level, 0 if it has none (see prec.go)
}

// leftRecKind says how an alternative of a one-of rule may parse the rule
// itself first, at the same position.
type leftRecKind uint8

const (
	notLeftRec      leftRecKind = iota
	directLeftRec               // a Seq rule whose first field is the one-of rule: an infix or postfix operator
	indirectLeftRec             // reaches the one-of rule first through other rules
)

// alternatives fills in the alternatives of the OneOf rule t (whose field
// first sets are known) in its definition.
func (b *ruleBuilder) alternatives(t reflect.Type, def *RuleDef, lookup func(reflect.Type) FirstSet) {
	def.alts = make([]Alternative, len(def.Fields))
	for i, f := range def.Fields {
		def.alts[i] = b.classify(t, f.BaseType, f.Pointer, lookup)
		def.alts[i].first = def.firsts[i]
		if def.alts[i].prec == 0 {
			def.alts[i].prec = def.Prec // the group's level is the default
		}
	}
}

// classify returns alt as an alternative of the one-of rule t, but for its
// first set: its left recursion and the level of its own rule.
func (b *ruleBuilder) classify(t, alt reflect.Type, pointer bool, lookup func(reflect.Type) FirstSet) Alternative {
	var a Alternative
	altDef, err := b.def(alt)
	if err == nil {
		a.prec = altDef.Prec
	}
	if !b.reachesFirst(alt, t, lookup, map[reflect.Type]bool{}) {
		return a
	}
	a.leftRec = indirectLeftRec
	if err == nil && pointer && !altDef.OneOf && len(altDef.Fields) > 0 {
		if first := altDef.Fields[0]; first.BaseType == t && !first.Pointer && !first.Array {
			a.leftRec, a.cont = directLeftRec, seqFirst(altDef, 1, lookup)
		}
	}
	return a
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
