# Asynq Queue Package

## 功能特性

- ✅ 任务入队与消费
- ✅ 定时任务调度
- ✅ 任务重试和死信队列
- ✅ 熔断器和背压控制
- ✅ 任务状态机管理
- ✅ Prometheus 指标监控
- ✅ 任务轮询锁机制
- ✅ 健康检查
- ✅ 中间件支持

## 依赖说明

### 必需实现的接口

1. **AsynqConfig** - 配置接口
   - `GetRedisAddrs() []string` - Redis 地址列表
   - `GetRedisPassword() string` - Redis 密码
   - `GetRedisUsername() string` - Redis 用户名（Redis 6.0+ ACL）
   - `GetConcurrency() int` - Worker 并发数
   - `GetQueuePrefix() string` - 队列前缀（多环境隔离）
   - `IsPollingEnabled() bool` - 是否启用轮询
   - `IsRedisEnabled() bool` - 是否启用 Redis

2. **AsynqLogger** - 日志接口
   - `Log(msg string)` - 普通日志
   - `LogSkip(skip int, msg string)` - 带调用栈跳过的日志
   - `Error(msg string)` - 错误日志
   - `ErrorSkip(skip int, msg string)` - 带调用栈跳过的错误日志

### 使用方式

#### 方案 1：使用默认实现（快速开始）

```go
import "github.com/everyday-items/toolkit/infra/queue/asynq"

// 使用默认配置
config := &asynq.DefaultAsynqConfig{
    RedisAddrs:     []string{"localhost:6379"},
    RedisPassword:  "",
    Concurrency:    10,
    PollingEnabled: true,
    RedisEnabled:   true,
}

// 使用标准输出日志
logger := &asynq.StdLogger{}
```

#### 方案 2：适配现有项目

如果你有现有的配置和日志系统，创建适配器：

```go
// 适配器示例
type MyConfigAdapter struct {
    // 你的配置结构
}

func (c *MyConfigAdapter) GetRedisAddrs() []string {
    return common.GetRedisAddrs() // 适配到你的配置
}

func (c *MyConfigAdapter) GetRedisPassword() string {
    return common.GetRedisPassword()
}

// ... 实现其他方法

type MyLoggerAdapter struct{}

func (l *MyLoggerAdapter) Log(msg string) {
    common.SysLog(msg) // 适配到你的日志系统
}

func (l *MyLoggerAdapter) Error(msg string) {
    common.SysError(msg)
}

// ... 实现其他方法
```

#### 方案 3：自定义实现

如果你有特定需求，可以完全自定义配置和日志实现。

## 文件说明

| 文件 | 说明 |
|------|------|
| `config.go` | 配置和日志接口定义 |
| `manager.go` | Asynq 管理器核心实现 |
| `task.go` | 任务构建器和辅助函数 |
| `task_types.go` | 任务类型定义 |
| `init.go` | 轮询系统初始化 |
| `queues.go` | 队列配置管理 |
| `adapter.go` | 适配器和辅助函数 |
| `middleware.go` | 中间件实现 |
| `metrics.go` | Prometheus 指标 |
| `health.go` | 健康检查 |
| `circuit_breaker.go` | 熔断器 |
| `backpressure.go` | 背压控制 |
| `dead_letter.go` | 死信队列管理 |
| `state_machine.go` | 任务状态机 |
| `polling_lock.go` | 轮询分布式锁 |
| `task_tracer.go` | 任务追踪 |
| `errors.go` | 错误定义 |
| `testing_helpers.go` | 测试辅助函数 |

## 当前状态

✅ **已完成接口化改造**

此包已完全解耦外部依赖，可直接在任何 Go 项目中使用：
- 所有配置通过 `ConfigProvider` 接口提供
- 所有日志通过 `Logger` 接口输出
- 提供 `DefaultConfigProvider` 和 `StdLogger` 作为开箱即用的默认实现
- 任务类型和队列名称均为通用定义，业务方可根据需要自定义

### 下一步工作

1. 添加完整的使用示例到 `examples/infra/`
2. 添加单元测试
3. 添加性能基准测试
4. 完善 API 文档

## 贡献

欢迎提交 PR 来帮助改进此包！
