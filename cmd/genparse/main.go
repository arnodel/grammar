// Command genparse generates reflection-free Parse methods for the grammar
// rules defined in a Go source file.
//
// Usage:
//
//	genparse [file.go]
//
// With no argument it reads the file named by the GOFILE environment variable,
// so it can be run from a go:generate directive in the file holding the rules:
//
//	//go:generate go run github.com/arnodel/grammar/cmd/genparse
//
// The generated file, <file>.compiled.go, has a Parse method for every struct
// type whose first field is grammar.Seq, grammar.OneOf or grammar.Tok, which
// shadows the one those types provide.  Each method does what the reflection
// based one does, using the helpers in the grammar package, so the two are
// interchangeable; building with -tags nocompiledgrammar excludes the
// generated file and reinstates reflection.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/arnodel/grammar"
)

const grammarImportPath = "github.com/arnodel/grammar"

func main() {
	log.SetFlags(0)
	flag.Parse()

	var srcFile string
	switch flag.NArg() {
	case 0:
		var ok bool
		srcFile, ok = os.LookupEnv("GOFILE")
		if !ok {
			log.Fatal("no source file given and GOFILE not defined")
		}
	case 1:
		srcFile = flag.Arg(0)
	default:
		log.Fatal("only one source file supported")
	}

	out, err := Generate(srcFile)
	if err != nil {
		log.Fatal(err)
	}
	outFile := OutputPath(srcFile)
	if err := os.WriteFile(outFile, out, 0644); err != nil {
		log.Fatalf("cannot write %s: %s", outFile, err)
	}
	log.Printf("genparse: wrote %s", outFile)
}

// OutputPath returns the path of the generated file for the given source file.
func OutputPath(srcFile string) string {
	ext := filepath.Ext(srcFile)
	return strings.TrimSuffix(srcFile, ext) + ".compiled" + ext
}

// Generate returns the generated source for the rules in srcFile.
func Generate(srcFile string) ([]byte, error) {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, srcFile, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	grammarPkg := grammarPackageName(astFile)
	if grammarPkg == "" {
		return nil, fmt.Errorf("%s does not import %s", srcFile, grammarImportPath)
	}
	rules, err := collectRules(astFile, grammarPkg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", srcFile, err)
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("%s: no grammar rules found", srcFile)
	}
	var buf bytes.Buffer
	err = fileTemplate.Execute(&buf, &File{
		Source:     filepath.Base(srcFile),
		Package:    astFile.Name.Name,
		GrammarPkg: grammarPkg,
		Rules:      rules,
	})
	if err != nil {
		return nil, err
	}
	out, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("generated code does not compile: %w\n%s", err, buf.Bytes())
	}
	return out, nil
}

// grammarPackageName returns the name the grammar package is imported under,
// or "" if it is not imported.
func grammarPackageName(f *ast.File) string {
	for _, imp := range f.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path != grammarImportPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "grammar"
	}
	return ""
}

// A File is the data for the file template.
type File struct {
	Source     string
	Package    string
	GrammarPkg string
	Rules      []*Rule
}

// RuleKind says which grammar type a rule embeds first.
type RuleKind int

const (
	SeqRule RuleKind = iota
	OneOfRule
	TokRule
)

// A Rule describes a rule struct type.
type Rule struct {
	Name       string
	Kind       RuleKind
	Drop       string // the drop tag of the Seq or OneOf field
	Fields     []*Field
	GrammarPkg string
	// For a OneOf: some alternatives are Seq rules of this file starting
	// with this rule (see Field.LeftRec); they are parsed by precedence
	// climbing.
	Loop bool
	// For a Seq: it starts with a rule field, so it gets a ParseRest method.
	Rest bool
	// The level of the prec tag of the Seq or OneOf field, 0 if none.
	Prec int
}

// HasVars is true if the rule needs package-level option variables.
func (r *Rule) HasVars() bool {
	if r.Drop != "" || r.Kind == OneOfRule {
		return true
	}
	for _, f := range r.Fields {
		if f.Tok != "" || f.Array || f.Pointer {
			return true
		}
	}
	return false
}

