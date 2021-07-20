package json

import "strconv"

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

func (n Number) Compile() float64 {
	x, err := strconv.ParseFloat(n.Value.Value(), 64)
	if err != nil {
		panic(err)
	}
	return x
}

func (s String) Compile() string {
	cs, err := strconv.Unquote(s.Value.Value())
	if err != nil {
		panic(err)
	}
	return cs
}

func (n Null) Compile() interface{} {
	return nil
}

func (b Bool) Compile() bool {
	switch b.Value.Value() {
	case "true":
		return true
	case "false":
		return false
	default:
		panic("invalid boolean literal")
	}
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
