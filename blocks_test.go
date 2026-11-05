package grammar

import "testing"

func TestBlocks(t *testing.T) {
	var b blocks[int]
	var ptrs []*int
	for i := 0; i < 3*blockSize+1; i++ {
		p := b.new()
		if *p != 0 {
			t.Fatalf("value %d not zero", i)
		}
		*p = i
		ptrs = append(ptrs, p)
	}
	for i, p := range ptrs {
		if *p != i {
			t.Fatalf("value %d is %d", i, *p)
		}
	}
	if ptrs[0] == ptrs[1] || ptrs[blockSize-1] == ptrs[blockSize] {
		t.Fatal("same pointer handed out twice")
	}
}
