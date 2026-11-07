package grammar

import (
	"fmt"
	"reflect"
	"slices"
)

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
	return parseOneOf(r, s)
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

// alternatives is implemented by the OneOfN and Sum rules (and promoted to
// the structs embedding them).
type alternatives interface {
	alternativeTypes() []reflect.Type
}

var alternativesType = reflect.TypeFor[alternatives]()

// sumDeclarer is implemented by Sum: the definition of the declaration type,
// for its drop tag, default precedence and the tok tags of its alternatives.
type sumDeclarer interface{ sumDecl() *RuleDef }

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

// sumDrop returns the drop options of the sum type u: those of a Sum's
// declaration.
func sumDrop(u reflect.Type) TokenOptions {
	if d, ok := reflect.New(u).Interface().(sumDeclarer); ok {
		return d.sumDecl().DropOptions
	}
	return TokenOptions{}
}

// groupPrec returns the level a group (a OneOfN or Sum type, or a type
// embedding one) gives its alternatives by default, 0 if none: the prec tag
// of the embedded field, else that of a Sum's declaration.
func groupPrec(u reflect.Type) int {
	if u.Kind() == reflect.Struct && u.NumField() > 0 {
		p, err := precFromTagValue(u.Field(0).Tag.Get("prec"))
		if err != nil {
			panic(fmt.Sprintf("rule %s: %s", u, err))
		}
		if p != 0 {
			return p
		}
	}
	if d, ok := reflect.New(u).Interface().(sumDeclarer); ok {
		return d.sumDecl().Prec
	}
	return 0
}

// flattenSum returns the alternatives of the sum rule type t: an alternative
// that is itself a sum (a group) contributes its own alternatives, with the
// path of groups to wrap their values back in, and gives them its level as a
// default.  groups is the set of nested sum types flattened.
func (b *ruleBuilder) flattenSum(t reflect.Type) (alts []oneOfAlt, groups map[reflect.Type]bool) {
	groups = map[reflect.Type]bool{}
	var walk func(u reflect.Type, opts TokenOptions, path []reflect.Type, prec int)
	walk = func(u reflect.Type, opts TokenOptions, path []reflect.Type, prec int) {
		if types, altOpts, ok := sumAlternatives(u); ok && u != t && !slices.Contains(path, u) {
			groups[u] = true
			if p := groupPrec(u); p != 0 {
				prec = p
			}
			for i, alt := range types {
				walk(alt, altOpts[i], append(path[:len(path):len(path)], u), prec)
			}
			return
		}
		a := oneOfAlt{Alternative: b.classify(t, u, true, firstOfType), typ: u, opts: opts, path: path}
		a.first = firstOfField(u, opts, firstOfType)
		if a.prec == 0 {
			a.prec = prec
		}
		alts = append(alts, a)
	}
	types, altOpts, _ := sumAlternatives(t)
	for i, alt := range types {
		walk(alt, altOpts[i], nil, groupPrec(t))
	}
	return alts, groups
}
