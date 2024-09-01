package grammar

import (
	"fmt"
	"io"
	"strings"
)

// PrettyWrite outputs a pretty representation of the rule r on the writer. For
// a struct like this:
//
//	type RuleType struct {
//	    Field1 Rule1
//	    Field2 []Rule2
//	}
//
// It looks like this:
//
//	RuleType {
//	  Field1: <pretty representation of the value of Field1>
//	  Field2: [
//	     <pretty representation of the first item in Field2>
//	     <pretty representation of the second item>
//	     <...>
//	  ]
//	}
//
// Empty optional fields and empty repeated fields are omitted altogether.

type PrettyWriter interface {
	PrettyWrite(out io.Writer, r any, pfx string, indent int) (err error)
}

func PrettyWrite(out io.Writer, r any) error {
	return prettyWrite(out, r, "", 0)
}

func prettyWrite(out io.Writer, r any, pfx string, indent int) (err error) {
	if pw, ok := r.(PrettyWriter); ok {
		return pw.PrettyWrite(out, r, pfx, indent)
	}
	return prettyWriteStruct(out, r, pfx, indent)
}

func prettyWriteStruct(out io.Writer, r any, pfx string, indent int) (err error) {
	ruleDef, rV := getRuleDefAndValue(r)
	err = writeLine(out, pfx, indent, ruleDef.Name+" {")
	if err != nil {
		return
	}
	for _, ruleField := range ruleDef.Fields {
		fieldV := rV.Field(ruleField.Index)
		switch {
		case ruleField.Pointer:
			if !fieldV.IsNil() {
				prettyWrite(out, fieldV.Interface(), ruleField.Name, indent+2)
			}
		case ruleField.Array:
			if fieldV.Len() > 0 {
				writeLine(out, ruleField.Name, indent+2, "[")
				for i := 0; i < fieldV.Len(); i++ {
					prettyWrite(out, fieldV.Index(i).Addr().Interface(), "", indent+4)
				}
				writeLine(out, "", indent+2, "]")
			}
		default:
			prettyWrite(out, fieldV.Addr().Interface(), ruleField.Name, indent+2)
		}
	}
	writeLine(out, "", indent, "}")
	return nil
}

func writeLine(out io.Writer, pfx string, indent int, val interface{}) (err error) {
	if pfx != "" {
		_, err = fmt.Fprintf(out, "%s%s: %s\n", strings.Repeat(" ", indent), pfx, val)
	} else {
		_, err = fmt.Fprintf(out, "%s%s\n", strings.Repeat(" ", indent), val)
	}
	return
}
