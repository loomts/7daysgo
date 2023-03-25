package cache

import (
	"container/list"
)

type Value interface {
	Len() int
}

type Node struct {
	k    string
	v    Value
	freq int
}

type LFUCache struct {
	freq map[int]*list.List
	k2n  map[string]*list.Element
	cap  int
	mi   int
}

func (lfu *LFUCache) push(cnt int, n *Node) *list.Element {
	if _, ok := lfu.freq[cnt]; !ok {
		lfu.freq[cnt] = list.New()
	}
	return lfu.freq[cnt].PushBack(n)
}

func MakeLFU(capacity int) *LFUCache {
	return &LFUCache{
		freq: make(map[int]*list.List),
		k2n:  make(map[string]*list.Element),
		cap:  capacity,
		mi:   0,
	}
}

func (lfu *LFUCache) Get(key string) (Value, bool) {
	if le, ok := lfu.k2n[key]; ok {
		node := le.Value.(*Node)
		lfu.k2n[key] = lfu.push(node.freq+1, &Node{k: key, v: node.v, freq: node.freq + 1})
		lfu.freq[node.freq].Remove(le)
		if lfu.mi == node.freq && lfu.freq[node.freq].Len() == 0 {
			lfu.mi = node.freq + 1
		}
		return node.v, true
	} else {
		return nil, false
	}
}

func (lfu *LFUCache) Put(key string, value Value) {
	if le, ok := lfu.k2n[key]; ok {
		node := le.Value.(*Node)
		lfu.k2n[key] = lfu.push(node.freq+1, &Node{k: key, v: value, freq: node.freq + 1})
		lfu.freq[node.freq].Remove(le)
		if lfu.mi == node.freq && lfu.freq[node.freq].Len() == 0 {
			lfu.mi = node.freq + 1
		}
	} else {
		if len(lfu.k2n) == lfu.cap {
			head := lfu.freq[lfu.mi].Front()
			delete(lfu.k2n, head.Value.(*Node).k)
			lfu.freq[lfu.mi].Remove(head)
		}
		lfu.k2n[key] = lfu.push(1, &Node{k: key, v: value, freq: 1})
		lfu.mi = 1
	}
}