// A Field describes one field of a rule struct after the first.
type Field struct {
	Name      string
	Type      string // the field type as written, e.g. "*Number" or "[]Json"
	BaseType  string // the type without pointer or slice, e.g. "Number"
	Pointer   bool
	Array     bool
	Tok       string // the tok tag
	Sep       string // the sep tag
	Size      string // the size tag
	Trail     string // the trail tag
	LeftRec   bool   // alternative of a OneOf rule: a Seq rule of this file starting with it
	LeftField string // the name of that rule's first field
}

// Rest returns the fields after the first.
func (r *Rule) RestFields() []*Field { return r.Fields[1:] }

// UsesPrec is true if parsing the rule involves precedence bounds: it is an
// operator rule (a Seq with a level).
func (r *Rule) UsesPrec() bool { return r.Kind == SeqRule && r.Prec != 0 }

// Value is true for a plain (non-pointer, non-slice) field.
func (f *Field) Value() bool {
	return !f.Pointer && !f.Array
}

func collectRules(f *ast.File, grammarPkg string) ([]*Rule, error) {
	var rules []*Rule
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || typeSpec.Assign.IsValid() || len(structType.Fields.List) == 0 {
				continue
			}
			rule, err := ruleFromStruct(typeSpec.Name.Name, structType, grammarPkg)
			if err != nil {
				return nil, err
			}
			if rule != nil {
				rules = append(rules, rule)
			}
		}
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	markLeftRecursion(rules)
	return rules, nil
}

// markLeftRecursion finds the alternatives of OneOf rules that are Seq rules
// of the file starting with the OneOf rule itself (direct left recursion).
func markLeftRecursion(rules []*Rule) {
	byName := map[string]*Rule{}
	for _, r := range rules {
		byName[r.Name] = r
	}
	for _, r := range rules {
		if r.Kind == SeqRule && len(r.Fields) > 0 && r.Fields[0].Value() && r.Fields[0].Tok == "" {
			r.Rest = true
		}
		if r.Kind != OneOfRule {
			continue
		}
		for _, f := range r.Fields {
			alt := byName[f.BaseType]
			if !f.Pointer || alt == nil || alt.Kind != SeqRule || len(alt.Fields) == 0 {
				continue
			}
			if first := alt.Fields[0]; first.Value() && first.BaseType == r.Name {
				f.LeftRec, f.LeftField = true, first.Name
				r.Loop = true
			}
		}
	}
}

// ruleFromStruct returns the rule for a struct type, or nil if the struct is
// not a rule (its first field is not Seq, OneOf or Tok).
func ruleFromStruct(name string, structType *ast.StructType, grammarPkg string) (*Rule, error) {
	first := structType.Fields.List[0]
	if len(first.Names) > 0 {
		return nil, nil // the marker must be embedded
	}
	var kind RuleKind
	switch types.ExprString(first.Type) {
	case grammarPkg + ".Seq":
		kind = SeqRule
	case grammarPkg + ".OneOf":
		kind = OneOfRule
	case grammarPkg + ".Tok":
		kind = TokRule
	default:
		return nil, nil
	}
	rule := &Rule{Name: name, Kind: kind, Drop: tagValue(first, "drop"), GrammarPkg: grammarPkg}
	prec, err := grammar.PrecFromTag(tagValue(first, "prec"))
	if err != nil {
		return nil, fmt.Errorf("rule type %s: %w", name, err)
	}
	if prec != 0 && kind == TokRule {
		return nil, fmt.Errorf("rule type %s: a prec tag goes on a Seq or OneOf field", name)
	}
	rule.Prec = prec
	if kind == TokRule {
		if len(structType.Fields.List) > 1 {
			return nil, fmt.Errorf("Tok rule type %s must have exactly one field", name)
		}
		rule.Fields = []*Field{{Name: "Tok", Tok: tagValue(first, "tok")}}
		return rule, nil
	}
	for _, astField := range structType.Fields.List[1:] {
		fields, err := fieldsFromAST(astField, rule)
		if err != nil {
			return nil, err
		}
		rule.Fields = append(rule.Fields, fields...)
	}
	return rule, nil
}

