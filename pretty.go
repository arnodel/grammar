package grammar

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

func PrettyWrite(out io.Writer, r interface{}) error {
	return prettyWrite(out, r, "", 0)
}

func prettyWrite(out io.Writer, r interface{}, pfx string, indent int) (err error) {
	ruleDef, err := getRuleDef(reflect.TypeOf(r))
	rV := reflect.ValueOf(r)
	if err != nil {
		return writeLine(out, pfx, indent, r)
	}
	err = writeLine(out, pfx, indent, ruleDef.Name+" {")
	if err != nil {
		return
	}
	for _, ruleField := range ruleDef.Fields {
		fieldV := rV.Field(ruleField.Index)
		switch {
		case ruleField.Pointer:
			if !fieldV.IsNil() {
				prettyWrite(out, fieldV.Elem().Interface(), ruleField.Name, indent+2)
			}
		case ruleField.Array:
			for i := 0; i < fieldV.Len(); i++ {
				prettyWrite(out, fieldV.Index(i).Interface(), ruleField.Name, indent+2)
			}
		default:
			prettyWrite(out, fieldV.Interface(), ruleField.Name, indent+2)
		}
	}
	writeLine(out, "", indent, "}")
	return nil
}

func writeLine(out io.Writer, pfx string, indent int, val interface{}) (err error) {
	if pfx != "" {
		_, err = fmt.Fprintf(out, "%s%s: %v\n", strings.Repeat(" ", indent), pfx, val)
	} else {
		_, err = fmt.Fprintf(out, "%s%v\n", strings.Repeat(" ", indent), val)
	}
	return
}
