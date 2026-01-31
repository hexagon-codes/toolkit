package tokenizer

import (
	"testing"
)

func TestCounter_Count(t *testing.T) {
	counter := New(GPT4)

	tests := []struct {
		name     string
		text     string
		minToken int
		maxToken int
	}{
		{
			name:     "empty",
			text:     "",
			minToken: 0,
			maxToken: 0,
		},
		{
			name:     "english word",
			text:     "hello",
			minToken: 1,
			maxToken: 2,
		},
		{
			name:     "english sentence",
			text:     "Hello, world! How are you?",
			minToken: 4,
			maxToken: 10,
		},
		{
			name:     "chinese",
			text:     "你好世界",
			minToken: 2,
			maxToken: 5,
		},
		{
			name:     "mixed",
			text:     "Hello, 世界!",
			minToken: 2,
			maxToken: 6,
		},
		{
			name:     "long text",
			text:     "The quick brown fox jumps over the lazy dog. This is a test sentence.",
			minToken: 10,
			maxToken: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := counter.Count(tt.text)
			if count < tt.minToken || count > tt.maxToken {
				t.Errorf("Count(%q) = %d, expected between %d and %d", tt.text, count, tt.minToken, tt.maxToken)
			}
		})
	}
}

func TestCounter_CountMessages(t *testing.T) {
	counter := New(GPT4)

	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello!"},
	}

	count := counter.CountMessages(messages)

	// 应该包含：
	// - 每个消息的基础 token (3 * 2 = 6)
	// - role tokens
	// - content tokens
	// - 回复前导 (3)
	if count < 10 {
		t.Errorf("CountMessages() = %d, expected >= 10", count)
	}
}

func TestCounter_CountMessages_WithName(t *testing.T) {
	counter := New(GPT4)

	messages := []Message{
		{Role: "user", Content: "Hello!", Name: "Alice"},
	}

	countWithName := counter.CountMessages(messages)

	messagesWithoutName := []Message{
		{Role: "user", Content: "Hello!"},
	}
	countWithoutName := counter.CountMessages(messagesWithoutName)

	if countWithName <= countWithoutName {
		t.Errorf("messages with name should have more tokens: with=%d, without=%d", countWithName, countWithoutName)
	}
}

func TestCounter_CountPrompt(t *testing.T) {
	counter := New(GPT4)

	prompt := "Hello"
	completion := "World"

	count := counter.CountPrompt(prompt, completion)
	expectedMin := counter.Count(prompt) + counter.Count(completion)

	if count < expectedMin {
		t.Errorf("CountPrompt() = %d, expected >= %d", count, expectedMin)
	}
}

func TestCountGPT4(t *testing.T) {
	count := CountGPT4("Hello, world!")
	if count < 1 {
		t.Errorf("CountGPT4() = %d, expected >= 1", count)
	}
}

func TestCountClaude(t *testing.T) {
	count := CountClaude("Hello, world!")
	if count < 1 {
		t.Errorf("CountClaude() = %d, expected >= 1", count)
	}
}

func TestCountGemini(t *testing.T) {
	count := CountGemini("Hello, world!")
	if count < 1 {
		t.Errorf("CountGemini() = %d, expected >= 1", count)
	}
}

func TestCounter_CheckLimit(t *testing.T) {
	counter := New(GPT4)

	// 短文本应该在限制内
	if !counter.CheckLimit("Hello", GPT4Limit) {
		t.Error("short text should be within limit")
	}
}

func TestCounter_CheckMessagesLimit(t *testing.T) {
	counter := New(GPT4)

	messages := []Message{
		{Role: "user", Content: "Hello!"},
	}

	if !counter.CheckMessagesLimit(messages, GPT4Limit) {
		t.Error("short messages should be within limit")
	}
}

func TestCounter_TruncateToLimit(t *testing.T) {
	counter := New(GPT4)

	text := "Hello, world! This is a test sentence that should be truncated."
	truncated := counter.TruncateToLimit(text, 5)

	if counter.Count(truncated) > 5 {
		t.Errorf("truncated text exceeds limit: %d tokens", counter.Count(truncated))
	}

	// 短文本不应被截断
	short := "Hi"
	if counter.TruncateToLimit(short, 100) != short {
		t.Error("short text should not be truncated")
	}
}

