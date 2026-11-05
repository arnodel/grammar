package grammar

// blockSize is the number of values in a block of blocks[T].
const blockSize = 256

// blocks hands out values of type T allocated in blocks, rather than one
// allocation per value.  A block is an array, so values never move and
// pointers to them stay valid for as long as they are referenced; a full
// block is forgotten by blocks and lives on through those pointers.  The zero
// value is ready to use.
type blocks[T any] struct {
	block *[blockSize]T
	used  int // values of block handed out
}

// new returns a pointer to a new zero value.
func (b *blocks[T]) new() *T {
	if b.block == nil || b.used == len(b.block) {
		b.block = new([blockSize]T)
		b.used = 0
	}
	v := &b.block[b.used]
	b.used++
	return v
}
