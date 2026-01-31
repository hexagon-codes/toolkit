package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIClient(t *testing.T) {
	client := OpenAIClient("test-api-key")

	if client.baseURL != "https://api.openai.com/v1" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
	if client.headers["Authorization"] != "Bearer test-api-key" {
		t.Errorf("unexpected Authorization header: %s", client.headers["Authorization"])
	}
	if client.headers["Content-Type"] != "application/json" {
		t.Errorf("unexpected Content-Type header: %s", client.headers["Content-Type"])
	}
}

func TestOpenAIClientWithOrg(t *testing.T) {
	client := OpenAIClientWithOrg("test-api-key", "org-123")

	if client.headers["OpenAI-Organization"] != "org-123" {
		t.Errorf("unexpected OpenAI-Organization header: %s", client.headers["OpenAI-Organization"])
	}
}

func TestClaudeClient(t *testing.T) {
	client := ClaudeClient("test-api-key")

	if client.baseURL != "https://api.anthropic.com/v1" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
	if client.headers["x-api-key"] != "test-api-key" {
		t.Errorf("unexpected x-api-key header: %s", client.headers["x-api-key"])
	}
	if client.headers["anthropic-version"] != "2023-06-01" {
		t.Errorf("unexpected anthropic-version header: %s", client.headers["anthropic-version"])
	}
}

func TestClaudeClientWithVersion(t *testing.T) {
	client := ClaudeClientWithVersion("test-api-key", "2024-01-01")

	if client.headers["anthropic-version"] != "2024-01-01" {
		t.Errorf("unexpected anthropic-version header: %s", client.headers["anthropic-version"])
	}
}

func TestGeminiClient(t *testing.T) {
	client := GeminiClient("test-api-key")

	if client.baseURL != "https://generativelanguage.googleapis.com/v1beta" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
}

func TestDeepSeekClient(t *testing.T) {
	client := DeepSeekClient("test-api-key")

	if client.baseURL != "https://api.deepseek.com/v1" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
	if client.headers["Authorization"] != "Bearer test-api-key" {
		t.Errorf("unexpected Authorization header: %s", client.headers["Authorization"])
	}
}

func TestQwenClient(t *testing.T) {
	client := QwenClient("test-api-key")

	if client.baseURL != "https://dashscope.aliyuncs.com/api/v1" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
}

func TestZhipuClient(t *testing.T) {
	client := ZhipuClient("test-api-key")

	if client.baseURL != "https://open.bigmodel.cn/api/paas/v4" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
}

func TestMoonshotClient(t *testing.T) {
	client := MoonshotClient("test-api-key")

	if client.baseURL != "https://api.moonshot.cn/v1" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
}

func TestMistralClient(t *testing.T) {
	client := MistralClient("test-api-key")

	if client.baseURL != "https://api.mistral.ai/v1" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
}

func TestCohereClient(t *testing.T) {
	client := CohereClient("test-api-key")

	if client.baseURL != "https://api.cohere.ai/v1" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
}

func TestCustomAIClient(t *testing.T) {
	client := CustomAIClient("https://custom.api.com/v1", "custom-key")

	if client.baseURL != "https://custom.api.com/v1" {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
	if client.headers["Authorization"] != "Bearer custom-key" {
		t.Errorf("unexpected Authorization header: %s", client.headers["Authorization"])
	}
}

func TestCustomAIClientWithHeaders(t *testing.T) {
	headers := map[string]string{
		"X-Custom-Header": "custom-value",
		"Authorization":   "Custom auth",
	}
	client := CustomAIClientWithHeaders("https://custom.api.com", headers)

	if client.headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("unexpected X-Custom-Header: %s", client.headers["X-Custom-Header"])
	}
	if client.headers["Authorization"] != "Custom auth" {
		t.Errorf("unexpected Authorization: %s", client.headers["Authorization"])
	}
}

