package grammar

import (
	"sync"
	"sync/atomic"
)

// atomicMap is a map that is safe for concurrent use and as cheap to read as
// a plain map, for caches that are written to a few times and read a lot: a
// write copies the map.
type atomicMap[K comparable, V any] struct {
	mu sync.Mutex
	m  atomic.Pointer[map[K]V]
}

func (a *atomicMap[K, V]) load(k K) (V, bool) {
	m := a.m.Load()
	if m == nil {
		var zero V
		return zero, false
	}
	v, ok := (*m)[k]
	return v, ok
}

func (a *atomicMap[K, V]) store(k K, v V) {
	a.storeAll(map[K]V{k: v})
}

// storeAll adds the entries of kvs, in one copy of the map.  An entry already
// present is kept, so that concurrent builders of the same (immutable) values
// agree on which one is published.
func (a *atomicMap[K, V]) storeAll(kvs map[K]V) {
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.m.Load()
	var m map[K]V
	if old == nil {
		m = make(map[K]V, len(kvs))
	} else {
		m = make(map[K]V, len(*old)+len(kvs))
		for k, v := range *old {
			m[k] = v
		}
	}
	for k, v := range kvs {
		if _, present := m[k]; !present {
			m[k] = v
		}
	}
	a.m.Store(&m)
}
