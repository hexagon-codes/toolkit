[中文](USAGE.md) | English

# Asynq Usage Guide

## Quick Start

### 1. Installation

```bash
go get github.com/everyday-items/toolkit/infra/queue/asynq
```

### 2. Core Concepts

**Asynq** is a Redis-based distributed task queue. This package provides a production-grade wrapper.

**Core Components:**
- **Manager**: Manages task enqueue and worker scheduling
- **Worker**: Task processor implementing specific business logic
- **Queue**: Queue with different task priority levels
- **Payload**: Task payload containing the data required for a task

### 3. Dependency Injection Configuration

Before use, configure dependency injection:

```go
import "github.com/everyday-items/toolkit/infra/queue/asynq"

// 1. Implement the Logger interface
type MyLogger struct{}

func (l *MyLogger) Log(msg string)            { /* implement */ }
func (l *MyLogger) LogSkip(skip int, msg string) { /* implement */ }
func (l *MyLogger) Error(msg string)          { /* implement */ }
func (l *MyLogger) ErrorSkip(skip int, msg string) { /* implement */ }

// 2. Implement the ConfigProvider interface
type MyConfig struct{}

func (c *MyConfig) IsRedisEnabled() bool { return true }
func (c *MyConfig) GetRedisAddrs() []string { return []string{"localhost:6379"} }
func (c *MyConfig) GetRedisPassword() string { return "" }
func (c *MyConfig) GetRedisUsername() string { return "" }
func (c *MyConfig) GetConcurrency() int { return 10 }
func (c *MyConfig) GetQueuePrefix() string { return "" }
func (c *MyConfig) IsPollingEnabled() bool { return true }

// 3. Set dependencies
asynq.SetLogger(&MyLogger{})
asynq.SetConfigProvider(&MyConfig{})
```

### 4. Initialize Manager

```go
// Initialize from config provider
manager, err := asynq.InitManagerFromConfig(configProvider)
if err != nil {
    log.Fatal(err)
}

// Register task handlers
manager.RegisterHandler("email:send", handleEmail)
manager.RegisterHandler("report:generate", handleReport)

// Start Worker
ctx := context.Background()
if err := manager.Start(ctx); err != nil {
    log.Fatal(err)
}

// Graceful shutdown
defer manager.Stop()
```

### 5. Create Worker

**Recommended pattern: struct + ProcessTask method**

```go
type EmailWorker struct {
    // dependency injection
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

    // Panic recovery (required in production)
    defer func() {
        if r := recover(); r != nil {
            stack := debug.Stack()
            log.Printf("[PANIC] %v\nStack:\n%s", r, string(stack))
            err = fmt.Errorf("panic recovered: %v", r)
        }
    }()

    // Parse Payload
    var payload EmailPayload
    if err := json.Unmarshal(t.Payload(), &payload); err != nil {
        return fmt.Errorf("parse payload failed: %w", err)
    }

    // Record trace (optional)
    if w.tracer != nil {
        w.tracer.RecordEvent(ctx, payload.TraceID, "email_send_start", nil)
    }

    // Business logic
    if err := w.emailService.Send(payload.To, payload.Subject, payload.Body); err != nil {
        log.Printf("[EmailWorker] Send failed: %v", err)
        return err // return error to trigger retry
    }

    log.Printf("[EmailWorker] Done: to=%s, duration=%v", payload.To, time.Since(startTime))
    return nil
}
```

### 6. Enqueue Tasks

**Option 1: Use TaskBuilder (recommended)**

```go
import "github.com/everyday-items/toolkit/infra/queue/asynq"

// Simple task
task := asynq.NewTask("email:send").
    Payload(map[string]string{
        "to": "user@example.com",
        "subject": "Welcome",
    }).
    Queue(asynq.QueueHigh).
    MaxRetry(3).
    Enqueue(ctx)

// Delayed task
task := asynq.NewTask("report:generate").
    Payload(reportData).
    ProcessIn(5 * time.Minute).
    Queue(asynq.QueueDefault).
    Enqueue(ctx)

// Scheduled task
task := asynq.NewTask("cleanup").
    Payload(nil).
    ProcessAt(time.Now().Add(24 * time.Hour)).
    Queue(asynq.QueueLow).
    Enqueue(ctx)

// Unique task (deduplication)
task := asynq.NewTask("user:sync").
    Payload(userData).
    TaskID(fmt.Sprintf("sync:%d", userID)).
    Unique(10 * time.Minute).
    Enqueue(ctx)
```

**Option 2: Use Native API**

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

## Advanced Features

### 1. Queue Priority

```go
// Predefined queues (priority from high to low)
asynq.QueueCritical   // highest priority
asynq.QueueHigh       // high priority
asynq.QueueDefault    // default priority
asynq.QueueScheduled  // scheduled queue
asynq.QueueLow        // low priority
asynq.QueueDeadLetter // dead letter queue
```

### 2. Error Handling and Retry

```go
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) error {
    // Returning an error triggers retry
    if err := doSomething(); err != nil {
        return err // Asynq will retry based on MaxRetry
    }

    // Return nil on success
    return nil
}
```

