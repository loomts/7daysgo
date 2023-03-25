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

func (c *cache) get(key string) (ByteView, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lfu == nil {
		c.lfu = MakeLFU(c.cacheBytes)
	}
	if v, ok := c.lfu.Get(key); ok {
		return v.(ByteView), ok
	} else {
		return ByteView{}, ok
	}
}
