package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/everyday-items/toolkit/infra/queue/asynq"
	asq "github.com/hibiken/asynq"
)

// =========================================
// 完整的 Asynq 使用示例
// 参考生产环境最佳实践
// =========================================

func main() {
	fmt.Println("╔═══════════════════════════════════════════════╗")
	fmt.Println("║   Asynq 完整示例 - 生产级实践                  ║")
	fmt.Println("╚═══════════════════════════════════════════════╝")

	// 1. 配置依赖注入
	setupDependencies()

	// 2. 初始化管理器
	manager, err := initManager()
	if err != nil {
		log.Fatalf("❌ 初始化失败: %v", err)
	}

	// 3. 注册 Workers
	registerWorkers(manager)

	// 4. 启动 Worker
	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		log.Fatalf("❌ 启动失败: %v", err)
	}

	fmt.Println("✅ Worker 已启动")

	// 5. 模拟业务场景：入队各种任务
	demonstrateTaskQueuing(ctx, manager)

	// 6. 等待任务处理
	fmt.Println("\n⏳ 等待任务处理（15秒）...")
	time.Sleep(15 * time.Second)

	// 7. 优雅关闭
	fmt.Println("\n🛑 优雅关闭...")
	manager.Stop()

	fmt.Println("\n✅ 示例完成!")
}

// =========================================
// 步骤 1：配置依赖注入
// =========================================

func setupDependencies() {
	fmt.Println("📝 配置依赖注入...")

	// 日志
	logger := &ProductionLogger{}
	asynq.SetLogger(logger)

	// 配置提供者
	config := &ProductionConfig{
		redisAddrs:    []string{"localhost:6379"},
		redisPassword: "",
		concurrency:   5,
		redisEnabled:  true,
	}
	asynq.SetConfigProvider(config)

	fmt.Println("   ✓ Logger 已设置")
	fmt.Println("   ✓ ConfigProvider 已设置")
	fmt.Println()
}

// =========================================
// 步骤 2：初始化管理器
// =========================================

func initManager() (*asynq.Manager, error) {
	fmt.Println("🚀 初始化 Asynq Manager...")

	configProvider := asynq.GetConfigProvider()
	manager, err := asynq.InitManagerFromConfig(configProvider)
	if err != nil {
		return nil, err
	}

	fmt.Println("   ✓ Manager 初始化成功")
	fmt.Println()
	return manager, nil
}

// =========================================
// 步骤 3：注册 Workers
// =========================================

func registerWorkers(manager *asynq.Manager) {
	fmt.Println("📋 注册 Workers...")

	// 注册邮件 Worker
	emailWorker := NewEmailWorker()
	manager.RegisterHandler("email:send", emailWorker.ProcessTask)
	fmt.Println("   ✓ EmailWorker 已注册")

	// 注册报告 Worker
	reportWorker := NewReportWorker()
	manager.RegisterHandler("report:generate", reportWorker.ProcessTask)
	fmt.Println("   ✓ ReportWorker 已注册")

	// 注册数据同步 Worker
	syncWorker := NewDataSyncWorker()
	manager.RegisterHandler("data:sync", syncWorker.ProcessTask)
	fmt.Println("   ✓ DataSyncWorker 已注册")

	fmt.Println()
}

// =========================================
// 步骤 5：演示任务入队
// =========================================

func demonstrateTaskQueuing(ctx context.Context, manager *asynq.Manager) {
	fmt.Println("📤 入队任务...")

	// 场景 1：立即执行的高优先级任务
	emailPayload := EmailPayload{
		To:      "user@example.com",
		Subject: "Welcome!",
		Body:    "Thanks for signing up",
	}
	enqueueTask(ctx, manager, "email:send", emailPayload, asynq.QueueHigh, 0, 3)

	// 场景 2：延迟执行的任务
	reportPayload := ReportPayload{
		Type:   "monthly",
		Month:  "2024-01",
		UserID: 123,
	}
	enqueueTask(ctx, manager, "report:generate", reportPayload, asynq.QueueDefault, 5*time.Second, 2)

	// 场景 3：计划任务（更长延迟）
	syncPayload := DataSyncPayload{
		Source: "database",
		Target: "cache",
		Tables: []string{"users", "orders"},
	}
	enqueueTask(ctx, manager, "data:sync", syncPayload, asynq.QueueLow, 10*time.Second, 1)
}

func enqueueTask(ctx context.Context, manager *asynq.Manager, taskType string, payload interface{}, queue string, delay time.Duration, maxRetry int) {
	data, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("   ❌ 序列化失败: %v\n", err)
		return
	}

	opts := []asq.Option{
		asq.Queue(queue),
		asq.MaxRetry(maxRetry),
	}

	if delay > 0 {
		opts = append(opts, asq.ProcessIn(delay))
	}

	task := asq.NewTask(taskType, data, opts...)
	info, err := manager.Enqueue(ctx, task, opts...)
	if err != nil {
		fmt.Printf("   ❌ 入队失败: %v\n", err)
		return
	}

	delayMsg := ""
	if delay > 0 {
		delayMsg = fmt.Sprintf(", %s后处理", delay)
	}

	fmt.Printf("   ✅ [%s] %s | ID=%s, Retry=%d%s\n",
		queue, taskType, info.ID[:8], maxRetry, delayMsg)
}

