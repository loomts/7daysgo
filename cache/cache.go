package cache

import (
	"sync"
)

type cache struct {
	mu         sync.Mutex
	lfu        *LFUCache
	cacheBytes int
}

func (c *cache) put(key string, value ByteView) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lfu == nil {
		c.lfu = MakeLFU(c.cacheBytes)
	}
	c.lfu.Put(key, value)
}

func (c *cache) get(key string) (value ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lfu == nil {
		return
	}
	if v, ok := c.lfu.Get(key); ok {
		return v.(ByteView), ok
	}
	return
}
