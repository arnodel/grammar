package grammar

import (
	"reflect"
	"testing"
)

func TestMemoTable(t *testing.T) {
	var table memoTable
	kA := memoKey{ruleType: reflect.TypeOf(0)}
	kB := memoKey{ruleType: reflect.TypeOf(""), extra: "x"}
	kC := memoKey{ruleType: reflect.TypeOf(""), extra: "y"}

	if table.get(0, kA) != nil || table.get(1000, kA) != nil {
		t.Fatal("found an entry in an empty table")
	}
	table.delete(5, kA) // no-op

	a0 := table.add(0, kA)
	b0 := table.add(0, kB)
	c0 := table.add(0, kC)
	a7 := table.add(7, kA)
	if table.get(0, kA) != a0 || table.get(0, kB) != b0 || table.get(0, kC) != c0 || table.get(7, kA) != a7 {
		t.Fatal("entries not found")
	}
	if table.get(7, kB) != nil || table.get(1, kA) != nil {
		t.Fatal("found entries that were not added")
	}

	// Entries stay where they are when the table grows, so pointers to
	// them remain valid.
	a0.pos = 42
	for i := 1; i < 5000; i++ {
		table.add(i, kA)
	}
	if table.get(0, kA) != a0 || a0.pos != 42 || table.get(4999, kA) == nil {
		t.Fatal("entries lost after growth")
	}

	// Deleting from the middle, head and tail of a position's list.
	table.delete(0, kB)
	if table.get(0, kB) != nil || table.get(0, kA) != a0 || table.get(0, kC) != c0 {
		t.Fatal("delete middle")
	}
	table.delete(0, kC)
	table.delete(0, kA)
	if table.get(0, kA) != nil || table.get(0, kC) != nil || table.heads[0] != nil {
		t.Fatal("delete head and tail")
	}
	// Re-adding after a delete works.
	if table.add(0, kA) == a0 || table.get(0, kA) == nil {
		t.Fatal("re-add")
	}
}
