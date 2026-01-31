# Asynq 使用指南

## 快速开始

### 1. 安装

```bash
go get github.com/everyday-items/toolkit/infra/queue/asynq
```

### 2. 基本概念

**Asynq** 是一个基于 Redis 的分布式任务队列，本包提供了生产级的封装。

**核心组件：**
- **Manager**: 管理器，负责任务入队和 Worker 调度
- **Worker**: 任务处理器，实现具体业务逻辑
- **Queue**: 队列，不同优先级的任务队列
- **Payload**: 任务载荷，包含任务所需的数据

### 3. 依赖注入配置

使用前必须配置依赖注入：

```go
import "github.com/everyday-items/toolkit/infra/queue/asynq"

// 1. 实现 Logger 接口
type MyLogger struct{}

func (l *MyLogger) Log(msg string)            { /* 实现 */ }
func (l *MyLogger) LogSkip(skip int, msg string) { /* 实现 */ }
func (l *MyLogger) Error(msg string)          { /* 实现 */ }
func (l *MyLogger) ErrorSkip(skip int, msg string) { /* 实现 */ }

// 2. 实现 ConfigProvider 接口
type MyConfig struct{}

func (c *MyConfig) IsRedisEnabled() bool { return true }
func (c *MyConfig) GetRedisAddrs() []string { return []string{"localhost:6379"} }
func (c *MyConfig) GetRedisPassword() string { return "" }
func (c *MyConfig) GetRedisUsername() string { return "" }
func (c *MyConfig) GetConcurrency() int { return 10 }
func (c *MyConfig) GetQueuePrefix() string { return "" }
func (c *MyConfig) IsPollingEnabled() bool { return true }

// 3. 设置依赖
asynq.SetLogger(&MyLogger{})
asynq.SetConfigProvider(&MyConfig{})
```

### 4. 初始化 Manager

```go
// 从配置提供者初始化
manager, err := asynq.InitManagerFromConfig(configProvider)
if err != nil {
    log.Fatal(err)
}

// 注册任务处理器
manager.RegisterHandler("email:send", handleEmail)
manager.RegisterHandler("report:generate", handleReport)

// 启动 Worker
ctx := context.Background()
if err := manager.Start(ctx); err != nil {
    log.Fatal(err)
}

// 优雅关闭
defer manager.Stop()
```

### 5. 创建 Worker

**推荐模式：结构体 + ProcessTask 方法**

```go
type EmailWorker struct {
    // 依赖注入
    emailService *EmailService
    tracer       *Tracer
}

func NewEmailWorker(emailService *EmailService) *EmailWorker {
    return &EmailWorker{
        emailService: emailService,
        tracer:       GetTracer(),
    }
}

func (w *EmailWorker) ProcessTask(ctx context.Context, t *asynq.Task) (err error) {
    startTime := time.Now()

    // Panic 恢复（生产环境必须）
    defer func() {
        if r := recover(); r != nil {
            stack := debug.Stack()
            log.Printf("[PANIC] %v\nStack:\n%s", r, string(stack))
            err = fmt.Errorf("panic recovered: %v", r)
        }
    }()

    // 解析 Payload
    var payload EmailPayload
    if err := json.Unmarshal(t.Payload(), &payload); err != nil {
        return fmt.Errorf("parse payload failed: %w", err)
    }

    // 记录追踪（可选）
    if w.tracer != nil {
        w.tracer.RecordEvent(ctx, payload.TraceID, "email_send_start", nil)
    }

    // 业务逻辑
    if err := w.emailService.Send(payload.To, payload.Subject, payload.Body); err != nil {
        log.Printf("[EmailWorker] 发送失败: %v", err)
        return err // 返回错误触发重试
    }

    log.Printf("[EmailWorker] 完成: to=%s, 耗时=%v", payload.To, time.Since(startTime))
    return nil
}
```

### 6. 入队任务

**方式 1：使用 TaskBuilder（推荐）**

```go
import "github.com/everyday-items/toolkit/infra/queue/asynq"

// 简单任务
task := asynq.NewTask("email:send").
    Payload(map[string]string{
        "to": "user@example.com",
        "subject": "Welcome",
    }).
    Queue(asynq.QueueHigh).
    MaxRetry(3).
    Enqueue(ctx)

// 延迟任务
task := asynq.NewTask("report:generate").
    Payload(reportData).
    ProcessIn(5 * time.Minute).
    Queue(asynq.QueueDefault).
    Enqueue(ctx)

// 定时任务
task := asynq.NewTask("cleanup").
    Payload(nil).
    ProcessAt(time.Now().Add(24 * time.Hour)).
    Queue(asynq.QueueLow).
    Enqueue(ctx)

// 唯一任务（去重）
task := asynq.NewTask("user:sync").
    Payload(userData).
    TaskID(fmt.Sprintf("sync:%d", userID)).
    Unique(10 * time.Minute).
    Enqueue(ctx)
```

**方式 2：使用原生 API**

```go
import asq "github.com/hibiken/asynq"

payload, _ := json.Marshal(data)
task := asq.NewTask("task:type", payload)

manager := asynq.GetManager()
info, err := manager.Enqueue(ctx, task,
    asq.Queue(asynq.QueueHigh),
    asq.MaxRetry(3),
    asq.Timeout(5 * time.Minute),
)
```

## 高级特性

### 1. 队列优先级

```go
// 预定义队列（优先级从高到低）
asynq.QueueCritical   // 最高优先级
asynq.QueueHigh       // 高优先级
asynq.QueueDefault    // 默认优先级
asynq.QueueScheduled  // 调度队列
asynq.QueueLow        // 低优先级
asynq.QueueDeadLetter // 死信队列
```