**Retry strategy:**
- Exponential backoff: 1s, 2s, 4s, 8s, 16s...
- Maximum retry count set by `MaxRetry()`
- Tasks exceeding retry count go to dead letter queue

### 3. Timeout Control

```go
task := asynq.NewTask("long:task").
    Payload(data).
    Timeout(10 * time.Minute). // 10-minute timeout
    Deadline(time.Now().Add(1 * time.Hour)). // 1-hour deadline
    Enqueue(ctx)
```

### 4. Task Deduplication

```go
// Deduplication using TaskID
task := asynq.NewTask("user:sync").
    Payload(userData).
    TaskID(fmt.Sprintf("sync:%d", userID)). // same ID causes conflict
    Enqueue(ctx)

// Deduplication using Unique time window
task := asynq.NewTask("notification").
    Payload(data).
    Unique(5 * time.Minute). // no duplicate enqueue within 5 minutes
    Enqueue(ctx)
```

### 5. Scheduled Tasks (Cron)

```go
// Register scheduled task
manager.RegisterSchedule(
    "@every 1h",                    // Cron expression
    asq.NewTask("cleanup", nil),    // task
    asq.Queue(asynq.QueueLow),      // options
)

// Cron expression examples
"@every 1h"       // every hour
"0 */5 * * *"     // every 5 minutes
"0 0 * * *"       // daily at midnight
"0 9 * * 1"       // every Monday at 9am
```

### 6. Monitoring and Tracing

```go
// Get statistics
stats := asynq.GetStats()
fmt.Printf("Running: %v, Handlers: %d\n", stats["started"], stats["handlers"])

// Use Inspector to query
inspector := manager.GetInspector()
defer inspector.Close()

// Query queue info
queueInfo, err := inspector.GetQueueInfo("default")
fmt.Printf("Pending: %d, Active: %d\n", queueInfo.Pending, queueInfo.Active)
```

## Production Best Practices

### 1. Panic Recovery (Required)

```go
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) (err error) {
    defer func() {
        if r := recover(); r != nil {
            stack := debug.Stack()
            log.Printf("[PANIC] %v\nStack:\n%s", r, string(stack))
            err = fmt.Errorf("panic recovered: %v", r)
        }
    }()

    // Business logic
}
```

### 2. Idempotency Design

```go
// Use unique identifier to prevent duplicate processing
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) error {
    taskID := payload.TaskID

    // Check if already processed
    if isProcessed(taskID) {
        log.Printf("[Worker] Task already processed: %s", taskID)
        return nil // return nil to avoid retry
    }

    // Mark as processing
    markAsProcessing(taskID)

    // Execute business logic
    // ...

    // Mark as completed
    markAsCompleted(taskID)
    return nil
}
```

### 3. Timeout Control

```go
// Context timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

// Check Context
select {
case <-ctx.Done():
    return fmt.Errorf("task cancelled: %w", ctx.Err())
default:
    // continue processing
}
```

### 4. Resource Cleanup

```go
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) error {
    // Acquire resource
    conn := getConnection()
    defer conn.Close() // ensure release

    // Business logic
    // ...

    return nil
}
```

### 5. Logging

```go
func (w *Worker) ProcessTask(ctx context.Context, t *asynq.Task) error {
    startTime := time.Now()

    log.Printf("[Worker] Start: task_id=%s", payload.TaskID)

    // Business logic
    err := w.process(payload)

    duration := time.Since(startTime)
    if err != nil {
        log.Printf("[Worker] Failed: task_id=%s, err=%v, duration=%v",
            payload.TaskID, err, duration)
    } else {
        log.Printf("[Worker] Success: task_id=%s, duration=%v",
            payload.TaskID, duration)
    }

    return err
}
```

## Complete Example

Refer to the example code in the project:
- `examples/infra/asynq_example.go` - basic example
- `examples/infra/asynq_complete_example.go` - production-grade complete example

## Troubleshooting

### 1. Tasks Not Being Processed

**Checklist:**
- [ ] Is Redis accessible?
- [ ] Is Manager started? (`manager.Start(ctx)`)
- [ ] Is the task type registered? (`manager.RegisterHandler`)
- [ ] Is queue configuration correct?
- [ ] Is Worker concurrency sufficient?

### 2. Tasks Keep Retrying

**Causes:**
- Workers returning errors trigger retries
- Check if business logic is throwing errors
- Check MaxRetry configuration

**Solution:**
```go
// For non-retryable errors, return nil
if isNonRetryableError(err) {
    log.Printf("Non-retryable error: %v", err)
    return nil // no retry
}
return err // retryable error
```

### 3. Tasks in Dead Letter Queue

**View dead letter queue:**
```go
inspector := manager.GetInspector()
tasks, err := inspector.ListDeadletterTasks("default")
for _, task := range tasks {
    log.Printf("Dead task: %s, error: %s", task.ID, task.LastErr)
}
```

**Re-enqueue:**
```go
// Re-enqueue from dead letter queue
err := inspector.DeleteTask("default", taskID)
```

## More Resources

- [Asynq Official Documentation](https://github.com/hibiken/asynq)
- [Project Example Code](../../examples/infra/)
- [Interface Definitions](./interfaces.go)
