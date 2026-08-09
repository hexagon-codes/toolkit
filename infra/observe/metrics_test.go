package observe

import (
	"testing"
	"time"
)

func TestTimerContext(t *testing.T) {
	tc := &TimerContext{
		startTime: time.Now().Add(-100 * time.Millisecond),
	}

	duration := tc.Stop()
	if duration < 100*time.Millisecond {
		t.Errorf("expected duration >= 100ms, got %v", duration)
	}
}

func TestNewTimerContext(t *testing.T) {
	tc := NewTimerContext(nil)

	if tc == nil {
		t.Fatal("expected non-nil TimerContext")
		return
	}

	if tc.startTime.IsZero() {
		t.Error("expected non-zero start time")
	}
}

func TestMetricConstants(t *testing.T) {
	// Agent 指标
	agentMetrics := []string{
		MetricAgentRunsTotal,
		MetricAgentRunDuration,
		MetricAgentRunErrors,
		MetricAgentActiveCount,
	}

	for _, m := range agentMetrics {
		if m == "" {
			t.Error("agent metric constant should not be empty")
		}
	}

	// LLM 指标
	llmMetrics := []string{
		MetricLLMCallsTotal,
		MetricLLMCallDuration,
		MetricLLMCallErrors,
		MetricLLMPromptTokens,
		MetricLLMCompletionTokens,
	}

	for _, m := range llmMetrics {
		if m == "" {
			t.Error("llm metric constant should not be empty")
		}
	}

	// Tool 指标
	toolMetrics := []string{
		MetricToolCallsTotal,
		MetricToolCallDuration,
		MetricToolCallErrors,
	}

	for _, m := range toolMetrics {
		if m == "" {
			t.Error("tool metric constant should not be empty")
		}
	}

	// Retrieval 指标
	retrievalMetrics := []string{
		MetricRetrievalTotal,
		MetricRetrievalDuration,
		MetricRetrievalDocCount,
	}

	for _, m := range retrievalMetrics {
		if m == "" {
			t.Error("retrieval metric constant should not be empty")
		}
	}
}
