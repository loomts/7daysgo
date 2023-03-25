1. LFU
双hash，
2. byteView -> special Value
3. cache -> Lock RW
4. gee -> server
5. http -> Opens the HTTP interface for client calls

```zsh
lsof -i :8001 -i :8002 -i :8003 | grep LISTEN | awk '{print $2}' | xargs kill -9
```