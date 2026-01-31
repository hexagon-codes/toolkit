package meter

import (
	"errors"
	"testing"
	"time"
)

func TestMeter_Record(t *testing.T) {
	m := New()

	m.Record("gpt-4", 100, 50)
	m.Record("gpt-4", 200, 100)

	stats := m.Stats()

	if stats.TotalRequests != 2 {
		t.Errorf("expected 2 requests, got %d", stats.TotalRequests)
	}
	if stats.InputTokens != 300 {
		t.Errorf("expected 300 input tokens, got %d", stats.InputTokens)
	}
	if stats.OutputTokens != 150 {
		t.Errorf("expected 150 output tokens, got %d", stats.OutputTokens)
	}
	if stats.TotalTokens != 450 {
		t.Errorf("expected 450 total tokens, got %d", stats.TotalTokens)
	}
}

func TestMeter_RecordWithLatency(t *testing.T) {
	m := New()

	m.RecordWithLatency("gpt-4", 100, 50, 100*time.Millisecond)
	m.RecordWithLatency("gpt-4", 100, 50, 200*time.Millisecond)
	m.RecordWithLatency("gpt-4", 100, 50, 300*time.Millisecond)

	stats := m.Stats()

	if stats.MinLatency != 100*time.Millisecond {
		t.Errorf("expected min latency 100ms, got %v", stats.MinLatency)
	}
	if stats.MaxLatency != 300*time.Millisecond {
		t.Errorf("expected max latency 300ms, got %v", stats.MaxLatency)
	}
	if stats.AvgLatency != 200*time.Millisecond {
		t.Errorf("expected avg latency 200ms, got %v", stats.AvgLatency)
	}
}

func TestMeter_RecordError(t *testing.T) {
	m := New()

	m.Record("gpt-4", 100, 50)
	m.RecordError("gpt-4", 100, errors.New("rate limit"))

	stats := m.Stats()

	if stats.TotalRequests != 2 {
		t.Errorf("expected 2 requests, got %d", stats.TotalRequests)
	}
	if stats.SuccessRequests != 1 {
		t.Errorf("expected 1 success, got %d", stats.SuccessRequests)
	}
	if stats.FailedRequests != 1 {
		t.Errorf("expected 1 failure, got %d", stats.FailedRequests)
	}
}

func TestMeter_EstimatedCost(t *testing.T) {
	m := New()

	// GPT-4: $30/1M input, $60/1M output
	m.Record("gpt-4", 1000, 500)

	stats := m.Stats()

	// 1000 input = $0.03, 500 output = $0.03
	expectedCost := 0.03 + 0.03
	if stats.EstimatedCost != expectedCost {
		t.Errorf("expected cost $%.4f, got $%.4f", expectedCost, stats.EstimatedCost)
	}
}

func TestMeter_StatsByModel(t *testing.T) {
	m := New()

	m.Record("gpt-4", 100, 50)
	m.Record("gpt-4", 200, 100)
	m.Record("gpt-3.5-turbo", 300, 150)

	gpt4Stats := m.StatsByModel("gpt-4")

	if gpt4Stats.TotalRequests != 2 {
		t.Errorf("expected 2 gpt-4 requests, got %d", gpt4Stats.TotalRequests)
	}
	if gpt4Stats.TotalTokens != 450 {
		t.Errorf("expected 450 gpt-4 tokens, got %d", gpt4Stats.TotalTokens)
	}

	gpt35Stats := m.StatsByModel("gpt-3.5-turbo")
	if gpt35Stats.TotalRequests != 1 {
		t.Errorf("expected 1 gpt-3.5 request, got %d", gpt35Stats.TotalRequests)
	}
}

func TestMeter_AllModelStats(t *testing.T) {
	m := New()

	m.Record("gpt-4", 100, 50)
	m.Record("gpt-4", 100, 50)
	m.Record("gpt-3.5-turbo", 100, 50)

	allStats := m.AllModelStats()

	if len(allStats) != 2 {
		t.Fatalf("expected 2 models, got %d", len(allStats))
	}

	// 应该按请求数排序
	if allStats[0].Model != "gpt-4" {
		t.Errorf("expected gpt-4 first, got %s", allStats[0].Model)
	}
}

func TestMeter_Records(t *testing.T) {
	m := New()

	m.Record("gpt-4", 100, 50)
	m.Record("gpt-4", 200, 100)

	records := m.Records()

	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}

func TestMeter_RecordsSince(t *testing.T) {
	m := New()

	m.Record("gpt-4", 100, 50)
	time.Sleep(10 * time.Millisecond)
	checkpoint := time.Now()
	time.Sleep(10 * time.Millisecond)
	m.Record("gpt-4", 200, 100)

	records := m.RecordsSince(checkpoint)

	if len(records) != 1 {
		t.Errorf("expected 1 record since checkpoint, got %d", len(records))
	}
}

func TestMeter_Clear(t *testing.T) {
	m := New()

	m.Record("gpt-4", 100, 50)
	m.Record("gpt-4", 200, 100)

	m.Clear()

	stats := m.Stats()
	if stats.TotalRequests != 0 {
		t.Errorf("expected 0 requests after clear, got %d", stats.TotalRequests)
	}

	records := m.Records()
	if len(records) != 0 {
		t.Errorf("expected 0 records after clear, got %d", len(records))
	}
}

func TestMeter_SetPricing(t *testing.T) {
	m := New()

	m.SetPricing("custom-model", Pricing{
		InputPrice:  10.0,
		OutputPrice: 20.0,
	})

	m.Record("custom-model", 1000, 1000)

	stats := m.StatsByModel("custom-model")

	// 1000 input = $0.01, 1000 output = $0.02
	expectedCost := 0.01 + 0.02
	if stats.EstimatedCost != expectedCost {
		t.Errorf("expected cost $%.4f, got $%.4f", expectedCost, stats.EstimatedCost)
	}
}

