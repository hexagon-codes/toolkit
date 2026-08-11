// examples 是独立模块，不属于 toolkit 库的发布表面。
// 这样 go get github.com/hexagon-codes/toolkit 不会拉入示例及其依赖图。
// 通过本地 replace 始终绑定当前源码树，不依赖尚未发布的 toolkit 版本。
module github.com/hexagon-codes/toolkit/examples

go 1.25.12

require (
	github.com/hexagon-codes/toolkit v0.0.0
	github.com/hibiken/asynq v0.25.1
	github.com/redis/go-redis/v9 v9.17.3
)

require (
	filippo.io/edwards25519 v1.1.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/go-sql-driver/mysql v1.9.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/time v0.8.0 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)

replace github.com/hexagon-codes/toolkit => ..
