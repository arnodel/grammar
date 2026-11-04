//go:build !nocompiledgrammar

package json

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Check that the generated Parse methods shadow the reflection ones.
func TestGeneratedParseInUse(t *testing.T) {
	for _, rule := range []any{(*Json).Parse, (*Array).Parse, (*Number).Parse, (*DictItem).Parse} {
		fn := runtime.FuncForPC(reflect.ValueOf(rule).Pointer())
		file, _ := fn.FileLine(fn.Entry())
		if !strings.HasSuffix(file, "grammar.compiled.go") {
			t.Errorf("%s is defined in %s, want the generated file", fn.Name(), file)
		}
	}
}
