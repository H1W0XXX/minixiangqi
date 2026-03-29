package engine

import "container/list"

const nnEvalCacheCapacity = 16384

type nnCacheKey struct {
	Hash         uint64
	Stage        int8
	ChosenSquare int16
}

type nnCacheEntry struct {
	key   nnCacheKey
	value *NNResult
}

type nnEvalCache struct {
	capacity int
	ll       *list.List
	items    map[nnCacheKey]*list.Element
}

func newNNEvalCache(capacity int) *nnEvalCache {
	if capacity <= 0 {
		capacity = nnEvalCacheCapacity
	}
	return &nnEvalCache{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[nnCacheKey]*list.Element, capacity),
	}
}

func (c *nnEvalCache) Get(key nnCacheKey) (*NNResult, bool) {
	if c == nil {
		return nil, false
	}
	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(elem)
	entry := elem.Value.(*nnCacheEntry)
	return cloneNNResult(entry.value), true
}

func (c *nnEvalCache) Put(key nnCacheKey, value *NNResult) {
	if c == nil || value == nil {
		return
	}
	if elem, ok := c.items[key]; ok {
		c.ll.MoveToFront(elem)
		elem.Value.(*nnCacheEntry).value = cloneNNResult(value)
		return
	}
	elem := c.ll.PushFront(&nnCacheEntry{key: key, value: cloneNNResult(value)})
	c.items[key] = elem
	if c.ll.Len() <= c.capacity {
		return
	}
	tail := c.ll.Back()
	if tail == nil {
		return
	}
	c.ll.Remove(tail)
	entry := tail.Value.(*nnCacheEntry)
	delete(c.items, entry.key)
}

func cloneNNResult(src *NNResult) *NNResult {
	if src == nil {
		return nil
	}
	dst := *src
	if src.Policy != nil {
		dst.Policy = append([]float32(nil), src.Policy...)
	}
	return &dst
}
