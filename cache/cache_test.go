package cache

import (
	"encoding/base64"
	"fmt"
	"github.com/go-redis/redis"

	"math/rand"
	"reflect"
	"testing"
	"time"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func CacheTest(t *testing.T) {
	start := time.Now()
	loadCounts := make(map[string]int, len(db))
	gee := MakeGroup("scores", 10*(1<<20), GetterFunc(
		func(key string) ([]byte, error) {
			//log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				if _, ok := loadCounts[key]; !ok {
					loadCounts[key] = 0
				}
				loadCounts[key] += 1
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
	for i := 0; i < 100; i++ {
		for k, v := range db {
			if view, err := gee.Get(k); err != nil || view.String() != v {
				t.Fatal("failed to get value of Tom")
			} // load from callback function
			if _, err := gee.Get(k); err != nil || loadCounts[k] > 1 {
				t.Fatalf("cache %s miss", k)
			} // cache hit
		}
	}
	fmt.Println("cache::----------------", time.Since(start).Seconds(), "---------------------")
}
func RedisTest(t *testing.T) {
	start := time.Now()
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	client.ConfigSet("maxmemory-policy", "allkeys-lfu")
	client.ConfigSet("maxmemory", "100mb")
	for i := 0; i < 100; i++ {
		for k, v := range db {
			err := client.Set(k, v, 0).Err()
			if err != nil {
				panic(err)
			}
			val, err := client.Get(k).Result()
			if err != nil {
				panic(err)
			}
			if val != v {
				t.Fatalf("Redis fail to get data")
			}
		}
	}
	fmt.Println("redis::----------------", time.Since(start).Seconds(), "---------------------")

}
func generateRandomString(length int) string {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}

func TestCompareWithRedis(t *testing.T) {
	for i := 1; i <= 1000; i++ {
		k, v := generateRandomString(rand.Int()%i+i), generateRandomString(rand.Int()%i+i)
		db[k] = v
	}
	CacheTest(t)
	RedisTest(t)
}

func TestGetter(t *testing.T) {
	var f Getter = GetterFunc(func(key string) ([]byte, error) {
		return []byte(key), nil
	})

	expect := []byte("key")
	if v, _ := f.Get("key"); !reflect.DeepEqual(v, expect) {
		t.Errorf("callback failed")
	}
}
