module github.com/quangledang23/mezon-sdk-go/redisstore

go 1.26.4

require (
	github.com/alicebob/miniredis/v2 v2.38.0
	github.com/quangledang23/mezon-sdk-go v0.1.1
	github.com/redis/go-redis/v9 v9.7.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

// During development the adapter builds against the core module in this repo.
// Remove or keep this replace as preferred; when consumed as a dependency the
// require version above is used and this directive is ignored.
replace github.com/quangledang23/mezon-sdk-go => ../