// fieldsFromAST returns the fields declared by one struct field declaration
// (which may declare several names, e.g. "A, B Rule1").
func fieldsFromAST(astField *ast.Field, rule *Rule) ([]*Field, error) {
	proto := Field{
		Type:  types.ExprString(astField.Type),
		Tok:   tagValue(astField, "tok"),
		Sep:   tagValue(astField, "sep"),
		Size:  tagValue(astField, "size"),
		Trail: tagValue(astField, "trail"),
	}
	if tagValue(astField, "prec") != "" {
		return nil, fmt.Errorf("field %s of rule type %s: a prec tag goes on the Seq field of an operator rule", fieldNames(astField), rule.Name)
	}
	switch t := astField.Type.(type) {
	case *ast.StarExpr:
		proto.Pointer = true
		proto.BaseType = types.ExprString(t.X)
	case *ast.ArrayType:
		if t.Len != nil {
			return nil, fmt.Errorf("field %s of rule type %s: arrays are not supported, use a slice", fieldNames(astField), rule.Name)
		}
		proto.Array = true
		proto.BaseType = types.ExprString(t.Elt)
	default:
		if rule.Kind == OneOfRule {
			return nil, fmt.Errorf("field %s of rule type %s: OneOf fields must be pointers or slices", fieldNames(astField), rule.Name)
		}
		proto.BaseType = proto.Type
	}
	// Validate the tags the same way the runtime does.
	if proto.Array {
		if _, err := grammar.RepeatOptionsFromTags[grammar.Empty](proto.Size, proto.Sep, proto.Trail, rule.Drop, proto.Tok); err != nil {
			return nil, fmt.Errorf("field %s of rule type %s: %w", fieldNames(astField), rule.Name, err)
		}
	} else {
		if _, err := grammar.SizeOptionsFromTag(proto.Size); err != nil {
			return nil, fmt.Errorf("field %s of rule type %s: %w", fieldNames(astField), rule.Name, err)
		}
		if _, err := grammar.TrailModeFromTag(proto.Trail); err != nil {
			return nil, fmt.Errorf("field %s of rule type %s: %w", fieldNames(astField), rule.Name, err)
		}
	}
	if len(astField.Names) == 0 {
		// Embedded field: named after its type.
		f := proto
		f.Name = embeddedName(astField.Type)
		return []*Field{&f}, nil
	}
	var fields []*Field
	for _, name := range astField.Names {
		f := proto
		f.Name = name.Name
		fields = append(fields, &f)
	}
	return fields, nil
}

func fieldNames(f *ast.Field) string {
	if len(f.Names) == 0 {
		return embeddedName(f.Type)
	}
	names := make([]string, len(f.Names))
	for i, n := range f.Names {
		names[i] = n.Name
	}
	return strings.Join(names, ", ")
}

func embeddedName(e ast.Expr) string {
	switch ee := e.(type) {
	case *ast.StarExpr:
		return embeddedName(ee.X)
	case *ast.Ident:
		return ee.Name
	case *ast.SelectorExpr:
		return ee.Sel.Name
	case *ast.IndexExpr:
		return embeddedName(ee.X)
	case *ast.IndexListExpr:
		return embeddedName(ee.X)
	default:
		return ""
	}
}

func tagValue(f *ast.Field, key string) string {
	if f.Tag == nil {
		return ""
	}
	tag, _ := strconv.Unquote(f.Tag.Value)
	return reflect.StructTag(tag).Get(key)
}

