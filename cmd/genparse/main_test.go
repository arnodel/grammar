package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The generated files committed with the examples must be up to date with
// the generator and the example sources: run "go generate ./..." otherwise.
func TestExamplesUpToDate(t *testing.T) {
	sources, err := filepath.Glob("../../examples/*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, src := range sources {
		content, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(content, []byte("//go:generate go run github.com/arnodel/grammar/cmd/genparse")) {
			continue
		}
		checked++
		t.Run(filepath.Base(filepath.Dir(src)), func(t *testing.T) {
			want, err := os.ReadFile(OutputPath(src))
			if err != nil {
				t.Fatalf("missing generated file: %s", err)
			}
			got, err := Generate(src)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s is out of date, run go generate ./...", OutputPath(src))
			}
		})
	}
	if checked == 0 {
		t.Fatal("no example with a go:generate directive found")
	}
}
