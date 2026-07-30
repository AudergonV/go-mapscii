package tileprovider

import "container/list"

// lruCache is a small fixed-capacity, least-recently-used cache. It is
// not safe for concurrent use without external locking.
type lruCache struct {
	capacity int
	ll       *list.List
	items    map[interface{}]*list.Element
}

type lruEntry struct {
	key   interface{}
	value interface{}
}

func newLRUCache(capacity int) *lruCache {
	if capacity < 1 {
		capacity = 1
	}
	return &lruCache{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[interface{}]*list.Element, capacity),
	}
}

func (c *lruCache) Get(key interface{}) (interface{}, bool) {
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*lruEntry).value, true
}

func (c *lruCache) Put(key, value interface{}) {
	if el, ok := c.items[key]; ok {
		el.Value.(*lruEntry).value = value
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&lruEntry{key: key, value: value})
	c.items[key] = el
	if c.ll.Len() > c.capacity {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			delete(c.items, oldest.Value.(*lruEntry).key)
		}
	}
}