func TestMeter_Report(t *testing.T) {
	m := New()

	m.Record("gpt-4", 100, 50)
	m.RecordWithLatency("gpt-4", 200, 100, 150*time.Millisecond)

	report := m.Report()

	if report == "" {
		t.Error("expected non-empty report")
	}

	// 检查报告包含关键信息
	if !contains(report, "Total Requests") {
		t.Error("report should contain 'Total Requests'")
	}
	if !contains(report, "gpt-4") {
		t.Error("report should contain 'gpt-4'")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMeter_JSON(t *testing.T) {
	m := New()

	m.Record("gpt-4", 100, 50)

	data, err := m.JSON()
	if err != nil {
		t.Fatalf("JSON error: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}

func TestMeter_StatsInWindow(t *testing.T) {
	m := New()

	// 记录一些数据
	m.Record("gpt-4", 100, 50)
	time.Sleep(10 * time.Millisecond)
	m.Record("gpt-4", 200, 100)

	// 获取最近 1 秒的统计
	ws := m.StatsInWindow(1 * time.Second)

	if ws.Stats.TotalRequests != 2 {
		t.Errorf("expected 2 requests in window, got %d", ws.Stats.TotalRequests)
	}

	if ws.Window != 1*time.Second {
		t.Errorf("expected window 1s, got %v", ws.Window)
	}
}

func TestNewWithPricing(t *testing.T) {
	customPricing := map[string]Pricing{
		"my-model": {InputPrice: 5.0, OutputPrice: 10.0},
	}

	m := NewWithPricing(customPricing)

	m.Record("my-model", 1000, 1000)

	stats := m.StatsByModel("my-model")

	// 1000 input = $0.005, 1000 output = $0.01
	expectedCost := 0.005 + 0.01
	if stats.EstimatedCost != expectedCost {
		t.Errorf("expected cost $%.4f, got $%.4f", expectedCost, stats.EstimatedCost)
	}
}

func TestGlobal(t *testing.T) {
	// 清空全局统计器
	Global().Clear()

	RecordGlobal("gpt-4", 100, 50)

	stats := StatsGlobal()
	if stats.TotalRequests != 1 {
		t.Errorf("expected 1 global request, got %d", stats.TotalRequests)
	}
}

func TestTracker(t *testing.T) {
	m := New()

	tracker := m.NewTracker("gpt-4").SetInputTokens(100)
	time.Sleep(10 * time.Millisecond)
	tracker.Done(50)

	stats := m.Stats()

	if stats.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", stats.TotalRequests)
	}
	if stats.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", stats.InputTokens)
	}
	if stats.OutputTokens != 50 {
		t.Errorf("expected 50 output tokens, got %d", stats.OutputTokens)
	}
	if stats.AvgLatency < 10*time.Millisecond {
		t.Errorf("expected latency >= 10ms, got %v", stats.AvgLatency)
	}
}

func TestTracker_Error(t *testing.T) {
	m := New()

	tracker := m.NewTracker("gpt-4").SetInputTokens(100)
	tracker.Error(errors.New("timeout"))

	stats := m.Stats()

	if stats.SuccessRequests != 0 {
		t.Errorf("expected 0 success, got %d", stats.SuccessRequests)
	}
	if stats.FailedRequests != 1 {
		t.Errorf("expected 1 failure, got %d", stats.FailedRequests)
	}
}

func TestMeter_Concurrent(t *testing.T) {
	m := New()

	done := make(chan bool)

	// 并发记录
	for i := 0; i < 100; i++ {
		go func() {
			m.Record("gpt-4", 100, 50)
			done <- true
		}()
	}

	// 等待完成
	for i := 0; i < 100; i++ {
		<-done
	}

	stats := m.Stats()
	if stats.TotalRequests != 100 {
		t.Errorf("expected 100 requests, got %d", stats.TotalRequests)
	}
}

func BenchmarkMeter_Record(b *testing.B) {
	m := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Record("gpt-4", 100, 50)
	}
}

func BenchmarkMeter_Stats(b *testing.B) {
	m := New()
	for i := 0; i < 1000; i++ {
		m.Record("gpt-4", 100, 50)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Stats()
	}
}

func TestMeter_MaxRecords(t *testing.T) {
	// 创建一个最大记录数为 100 的统计器
	m := NewWithOptions(nil, 100)

	// 记录 150 条
	for i := 0; i < 150; i++ {
		m.Record("gpt-4", 100, 50)
	}

	records := m.Records()
	// 超过限制后会删除 10%（10 条），所以应该剩余 90 条左右
	// 由于是在超过时触发清理，实际记录数应该在 90-100 之间
	if len(records) > 100 {
		t.Errorf("expected records <= 100, got %d", len(records))
	}

	// 验证计数器仍然正确统计了所有请求
	stats := m.Stats()
	if stats.TotalRequests != 150 {
		t.Errorf("expected 150 total requests, got %d", stats.TotalRequests)
	}
}

func TestMeter_NewWithOptions(t *testing.T) {
	customPricing := map[string]Pricing{
		"custom-model": {InputPrice: 1.0, OutputPrice: 2.0},
	}

	m := NewWithOptions(customPricing, 50)

	m.Record("custom-model", 1000, 1000)

	stats := m.StatsByModel("custom-model")

	// 1000 input = $0.001, 1000 output = $0.002
	expectedCost := 0.001 + 0.002
	if stats.EstimatedCost != expectedCost {
		t.Errorf("expected cost $%.4f, got $%.4f", expectedCost, stats.EstimatedCost)
	}
}