// =========================================
// Worker 实现 - EmailWorker
// =========================================

type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type EmailWorker struct{}

func NewEmailWorker() *EmailWorker {
	return &EmailWorker{}
}

func (w *EmailWorker) ProcessTask(ctx context.Context, t *asq.Task) (err error) {
	startTime := time.Now()

	// Panic 恢复
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Printf("[PANIC] EmailWorker: %v\nStack:\n%s", r, string(stack))
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()

	// 解析 Payload
	var payload EmailPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		log.Printf("[EmailWorker] 解析失败: %v", err)
		return fmt.Errorf("parse payload failed: %w", err)
	}

	log.Printf("📧 [EmailWorker] 开始处理: to=%s, subject=%s", payload.To, payload.Subject)

	// 业务逻辑：发送邮件
	if err := w.sendEmail(&payload); err != nil {
		log.Printf("[EmailWorker] 发送失败: %v", err)
		return err // 返回错误会触发重试
	}

	duration := time.Since(startTime)
	log.Printf("✅ [EmailWorker] 完成: to=%s, 耗时=%v", payload.To, duration)
	return nil
}

func (w *EmailWorker) sendEmail(payload *EmailPayload) error {
	// 模拟邮件发送
	time.Sleep(1 * time.Second)

	// 模拟失败（10% 概率）
	// if rand.Intn(10) == 0 {
	// 	return fmt.Errorf("SMTP connection failed")
	// }

	return nil
}

// =========================================
// Worker 实现 - ReportWorker
// =========================================

type ReportPayload struct {
	Type   string `json:"type"`
	Month  string `json:"month"`
	UserID int    `json:"user_id"`
}

type ReportWorker struct{}

func NewReportWorker() *ReportWorker {
	return &ReportWorker{}
}

func (w *ReportWorker) ProcessTask(ctx context.Context, t *asq.Task) error {
	var payload ReportPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("parse payload failed: %w", err)
	}

	log.Printf("📊 [ReportWorker] 开始处理: type=%s, month=%s, user=%d",
		payload.Type, payload.Month, payload.UserID)

	// 业务逻辑：生成报告
	if err := w.generateReport(&payload); err != nil {
		return err
	}

	log.Printf("✅ [ReportWorker] 完成: type=%s", payload.Type)
	return nil
}

func (w *ReportWorker) generateReport(payload *ReportPayload) error {
	// 模拟报告生成
	time.Sleep(2 * time.Second)
	return nil
}

// =========================================
// Worker 实现 - DataSyncWorker
// =========================================

type DataSyncPayload struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Tables []string `json:"tables"`
}

type DataSyncWorker struct{}

func NewDataSyncWorker() *DataSyncWorker {
	return &DataSyncWorker{}
}

func (w *DataSyncWorker) ProcessTask(ctx context.Context, t *asq.Task) error {
	var payload DataSyncPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("parse payload failed: %w", err)
	}

	log.Printf("🔄 [DataSyncWorker] 开始处理: %s -> %s, tables=%v",
		payload.Source, payload.Target, payload.Tables)

	// 业务逻辑：数据同步
	for _, table := range payload.Tables {
		if err := w.syncTable(table, payload.Source, payload.Target); err != nil {
			log.Printf("[DataSyncWorker] 同步失败: table=%s, err=%v", table, err)
			return err
		}
		log.Printf("   ✓ 表 %s 同步完成", table)
	}

	log.Printf("✅ [DataSyncWorker] 完成: %d 个表同步成功", len(payload.Tables))
	return nil
}

func (w *DataSyncWorker) syncTable(table, source, target string) error {
	// 模拟数据同步
	time.Sleep(500 * time.Millisecond)
	return nil
}

// =========================================
// 生产级实现
// =========================================

type ProductionLogger struct{}

func (l *ProductionLogger) Log(msg string) {
	log.Printf("[INFO] %s", msg)
}

func (l *ProductionLogger) LogSkip(skip int, msg string) {
	log.Printf("[INFO] %s", msg)
}

func (l *ProductionLogger) Error(msg string) {
	log.Printf("[ERROR] %s", msg)
}

func (l *ProductionLogger) ErrorSkip(skip int, msg string) {
	log.Printf("[ERROR] %s", msg)
}

type ProductionConfig struct {
	redisAddrs    []string
	redisPassword string
	concurrency   int
	redisEnabled  bool
}

func (c *ProductionConfig) IsRedisEnabled() bool     { return c.redisEnabled }
func (c *ProductionConfig) GetRedisAddrs() []string  { return c.redisAddrs }
func (c *ProductionConfig) GetRedisPassword() string { return c.redisPassword }
func (c *ProductionConfig) GetRedisUsername() string { return "" }
func (c *ProductionConfig) GetConcurrency() int      { return c.concurrency }
func (c *ProductionConfig) GetQueuePrefix() string   { return "" }
func (c *ProductionConfig) IsPollingEnabled() bool   { return true }
