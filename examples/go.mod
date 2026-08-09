// examples 是独立模块，不属于 toolkit 库的发布表面。
// 这样 go get github.com/hexagon-codes/toolkit 不会拉入示例及其依赖图。
// 源码树验证由 ../scripts/verify-examples.sh 创建临时 go.work 并绑定当前 toolkit。
// 独立构建仅面向已发布版本；toolkit 新版本发布后需同步更新下方版本号。
module github.com/hexagon-codes/toolkit/examples

go 1.25.5

require (
	github.com/hexagon-codes/toolkit v0.0.6
	github.com/hibiken/asynq v0.25.1
	github.com/redis/go-redis/v9 v9.17.3
)
