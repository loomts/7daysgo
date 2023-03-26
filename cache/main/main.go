package main

import (
	"7daysgo/cache"
	"flag"
	"fmt"
	"log"
	"net/http"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func createGroup() *cache.Group {
	return cache.MakeGroup("scores", 2<<10, cache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
}

func startCacheServer(addr string, addrs []string, gee *cache.Group) {
	server := cache.MakeHTTPPool(addr)
	server.Set(addrs...)
	gee.RegisterPeers(server)
	log.Println("cache is running at", addr)
	log.Fatal(http.ListenAndServe(addr[7:], server))
}

func startAPIServer(apiAddr string, gee *cache.Group) {
	http.Handle("/api", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("key")
			view, err := gee.Get(key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(view.ByteSlice())
		}))
	log.Println("frontend server is running at", apiAddr)
	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))
}

func startCacheServerGRPC(addr string, addrs []string, gee *cache.Group) {
	server := cache.MakeGrpcPool(addr)
	server.Set(addrs...)
	gee.RegisterPeers(server)
	log.Println("cache is running at", addr)
	server.Run()
}

func main() {
	var port int
	var api bool
	flag.IntVar(&port, "port", 8004, "cache server port")
	flag.BoolVar(&api, "api", true, "Start a api server?")
	flag.Parse()

	apiAddr := "http://localhost:9999"
	addrMap := map[int]string{
		8001: "116.205.130.21:8001", //HTTP VERSION: http://localhost:8001
		8002: "116.205.130.21:8002", //HTTP VERSION: http://localhost:8002
		8003: "116.205.130.21:8003", //HTTP VERSION: http://localhost:8003
		//8004: "localhost:8004",
	}

	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}

	gee := createGroup()
	if api {
		go startAPIServer(apiAddr, gee)
	}
	//HTTP VERSION: startCacheServer(addrMap[port], addrs, gee)
	startCacheServerGRPC(addrMap[port], addrs, gee)
}