var fileTemplate = template.Must(template.New("file").Funcs(template.FuncMap{
	"quote": strconv.Quote,
	"list":  func(items ...any) []any { return items },
	"add":   func(a, b int) int { return a + b },
}).Parse(`
{{- /* "tok" renders the token options argument for a field: the package-level
       variable if the field has a tok tag, else the zero value. */ -}}
{{- define "prec" }}{{ $i := index . 0 }}pending.Prec({{ $i }}){{ end -}}
{{- define "fields" }}
{{- $r := index . 0 }}{{ $fields := index . 1 }}{{ $from := index . 2 }}{{ $usePrec := index . 3 }}
{{- range $j, $f := $fields }}{{ $i := add $j $from }}
	// {{ .Name }} {{ .Type }}
	if s.Debug() {
		s.Logf("  .{{ .Name }} tok #%d", s.Save())
	}
{{- if $r.Drop }}
	_{{ $r.Name }}_drop.DropMatchingNextTokens(s)
{{- end }}
{{- if .Pointer }}
{{- if $usePrec }}
	s.SetPrec({{ template "prec" (list $i .) }})
{{- end }}
	if item, fieldErr := {{ $r.GrammarPkg }}.ParseOptional[{{ .BaseType }}](s, {{ template "tok" (list $r .) }}, _{{ $r.Name }}_{{ .Name }}_first); fieldErr != nil {
		err = err.Merge(fieldErr)
	} else {
		r.{{ .Name }} = item
		itemCount++
	}
{{- else if .Array }}
{{- if $usePrec }}
	rep{{ $i }} := _{{ $r.Name }}_{{ .Name }}_rep
	rep{{ $i }}.Prec = {{ template "prec" (list $i .) }}
	if items, n, fieldErr, ok := {{ $r.GrammarPkg }}.ParseRepeated[{{ .BaseType }}](s, {{ template "tok" (list $r .) }}, rep{{ $i }}); ok {
{{- else }}
	if items, n, fieldErr, ok := {{ $r.GrammarPkg }}.ParseRepeated[{{ .BaseType }}](s, {{ template "tok" (list $r .) }}, _{{ $r.Name }}_{{ .Name }}_rep); ok {
{{- end }}
		err = err.Merge(fieldErr)
		r.{{ .Name }} = items
		itemCount += n
	} else {
		return err.Merge(fieldErr)
	}
{{- else }}
{{- if $usePrec }}
	s.SetPrec({{ template "prec" (list $i .) }})
{{- end }}
	if fieldErr := {{ $r.GrammarPkg }}.ParseWithOptions(&r.{{ .Name }}, s, {{ template "tok" (list $r .) }}); fieldErr != nil {
		return err.Merge(fieldErr)
	}
	itemCount++
{{- end }}
{{- end }}
{{- end }}
{{- define "tok" }}{{ $r := index . 0 }}{{ $f := index . 1 }}{{ if $f.Tok }}_{{ $r.Name }}_{{ $f.Name }}_tok{{ else }}{{ $r.GrammarPkg }}.TokenOptions{}{{ end }}{{ end -}}
// Code generated by genparse from {{ .Source }}; DO NOT EDIT.

//go:build !nocompiledgrammar

package {{ .Package }}

import {{ .GrammarPkg }} "` + grammarImportPath + `"

{{- $g := .GrammarPkg }}
{{- range .Rules }}
{{- $r := . }}

{{ if eq .Kind 2 -}}
var _{{ .Name }}_tok = {{ $g }}.TokenOptionsFromTag({{ quote (index .Fields 0).Tok }})

// Parse implements {{ $g }}.Parser for {{ .Name }} without reflection.
func (r *{{ .Name }}) Parse(_ any, s *{{ $g }}.ParserState, _ {{ $g }}.TokenOptions) *{{ $g }}.ParseError {
	return r.Tok.Parse(&r.Tok, s, _{{ .Name }}_tok)
}
{{- else }}
{{- if .HasVars }}
var (
{{- if .Drop }}
	_{{ .Name }}_drop = {{ $g }}.TokenOptionsFromTag({{ quote .Drop }})
{{- end }}
{{- if eq .Kind 1 }}
	_{{ .Name }}_alts = {{ $g }}.AlternativesOf[{{ .Name }}]()
{{- if not .Loop }}
	_{{ .Name }}_plain = {{ $g }}.PlainAlternatives[{{ .Name }}]()
{{- end }}
{{- end }}
{{- if .Loop }}
	_{{ .Name }}_climb    = {{ $g }}.CanClimb[{{ .Name }}]()
	_{{ .Name }}_dispatch = {{ $g }}.DispatchOf[{{ .Name }}]()
{{- end }}
{{- range .Fields }}
{{- if .Tok }}
	_{{ $r.Name }}_{{ .Name }}_tok = {{ $g }}.TokenOptionsFromTag({{ quote .Tok }})
{{- end }}
{{- if .Array }}
	_{{ $r.Name }}_{{ .Name }}_rep = {{ $g }}.MustRepeatOptions[{{ .BaseType }}]({{ quote .Size }}, {{ quote .Sep }}, {{ quote .Trail }}, {{ quote $r.Drop }}, {{ quote .Tok }})
{{- end }}
{{- if .Pointer }}
	_{{ $r.Name }}_{{ .Name }}_first = {{ $g }}.FirstSetOf[{{ .BaseType }}]({{ template "tok" (list $r .) }})
{{- end }}
{{- end }}
)
{{- end }}

{{ if eq .Kind 0 -}}
// Parse implements {{ $g }}.Parser for {{ .Name }} without reflection; it
// does what {{ $g }}.Seq.Parse does.
func (r *{{ .Name }}) Parse(_ any, s *{{ $g }}.ParserState, _ {{ $g }}.TokenOptions) (err *{{ $g }}.ParseError) {
	itemCount := 0
{{- if .UsesPrec }}
	pending := s.TakePendingOr({{ .Prec }})
{{- end }}
{{- template "fields" (list $r .Fields 0 .UsesPrec) }}
	if itemCount == 0 {
		return {{ $g }}.EmptyMatchError(s, {{ quote .Name }})
	}
	return nil
}
{{- if .Rest }}

// ParseRest parses the fields after the first, which is already set (the
// left operand); the others are bound to the precedence level.
func (r *{{ .Name }}) ParseRest(s *{{ $g }}.ParserState, pending {{ $g }}.PendingPrec) (err *{{ $g }}.ParseError) {
	itemCount := 1
{{- template "fields" (list $r .RestFields 1 true) }}
	_ = itemCount
	return nil
}
{{- end }}
{{- else -}}
{{- if .Loop }}
// Parse implements {{ $g }}.Parser for {{ .Name }} without reflection, by
// precedence climbing: the alternatives that start with {{ .Name }} itself
// extend the value parsed by the others, unless the rule turns out to be
// left recursive through other rules too.
func (r *{{ .Name }}) Parse(_ any, s *{{ $g }}.ParserState, _ {{ $g }}.TokenOptions) (err *{{ $g }}.ParseError) {
	if !_{{ .Name }}_climb {
		return r.parseGrowing(s)
	}
	c := {{ $g }}.ChoiceOf[{{ .Name }}](s)
{{- if .Drop }}
	_{{ .Name }}_drop.DropMatchingNextTokens(s)
{{- end }}
	seeded := false
{{- range $i, $f := .Fields }}
{{- if not .LeftRec }}
	// {{ .Name }} {{ .Type }}
{{- if .Pointer }}
	if seeded {
	} else if ok, tryErr := c.Try(_{{ $r.Name }}_alts[{{ $i }}]); !ok {
		err = err.Merge(tryErr)
	} else if item, fieldErr := {{ $g }}.ParseAlternative[{{ .BaseType }}](s, &c, _{{ $r.Name }}_alts[{{ $i }}], {{ template "tok" (list $r .) }}); fieldErr == nil {
		r.{{ .Name }} = item
		seeded = true
	} else {
		err = err.Merge(fieldErr)
	}
{{- else }}
	if seeded {
	} else if ok, tryErr := c.Try(_{{ $r.Name }}_alts[{{ $i }}]); !ok {
		err = err.Merge(tryErr)
	} else if items, n, fieldErr, ok := {{ $g }}.ParseRepeated[{{ .BaseType }}](s, {{ template "tok" (list $r .) }}, _{{ $r.Name }}_{{ .Name }}_rep); ok && n > 0 {
		r.{{ .Name }} = items
		seeded = true
	} else {
		err = err.Merge(fieldErr)
	}
{{- end }}
{{- end }}
{{- end }}
	if !seeded {
		return err
	}
	for {
		mask := _{{ .Name }}_dispatch.Lookup(s.Peek(), c.Level())
		if mask == 0 {
			break
		}
{{- range $i, $f := .Fields }}
{{- if .LeftRec }}
		// {{ .Name }} {{ .Type }}
		if mask&(1<<{{ $i }}) != 0 {
			start := s.Save()
			item := &{{ .BaseType }}{}
			item.{{ .LeftField }} = *r
			if fieldErr := item.ParseRest(s, c.PendingFor(_{{ $r.Name }}_alts[{{ $i }}])); fieldErr == nil && s.Save() > start {
				*r = {{ $r.Name }}{ {{ .Name }}: item}
				continue
			} else {
				err = err.Merge(fieldErr)
				s.Restore(start)
			}
		}
{{- end }}
{{- end }}
		break
	}
	_{{ .Name }}_dispatch.Expect(s, c.Level())
	return nil
}

// parseGrowing is Parse for when the rule is left recursive through other
// rules: left recursion is resolved by seed growing in the memo.
func (r *{{ .Name }}) parseGrowing(s *{{ $g }}.ParserState) (err *{{ $g }}.ParseError) {
	c := {{ $g }}.ChoiceOf[{{ .Name }}](s)
{{- else }}
// Parse implements {{ $g }}.Parser for {{ .Name }} without reflection; it
// does what {{ $g }}.OneOf.Parse does.
func (r *{{ .Name }}) Parse(_ any, s *{{ $g }}.ParserState, _ {{ $g }}.TokenOptions) (err *{{ $g }}.ParseError) {
	if !_{{ .Name }}_plain {
		return r.parseGeneral(s)
	}
{{- if .Drop }}
	_{{ .Name }}_drop.DropMatchingNextTokens(s)
{{- end }}
{{- range .Fields }}
	// {{ .Name }} {{ .Type }}
{{- if .Pointer }}
	if item, fieldErr := {{ $g }}.ParseOptional[{{ .BaseType }}](s, {{ template "tok" (list $r .) }}, _{{ $r.Name }}_{{ .Name }}_first); fieldErr == nil {
		r.{{ .Name }} = item
		return nil
	} else {
		err = err.Merge(fieldErr)
	}
{{- else }}
	if items, n, fieldErr, ok := {{ $g }}.ParseRepeated[{{ .BaseType }}](s, {{ template "tok" (list $r .) }}, _{{ $r.Name }}_{{ .Name }}_rep); ok && n > 0 {
		r.{{ .Name }} = items
		return nil
	} else {
		err = err.Merge(fieldErr)
	}
{{- end }}
{{- end }}
	return err
}

// parseGeneral is Parse for when an alternative is left recursive or has a
// precedence.
func (r *{{ .Name }}) parseGeneral(s *{{ $g }}.ParserState) (err *{{ $g }}.ParseError) {
	c := {{ $g }}.ChoiceOf[{{ .Name }}](s)
{{- end }}
{{- if .Drop }}
	_{{ .Name }}_drop.DropMatchingNextTokens(s)
{{- end }}
{{- range $i, $f := .Fields }}
	// {{ .Name }} {{ .Type }}
{{- if .Pointer }}
	if ok, tryErr := c.Try(_{{ $r.Name }}_alts[{{ $i }}]); !ok {
		err = err.Merge(tryErr)
	} else if item, fieldErr := {{ $g }}.ParseAlternative[{{ .BaseType }}](s, &c, _{{ $r.Name }}_alts[{{ $i }}], {{ template "tok" (list $r .) }}); fieldErr == nil {
		r.{{ .Name }} = item
		return nil
	} else {
		err = err.Merge(fieldErr)
	}
{{- else }}
	if ok, tryErr := c.Try(_{{ $r.Name }}_alts[{{ $i }}]); !ok {
		err = err.Merge(tryErr)
	} else if items, n, fieldErr, ok := {{ $g }}.ParseRepeated[{{ .BaseType }}](s, {{ template "tok" (list $r .) }}, _{{ $r.Name }}_{{ .Name }}_rep); ok && n > 0 {
		r.{{ .Name }} = items
		return nil
	} else {
		err = err.Merge(fieldErr)
	}
{{- end }}
{{- end }}
	return err
}
{{- end }}
{{- end }}
{{- end }}
`))