### 2. 错误处理和重试

```go
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) error {
    // 返回错误会触发重试
    if err := doSomething(); err != nil {
        return err // Asynq 会根据 MaxRetry 重试
    }

    // 返回 nil 表示成功
    return nil
}
```

**重试策略：**
- 指数退避：1s, 2s, 4s, 8s, 16s...
- 最大重试次数由 `MaxRetry()` 设定
- 超过重试次数后进入死信队列

### 3. 超时控制

```go
task := asynq.NewTask("long:task").
    Payload(data).
    Timeout(10 * time.Minute). // 10分钟超时
    Deadline(time.Now().Add(1 * time.Hour)). // 1小时截止
    Enqueue(ctx)
```

### 4. 任务去重

```go
// 使用 TaskID 去重
task := asynq.NewTask("user:sync").
    Payload(userData).
    TaskID(fmt.Sprintf("sync:%d", userID)). // 相同 ID 会冲突
    Enqueue(ctx)

// 使用 Unique 时间窗口去重
task := asynq.NewTask("notification").
    Payload(data).
    Unique(5 * time.Minute). // 5分钟内不重复入队
    Enqueue(ctx)
```

### 5. 定时任务（Cron）

```go
// 注册定时任务
manager.RegisterSchedule(
    "@every 1h",                    // Cron 表达式
    asq.NewTask("cleanup", nil),    // 任务
    asq.Queue(asynq.QueueLow),      // 选项
)

// Cron 表达式示例
"@every 1h"       // 每小时
"0 */5 * * *"     // 每5分钟
"0 0 * * *"       // 每天0点
"0 9 * * 1"       // 每周一9点
```

### 6. 监控和追踪

```go
// 获取统计信息
stats := asynq.GetStats()
fmt.Printf("Running: %v, Handlers: %d\n", stats["started"], stats["handlers"])

// 使用 Inspector 查询
inspector := manager.GetInspector()
defer inspector.Close()

// 查询队列信息
queueInfo, err := inspector.GetQueueInfo("default")
fmt.Printf("Pending: %d, Active: %d\n", queueInfo.Pending, queueInfo.Active)
```

## 生产环境最佳实践

### 1. Panic 恢复（必须）

```go
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) (err error) {
    defer func() {
        if r := recover(); r != nil {
            stack := debug.Stack()
            log.Printf("[PANIC] %v\nStack:\n%s", r, string(stack))
            err = fmt.Errorf("panic recovered: %v", r)
        }
    }()

    // 业务逻辑
}
```

### 2. 幂等性设计

```go
// 使用唯一标识防止重复处理
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) error {
    taskID := payload.TaskID

    // 检查是否已处理
    if isProcessed(taskID) {
        log.Printf("[Worker] Task already processed: %s", taskID)
        return nil // 返回 nil 避免重试
    }

    // 标记为处理中
    markAsProcessing(taskID)

    // 处理业务逻辑
    // ...

    // 标记为已完成
    markAsCompleted(taskID)
    return nil
}
```

### 3. 超时控制

```go
// Context 超时
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

// 检查 Context
select {
case <-ctx.Done():
    return fmt.Errorf("task cancelled: %w", ctx.Err())
default:
    // 继续处理
}
```

### 4. 资源清理

```go
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) error {
    // 获取资源
    conn := getConnection()
    defer conn.Close() // 确保释放

    // 业务逻辑
    // ...

    return nil
}
```

### 5. 日志记录

```go
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) error {
    startTime := time.Now()

    log.Printf("[Worker] 开始: task_id=%s", payload.TaskID)

    // 业务逻辑
    err := w.process(payload)

    duration := time.Since(startTime)
    if err != nil {
        log.Printf("[Worker] 失败: task_id=%s, err=%v, 耗时=%v",
            payload.TaskID, err, duration)
    } else {
        log.Printf("[Worker] 成功: task_id=%s, 耗时=%v",
            payload.TaskID, duration)
    }

    return err
}
```

## 完整示例

参考项目中的示例代码：
- `examples/infra/asynq_example.go` - 基础示例
- `examples/infra/asynq_complete_example.go` - 生产级完整示例

## 故障排查

### 1. 任务没有被处理

**检查清单：**
- [ ] Redis 是否可访问
- [ ] Manager 是否已启动（`manager.Start(ctx)`）
- [ ] 任务类型是否已注册（`manager.RegisterHandler`）
- [ ] 队列配置是否正确
- [ ] Worker 并发数是否足够

### 2. 任务一直重试

**原因：**
- Worker 返回错误会触发重试
- 检查业务逻辑是否抛出错误
- 检查 MaxRetry 配置

**解决：**
```go
// 对于不可重试的错误，返回 nil
if isNonRetryableError(err) {
    log.Printf("Non-retryable error: %v", err)
    return nil // 不重试
}
return err // 可重试错误
```

### 3. 任务进入死信队列

**查看死信队列：**
```go
inspector := manager.GetInspector()
tasks, err := inspector.ListDeadletterTasks("default")
for _, task := range tasks {
    log.Printf("Dead task: %s, error: %s", task.ID, task.LastErr)
}
```

**重新入队：**
```go
// 从死信队列重新入队
err := inspector.DeleteTask("default", taskID)
```

## 更多资源

- [Asynq 官方文档](https://github.com/hibiken/asynq)
- [项目示例代码](../../examples/infra/)
- [接口定义](./interfaces.go)
