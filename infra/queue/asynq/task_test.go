package asynq

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

type TestPayload struct {
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
}

func TestTaskBuilder_Build(t *testing.T) {
	payload := TestPayload{
		UserID: 123,
		Email:  "test@example.com",
	}

	task, err := NewTask("email:send").
		Payload(payload).
		Queue(QueueHigh).
		MaxRetry(3).
		Timeout(30 * time.Second).
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if task == nil {
		t.Fatal("task is nil")
	}

	if task.Type() != "email:send" {
		t.Errorf("expected type 'email:send', got '%s'", task.Type())
	}

	// 验证 payload
	var decoded TestPayload
	if err := json.Unmarshal(task.Payload(), &decoded); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if decoded.UserID != 123 || decoded.Email != "test@example.com" {
		t.Errorf("payload mismatch: %+v", decoded)
	}
}

func TestTaskBuilder_Build_NilPayload(t *testing.T) {
	task, err := NewTask("test:task").Build()

	if err != nil {
		t.Fatalf("Build with nil payload failed: %v", err)
	}

	if task == nil {
		t.Fatal("task is nil")
	}

	if len(task.Payload()) != 0 {
		t.Errorf("expected empty payload, got: %v", task.Payload())
	}
}

func TestTaskBuilder_Build_InvalidPayload(t *testing.T) {
	// 使用无法序列化的类型
	invalidPayload := make(chan int)

	_, err := NewTask("test:task").
		Payload(invalidPayload).
		Build()

	if err == nil {
		t.Error("expected error for invalid payload, got nil")
	}
}

func TestTaskBuilder_Chaining(t *testing.T) {
	now := time.Now()
	deadline := now.Add(time.Hour)
	processAt := now.Add(10 * time.Minute)

	builder := NewTask("test:task").
		Payload(TestPayload{UserID: 1, Email: "test@example.com"}).
		Queue(QueueLow).
		MaxRetry(5).
		Timeout(60 * time.Second).
		Deadline(deadline).
		ProcessAt(processAt).
		TaskID("unique-id-123").
		Unique(5 * time.Minute).
		Retention(24 * time.Hour)

	task, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if task == nil {
		t.Fatal("task is nil")
	}
}

func TestTaskBuilder_ProcessIn(t *testing.T) {
	builder := NewTask("delayed:task").
		Payload(TestPayload{UserID: 1}).
		ProcessIn(5 * time.Minute)

	task, err := builder.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if task == nil {
		t.Fatal("task is nil")
	}
}

func TestParsePayload(t *testing.T) {
	original := TestPayload{
		UserID: 456,
		Email:  "parse@example.com",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	task := asynq.NewTask("test:task", data)

	parsed, err := ParsePayload[TestPayload](task)
	if err != nil {
		t.Fatalf("ParsePayload failed: %v", err)
	}

	if parsed.UserID != 456 || parsed.Email != "parse@example.com" {
		t.Errorf("parsed payload mismatch: %+v", parsed)
	}
}

func TestParsePayload_InvalidJSON(t *testing.T) {
	task := asynq.NewTask("test:task", []byte("invalid json"))

	_, err := ParsePayload[TestPayload](task)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestParsePayload_EmptyPayload(t *testing.T) {
	task := asynq.NewTask("test:task", []byte{})

	_, err := ParsePayload[TestPayload](task)
	// 空 payload 应该返回错误
	if err == nil {
		t.Error("expected error for empty payload")
	}
}

func TestTaskBuilder_Enqueue_NoManager(t *testing.T) {
	// 确保全局管理器未初始化
	globalManager = nil

	builder := NewTask("test:task").Payload(TestPayload{UserID: 1})

	_, err := builder.Enqueue(context.Background())
	if err != ErrManagerNotInitialized {
		t.Errorf("expected ErrManagerNotInitialized, got: %v", err)
	}
}

func TestEnqueueTask_NoManager(t *testing.T) {
	// 确保全局管理器未初始化
	globalManager = nil

	_, err := EnqueueTask(context.Background(), "test:task", TestPayload{UserID: 1})
	if err != ErrManagerNotInitialized {
		t.Errorf("expected ErrManagerNotInitialized, got: %v", err)
	}
}

func TestEnqueueTaskDelayed_NoManager(t *testing.T) {
	globalManager = nil

	_, err := EnqueueTaskDelayed(context.Background(), "test:task", TestPayload{UserID: 1}, 5*time.Minute)
	if err != ErrManagerNotInitialized {
		t.Errorf("expected ErrManagerNotInitialized, got: %v", err)
	}
}

func TestEnqueueTaskAt_NoManager(t *testing.T) {
	globalManager = nil

	_, err := EnqueueTaskAt(context.Background(), "test:task", TestPayload{UserID: 1}, time.Now().Add(time.Hour))
	if err != ErrManagerNotInitialized {
		t.Errorf("expected ErrManagerNotInitialized, got: %v", err)
	}
}

func TestEnqueueTaskUnique_NoManager(t *testing.T) {
	globalManager = nil

	_, err := EnqueueTaskUnique(context.Background(), "test:task", TestPayload{UserID: 1}, "unique-id")
	if err != ErrManagerNotInitialized {
		t.Errorf("expected ErrManagerNotInitialized, got: %v", err)
	}
}

func TestErrors(t *testing.T) {
	// 测试错误定义
	if ErrManagerNotInitialized == nil {
		t.Error("ErrManagerNotInitialized should not be nil")
	}
	if ErrTaskNotFound == nil {
		t.Error("ErrTaskNotFound should not be nil")
	}
	if ErrInvalidPayload == nil {
		t.Error("ErrInvalidPayload should not be nil")
	}
	if ErrQueueNotFound == nil {
		t.Error("ErrQueueNotFound should not be nil")
	}
	if ErrHandlerNotRegistered == nil {
		t.Error("ErrHandlerNotRegistered should not be nil")
	}
}
