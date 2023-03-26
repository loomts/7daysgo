package consistent_hash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

type Hash func(data []byte) uint32

type Map struct {
	replicate int
	hash      Hash
	keys      []int
	hashMap   map[int]string // hash->machine
}

func New(replicas int, fn Hash) *Map {
	m := &Map{
		replicate: replicas,
		hash:      fn,
		hashMap:   make(map[int]string),
	}
	if m.hash == nil {
		m.hash = crc32.ChecksumIEEE
	}
	return m
}

func (m *Map) Add(keys ...string) {
	for _, key := range keys {
		for i := 0; i < m.replicate; i++ {
			hash := int(m.hash([]byte(strconv.Itoa(i) + key)))
			m.keys = append(m.keys, hash)
			m.hashMap[hash] = key
		}
	}
	sort.Ints(m.keys)
}

func (m *Map) Get(key string) string {
	if len(m.keys) == 0 {
		return ""
	}
	hash := int(m.hash([]byte(key)))
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= hash
	}) // find the first i that match m.keys[i] >= hash
	return m.hashMap[m.keys[idx%len(m.keys)]]
}
