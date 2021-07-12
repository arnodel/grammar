package grammar

import (
	"fmt"
	"regexp"
	"strconv"
)

type Json struct {
	OneOf
	*Number
	*String
	*Null
	*Bool
	*Array
	*Dict
}

func (j Json) Compile() interface{} {
	switch {
	case j.Number != nil:
		return j.Number.Compile()
	case j.String != nil:
		return j.String.Compile()
	case j.Null != nil:
		return j.Null.Compile()
	case j.Bool != nil:
		return j.Bool.Compile()
	case j.Array != nil:
		return j.Array.Compile()
	case j.Dict != nil:
		return j.Dict.Compile()
	default:
		panic("invalid json")
	}
}

type Number struct {
	Token SimpleToken `tok:"number"`
}

func (n Number) Compile() float64 {
	x, err := strconv.ParseFloat(n.Token.tokValue, 64)
	if err != nil {
		panic(err)
	}
	return x
}

type String struct {
	Token SimpleToken `tok:"string"`
}

func (s String) Compile() string {
	cs, err := strconv.Unquote(s.Token.tokValue)
	if err != nil {
		panic(err)
	}
	return cs
}

type Null struct {
	Token SimpleToken `tok:",null"`
}

func (n Null) Compile() interface{} {
	return nil
}

type Bool struct {
	Token SimpleToken `tok:"bool"`
}

func (b Bool) Compile() bool {
	switch b.Token.tokValue {
	case "true":
		return true
	case "false":
		return false
	default:
		panic("invalid boolean literal")
	}
}

type Array struct {
	Open SimpleToken `tok:",["`
	*ArrayBody
	Close SimpleToken `tok:",]"`
}

func (a Array) Compile() []interface{} {
	if a.ArrayBody == nil {
		return nil
	}
	ca := []interface{}{a.ArrayBody.First.Compile()}
	for _, item := range a.ArrayBody.Items {
		ca = append(ca, item.Value.Compile())
	}
	return ca
}

type ArrayBody struct {
	First Json
	Items []ArrayItem
}

type ArrayItem struct {
	Comma SimpleToken `tok:",,"`
	Value Json
}

type Dict struct {
	Open SimpleToken `tok:",{"`
	*DictBody
	Close SimpleToken `tok:",}"`
}

func (d Dict) Compile() map[string]interface{} {
	if d.DictBody == nil {
		return nil
	}
	cd := map[string]interface{}{}
	cd[d.DictBody.First.Key.Compile()] = d.DictBody.First.Value.Compile()
	for _, item := range d.DictBody.Items {
		cd[item.KeyValue.Key.Compile()] = item.KeyValue.Value.Compile()
	}
	return cd
}

type DictBody struct {
	First KeyValue
	Items []DictItem
}

type KeyValue struct {
	Key   String
	Colon SimpleToken `tok:",:"`
	Value Json
}

type DictItem struct {
	Comma SimpleToken `tok:",,"`
	KeyValue
}

var jsonTokenPtn = regexp.MustCompile(`^(?:(\s+)|(null)|(true|false)|([{},:[\]])|("[^"]*")|(-?[0-9]+(?:\.[0-9]+)?))`)
var tokTypes = []string{"space", "null", "bool", "op", "string", "number"}

func TokeniseJsonString(s string) (*SliceTokenStream, error) {
	var toks []Token
	for len(s) > 0 {
		var tokType string
		matches := jsonTokenPtn.FindStringSubmatch(s)
		if matches == nil {
			return nil, fmt.Errorf("invalid json")
		}
		tokValue := matches[0]
		for i, match := range matches[1:] {
			if match != "" {
				tokType = tokTypes[i]
				break
			}
		}
		if tokType != "space" {
			toks = append(toks, SimpleToken{tokType: tokType, tokValue: tokValue})
		}
		s = s[len(tokValue):]
	}
	return &SliceTokenStream{tokens: toks}, nil
}
