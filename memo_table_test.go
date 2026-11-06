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

func TestMemoSeen(t *testing.T) {
	var m memoTable
	// First time false, second time true, independently per position and
	// rule id, including ids beyond the first word and positions beyond the
	// initial size.
	for _, pos := range []int{0, 5, 1000} {
		for _, id := range []int{0, 1, 63, 64, 200} {
			if m.parsedBefore(pos, id) {
				t.Errorf("pos %d id %d: seen before first parse", pos, id)
			}
			if !m.parsedBefore(pos, id) {
				t.Errorf("pos %d id %d: not seen after first parse", pos, id)
			}
		}
	}
	// Widening keeps what was marked.
	if !m.parsedBefore(5, 63) || m.parsedBefore(5, 62) || m.parsedBefore(6, 63) {
		t.Error("bits lost or gained when widening")
	}
}

// A rule parsed once at a position is not memoised; parsed again there it is.
func TestLazyMemo(t *testing.T) {
	toks, _ := parseTestTokenise(`1 + 2`)
	s := &ParserState{TokenStream: toks}
	var n1, n2 ptSum
	if err := ParseWithOptions(&n1, s, TokenOptions{}); err != nil {
		t.Fatal(err)
	}
	key := memoKey{ruleType: reflect.TypeOf(&n1)}
	if s.memo.get(0, key) != nil {
		t.Error("memoised on first parse")
	}
	s.Restore(0)
	if err := ParseWithOptions(&n2, s, TokenOptions{}); err != nil {
		t.Fatal(err)
	}
	if v := s.memo.get(0, key); v == nil || !v.ok || v.pos != 3 {
		t.Errorf("not memoised on second parse: %+v", v)
	}
}
