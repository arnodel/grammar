package grammar

import (
	"fmt"
	"reflect"
	"strconv"
)

// Operator precedence.
//
// An operator rule is a Seq rule with a prec tag, its level: a positive
// integer, higher binding tighter (0 means no level).  Among the
// alternatives of a one-of rule, an operator applies only when the level in
// effect is lower than its own, and its operands after the first are parsed
// under its own level.  So in `1 + 2 * 3`, the right operand of + (level 1)
// is parsed under level 1, where * (level 2) applies but another + does not.
//
// Levels travel between rules through the parser state (precState), since
// the Parser interface has no room for them:
//
//   - A rule sets the bound for the next rule it parses (SetPrec) and
//     ParseWithOptions takes it, so that a rule which sets none passes none
//     on: a level is consumed by the first rule entered, which is why the
//     expression inside brackets starts afresh.  The bound is part of the
//     memo key, as a rule parsed under two bounds may give two results.
//   - A one-of rule entering an operator alternative leaves it the pending
//     precedence (Choice.Enter): the bound its left operand is parsed under
//     and the level for its other operands.  The Seq rule takes it
//     (TakePending) and sets the bound of each field from it.

// A Bound is the precedence in effect: the level of the operator whose
// operand is being parsed and the family of that operator, the one-of rule
// type it is an alternative of.  Only that one-of rule is constrained by the
// bound, so that the levels of one expression grammar mean nothing to
// another (a language with value and type expressions can give each its own
// table).  A bound with no family constrains any one-of rule; the zero Bound
// constrains nothing.
type Bound struct {
	Level  int
	Family reflect.Type
}

// constrains returns the level the bound imposes on the one-of rule type t,
// 0 if none.
func (b Bound) constrains(t reflect.Type) int {
	if b.Family == nil || b.Family == t {
		return b.Level
	}
	return 0
}

// PendingPrec is what a one-of rule leaves for the operator alternative it
// is about to parse: the bound to parse its left operand under (the one the
// one-of rule itself is parsed under) and the level for its other fields.
// The zero value is nothing pending.
type PendingPrec struct {
	level  int          // of the operator, 0 if nothing is pending
	family reflect.Type // the one-of rule type
	outer  Bound        // the one-of rule is parsed under
}

// Prec returns the bound to parse field i of the operator rule under.
func (p PendingPrec) Prec(i int) Bound {
	switch {
	case p.level == 0:
		return Bound{}
	case i == 0:
		return p.outer
	default:
		return Bound{Level: p.level, Family: p.family}
	}
}

// precState is the part of the parser state that passes precedences between
// rules.
type precState struct {
	next    Bound       // set by a rule for the next rule it parses
	bound   Bound       // the rule being parsed is under; valid when its Parse method starts
	pending PendingPrec // for the operator rule about to be parsed
}

// take returns the bound set for the next rule and clears it.
func (p *precState) take() Bound {
	b := p.next
	p.next = Bound{}
	return b
}

// SetPrec sets the precedence bound for the next rule parsed.
func (s *ParserState) SetPrec(b Bound) { s.prec.next = b }

// TakePending returns the pending precedence for the operator rule being
// parsed and clears it.
func (s *ParserState) TakePending() PendingPrec {
	p := s.prec.pending
	s.prec.pending = PendingPrec{}
	return p
}

// TakePendingOr is TakePending for a Seq rule with its own level: entered
// otherwise than as an operator alternative (as the root rule, say) its
// fields after the first are bound to level, in the family of the bound it
// is parsed under.
func (s *ParserState) TakePendingOr(level int) PendingPrec {
	p := s.TakePending()
	if p.level == 0 && level != 0 {
		p = PendingPrec{level: level, family: s.prec.bound.Family, outer: s.prec.bound}
	}
	return p
}

// precFromTagValue parses the value of a prec tag: a level, which is a
// positive integer; "" is no level.
func precFromTagValue(v string) (int, error) {
	if v == "" {
		return 0, nil
	}
	p, err := strconv.Atoi(v)
	if err != nil || p <= 0 {
		return 0, fmt.Errorf("invalid prec tag %q (want a positive integer)", v)
	}
	return p, nil
}
