// examples 是独立模块，不属于 toolkit 库的发布表面。
// 这样 go get github.com/hexagon-codes/toolkit 不会拉入示例及其依赖图。
// 本地开发经仓库根的 go.work 解析依赖；发版时 go.work 移除，示例按版本号构建。
module github.com/hexagon-codes/toolkit/examples

go 1.25.5

require (
	github.com/hexagon-codes/toolkit v0.0.6
	github.com/hibiken/asynq v0.25.1
	github.com/redis/go-redis/v9 v9.17.3
)