func TestVertexAIClient(t *testing.T) {
	client := VertexAIClient("my-project", "us-central1", "test-token")

	expectedBase := "https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1"
	if client.baseURL != expectedBase {
		t.Errorf("unexpected baseURL: %s", client.baseURL)
	}
	if client.headers["Authorization"] != "Bearer test-token" {
		t.Errorf("unexpected Authorization header: %s", client.headers["Authorization"])
	}
}

func TestChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req AIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Model != "gpt-4" {
			t.Errorf("unexpected model: %s", req.Model)
		}

		resp := AIResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: 1677652288,
			Model:   "gpt-4",
			Choices: []AIChoice{
				{
					Index: 0,
					Message: AIMessage{
						Role:    "assistant",
						Content: "Hello! How can I help you?",
					},
					FinishReason: "stop",
				},
			},
			Usage: AIUsage{
				PromptTokens:     10,
				CompletionTokens: 8,
				TotalTokens:      18,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	req := &AIRequest{
		Model: "gpt-4",
		Messages: []AIMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	resp, err := client.ChatCompletion(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("unexpected ID: %s", resp.ID)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello! How can I help you?" {
		t.Errorf("unexpected content: %v", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 18 {
		t.Errorf("unexpected total tokens: %d", resp.Usage.TotalTokens)
	}
}

func TestChatCompletion_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": {"message": "Rate limit exceeded"}}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	req := &AIRequest{
		Model: "gpt-4",
		Messages: []AIMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	_, err := client.ChatCompletion(req)
	if err == nil {
		t.Fatal("expected error")
	}

	aiErr, ok := err.(*AIError)
	if !ok {
		t.Fatalf("expected *AIError, got %T", err)
	}
	if aiErr.StatusCode != 429 {
		t.Errorf("expected status 429, got %d", aiErr.StatusCode)
	}
}

func TestChatCompletionStream(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: [DONE]

`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req AIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if !req.Stream {
			t.Error("expected stream=true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseData))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))

	req := &AIRequest{
		Model: "gpt-4",
		Messages: []AIMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	stream, err := client.ChatCompletionStream(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	// 读取第一个块
	chunk1, err := stream.ReadOpenAIChunk()
	if err != nil {
		t.Fatalf("failed to read chunk: %v", err)
	}
	if chunk1.Choices[0].Delta.Role != "assistant" {
		t.Errorf("unexpected role: %s", chunk1.Choices[0].Delta.Role)
	}

	// 读取第二个块
	chunk2, err := stream.ReadOpenAIChunk()
	if err != nil {
		t.Fatalf("failed to read chunk: %v", err)
	}
	if chunk2.Choices[0].Delta.Content != "Hello" {
		t.Errorf("unexpected content: %s", chunk2.Choices[0].Delta.Content)
	}
}

func TestAIError(t *testing.T) {
	err := &AIError{
		StatusCode: 429,
		Body:       "Rate limit exceeded",
	}

	expected := "AI API error: Rate limit exceeded"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}
}

func TestAIRequest_WithTools(t *testing.T) {
	req := &AIRequest{
		Model: "gpt-4",
		Messages: []AIMessage{
			{Role: "user", Content: "What's the weather?"},
		},
		Tools: []AITool{
			{
				Type: "function",
				Function: AIFunction{
					Name:        "get_weather",
					Description: "Get the weather for a location",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "The city name",
							},
						},
						"required": []string{"location"},
					},
				},
			},
		},
		ToolChoice: "auto",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded AIRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(decoded.Tools))
	}
	if decoded.Tools[0].Function.Name != "get_weather" {
		t.Errorf("unexpected function name: %s", decoded.Tools[0].Function.Name)
	}
}

func TestAIMessage_MultiModal(t *testing.T) {
	msg := AIMessage{
		Role: "user",
		Content: []AIContentPart{
			{Type: "text", Text: "What's in this image?"},
			{
				Type: "image_url",
				ImageURL: &AIImageURL{
					URL:    "https://example.com/image.jpg",
					Detail: "high",
				},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded AIMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Role != "user" {
		t.Errorf("unexpected role: %s", decoded.Role)
	}
}