func TestCounter_SplitByTokens(t *testing.T) {
	counter := New(GPT4)

	text := "Hello world this is a test sentence for splitting by tokens."
	chunks := counter.SplitByTokens(text, 5)

	if len(chunks) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(chunks))
	}

	// 验证每个块不超过限制
	for i, chunk := range chunks {
		tokens := counter.Count(chunk)
		if tokens > 5 {
			t.Errorf("chunk %d exceeds limit: %d tokens", i, tokens)
		}
	}

	// 短文本不应被分割
	short := "Hi"
	shortChunks := counter.SplitByTokens(short, 100)
	if len(shortChunks) != 1 {
		t.Errorf("short text should not be split, got %d chunks", len(shortChunks))
	}
}

func TestEstimateCost(t *testing.T) {
	// 1M input tokens, 1M output tokens for GPT-4
	cost := EstimateCost(1_000_000, 1_000_000, GPT4Pricing)

	// GPT-4: $30/1M input + $60/1M output = $90
	expected := 90.0
	if cost != expected {
		t.Errorf("EstimateCost() = %f, expected %f", cost, expected)
	}

	// 小规模测试
	smallCost := EstimateCost(1000, 500, GPT4oPricing)
	// GPT-4o: $2.5/1M input + $10/1M output
	// = 0.001 * 2.5 + 0.0005 * 10 = 0.0025 + 0.005 = 0.0075
	expectedSmall := 0.0025 + 0.005
	if smallCost != expectedSmall {
		t.Errorf("EstimateCost() = %f, expected %f", smallCost, expectedSmall)
	}
}

func TestCountRunes(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"hello", 5},
		{"你好", 2},
		{"Hello, 世界!", 10},
		{"", 0},
	}

	for _, tt := range tests {
		count := CountRunes(tt.text)
		if count != tt.expected {
			t.Errorf("CountRunes(%q) = %d, expected %d", tt.text, count, tt.expected)
		}
	}
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"hello world", 2},
		{"one", 1},
		{"", 0},
		{"  spaced   words  ", 2},
	}

	for _, tt := range tests {
		count := CountWords(tt.text)
		if count != tt.expected {
			t.Errorf("CountWords(%q) = %d, expected %d", tt.text, count, tt.expected)
		}
	}
}

func TestCountChineseChars(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"你好", 2},
		{"Hello, 世界!", 2},
		{"hello", 0},
		{"", 0},
	}

	for _, tt := range tests {
		count := CountChineseChars(tt.text)
		if count != tt.expected {
			t.Errorf("CountChineseChars(%q) = %d, expected %d", tt.text, count, tt.expected)
		}
	}
}

func TestNew_DifferentModels(t *testing.T) {
	models := []Model{GPT4, GPT4o, GPT35Turbo, Claude3, Gemini, DeepSeek, Qwen}

	for _, model := range models {
		counter := New(model)
		if counter == nil {
			t.Errorf("New(%s) returned nil", model)
		}

		// 验证能正常计数
		count := counter.Count("Hello, world!")
		if count < 1 {
			t.Errorf("New(%s).Count() = %d, expected >= 1", model, count)
		}
	}
}

func TestContextLimits(t *testing.T) {
	limits := []ContextLimit{
		GPT4Limit,
		GPT4oLimit,
		GPT35TurboLimit,
		Claude3OpusLimit,
		Gemini15ProLimit,
		DeepSeekLimit,
	}

	for _, limit := range limits {
		if limit.MaxInputTokens <= 0 {
			t.Errorf("%s: MaxInputTokens should be > 0", limit.Model)
		}
		if limit.MaxOutputTokens <= 0 {
			t.Errorf("%s: MaxOutputTokens should be > 0", limit.Model)
		}
	}
}

func TestPricing(t *testing.T) {
	prices := []Pricing{
		GPT4Pricing,
		GPT4oPricing,
		GPT35TurboPricing,
		Claude3OpusPricing,
		Claude3SonnetPricing,
	}

	for _, p := range prices {
		if p.InputPrice <= 0 {
			t.Errorf("%s: InputPrice should be > 0", p.Model)
		}
		if p.OutputPrice <= 0 {
			t.Errorf("%s: OutputPrice should be > 0", p.Model)
		}
	}
}

func BenchmarkCounter_Count(b *testing.B) {
	counter := New(GPT4)
	text := "Hello, world! This is a benchmark test for token counting."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Count(text)
	}
}

func BenchmarkCounter_CountChinese(b *testing.B) {
	counter := New(GPT4)
	text := "你好，世界！这是一个用于测试 Token 计数的基准测试。"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Count(text)
	}
}

func BenchmarkCounter_CountMessages(b *testing.B) {
	counter := New(GPT4)
	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello, can you help me with something?"},
		{Role: "assistant", Content: "Of course! What do you need help with?"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.CountMessages(messages)
	}
}
