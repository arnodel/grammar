package grammar

import (
	"fmt"
	"reflect"
)

type Empty struct{}

func (Empty) Parse(r interface{}, s *ParserState, opts TokenOptions) *ParseError {
	return nil
}

// OneOf should be used as the first field of a Rule struct to signify that it
// should match exactly one of the fields
type OneOf struct{}

var _ Parser = OneOf{}

func (OneOf) Parse(r interface{}, s *ParserState, opts TokenOptions) *ParseError {
	ruleDef, elem := getRuleDefAndValue(r)
	var err, fieldErr *ParseError
	for _, ruleField := range ruleDef.Fields {
		switch {
		case ruleField.Pointer:
			{
				start := s.Save()
				fieldPtrV := reflect.New(ruleField.BaseType)
				fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.TokenOptions)
				if fieldErr == nil {
					elem.Field(ruleField.Index).Set(fieldPtrV)
					return nil
				}
				s.Restore(start)
				err = err.Merge(fieldErr)
			}
		case ruleField.Array:
			{
				itemsV := reflect.Zero(reflect.SliceOf(ruleField.BaseType))
				var sz int
				arrStart := s.Save()
				for sz = 0; ruleField.Max == 0 || sz < ruleField.Max; sz++ {
					start := s.Save()
					itemPtrV := reflect.New(ruleField.BaseType)
					fieldErr = ParseWithOptions(itemPtrV.Interface(), s, ruleField.TokenOptions)
					if fieldErr != nil {
						if sz < ruleField.Min {
							s.Restore(arrStart)
							break
						}
						s.Restore(start)
						if sz > 0 {
							elem.Field(ruleField.Index).Set(itemsV)
							return nil
						}
						err = err.Merge(fieldErr)
						break
					}
					itemsV = reflect.Append(itemsV, itemPtrV.Elem())
				}
			}
		default:
			panic("should not get here")
		}
	}
	return err
}

// Seq should be used as the first field of a Rule struct to signify that it
// should match all the fields in sequence.
type Seq struct{}

var _ Parser = Seq{}

func (Seq) Parse(r interface{}, s *ParserState, opts TokenOptions) *ParseError {
	ruleDef, elem := getRuleDefAndValue(r)
	var err, fieldErr *ParseError
	itemCount := 0
	needsSeparator := false
	var sep interface{}
	var sepOptional bool
	if ruleDef.Separator != nil {
		sep = reflect.New(ruleDef.Separator.BaseType).Interface()
		sepOptional = ruleDef.Separator.Pointer
	}
	parseSep := func() *ParseError {
		if sep == nil || !needsSeparator {
			return nil
		}
		start := s.Save()
		sepErr := ParseWithOptions(sep, s, ruleDef.Separator.TokenOptions)
		if sepErr != nil {
			if !sepOptional {
				return sepErr
			}
			s.Restore(start)
		}
		needsSeparator = false
		return nil
	}
	var fieldPtrV reflect.Value
	for _, ruleField := range ruleDef.Fields {
		if s.Debug() {
			s.Logf("  .%s tok #%d", ruleField.Name, s.Save())
		}
		switch {
		case ruleField.Pointer:
			{
				start := s.Save()
				needsSeparatorAtStart := needsSeparator
				fieldErr = parseSep()
				if fieldErr == nil {
					fieldPtrV = reflect.New(ruleField.BaseType)
					fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.TokenOptions)
				}
				if fieldErr != nil {
					err = err.Merge(fieldErr)
					needsSeparator = needsSeparatorAtStart
					s.Restore(start)
				} else {
					elem.Field(ruleField.Index).Set(fieldPtrV)
					itemCount++
					needsSeparator = true
				}
			}
		case ruleField.Array:
			{
				itemsV := reflect.Zero(reflect.SliceOf(ruleField.BaseType))
				var sz int
				for sz = 0; ruleField.Max == 0 || sz < ruleField.Max; sz++ {
					start := s.Save()
					needsSeparatorAtStart := needsSeparator
					fieldErr = parseSep()
					if fieldErr == nil {
						fieldPtrV = reflect.New(ruleField.BaseType)
						fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.TokenOptions)
					}
					if fieldErr != nil {
						err = err.Merge(fieldErr)
						if sz < ruleField.Min {
							return err
						}
						needsSeparator = needsSeparatorAtStart
						s.Restore(start)
						break
					}
					itemsV = reflect.Append(itemsV, fieldPtrV.Elem())
					itemCount++
					needsSeparator = true
				}
				elem.Field(ruleField.Index).Set(itemsV)
			}
		default:
			fieldErr = parseSep()
			if fieldErr == nil {
				fieldPtrV = reflect.New(ruleField.BaseType)
				fieldErr = ParseWithOptions(fieldPtrV.Interface(), s, ruleField.TokenOptions)
			}
			if fieldErr != nil {
				return err.Merge(fieldErr)
			}
			elem.Field(ruleField.Index).Set(fieldPtrV.Elem())
			itemCount++
			needsSeparator = true
		}
	}
	if itemCount == 0 {
		pos := s.Save()
		tok := s.Next()
		return &ParseError{
			Token: tok,
			Err:   fmt.Errorf("empty match for rule %s", ruleDef.Name),
			Pos:   pos,
		}
	}
	return nil
}
