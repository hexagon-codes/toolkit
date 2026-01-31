package httpx

import (
	"time"
)

// ============== AI API 预设客户端 ==============

// OpenAIClient 创建 OpenAI API 客户端
func OpenAIClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://api.openai.com/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// OpenAIClientWithOrg 创建带组织 ID 的 OpenAI API 客户端
func OpenAIClientWithOrg(apiKey, orgID string) *Client {
	return NewClient(
		WithBaseURL("https://api.openai.com/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("OpenAI-Organization", orgID),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// AzureOpenAIClient 创建 Azure OpenAI API 客户端
func AzureOpenAIClient(endpoint, apiKey, apiVersion string) *Client {
	return NewClient(
		WithBaseURL(endpoint),
		WithHeader("api-key", apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// ClaudeClient 创建 Anthropic Claude API 客户端
func ClaudeClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://api.anthropic.com/v1"),
		WithHeader("x-api-key", apiKey),
		WithHeader("anthropic-version", "2023-06-01"),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// ClaudeClientWithVersion 创建指定版本的 Claude API 客户端
func ClaudeClientWithVersion(apiKey, version string) *Client {
	return NewClient(
		WithBaseURL("https://api.anthropic.com/v1"),
		WithHeader("x-api-key", apiKey),
		WithHeader("anthropic-version", version),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// GeminiClient 创建 Google Gemini API 客户端
func GeminiClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://generativelanguage.googleapis.com/v1beta"),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
		// Gemini 使用 URL 参数传递 API Key
	)
}

// VertexAIClient 创建 Google Vertex AI 客户端
func VertexAIClient(projectID, region, accessToken string) *Client {
	baseURL := "https://" + region + "-aiplatform.googleapis.com/v1/projects/" + projectID + "/locations/" + region
	return NewClient(
		WithBaseURL(baseURL),
		WithHeader("Authorization", "Bearer "+accessToken),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// DeepSeekClient 创建 DeepSeek API 客户端
func DeepSeekClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://api.deepseek.com/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// QwenClient 创建通义千问 API 客户端
func QwenClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://dashscope.aliyuncs.com/api/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// ZhipuClient 创建智谱 GLM API 客户端
func ZhipuClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://open.bigmodel.cn/api/paas/v4"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// BaichuanClient 创建百川 API 客户端
func BaichuanClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://api.baichuan-ai.com/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// MoonshotClient 创建月之暗面 Kimi API 客户端
func MoonshotClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://api.moonshot.cn/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// SparkClient 创建讯飞星火 API 客户端
func SparkClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://spark-api-open.xf-yun.com/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// DoubaoClient 创建字节豆包 API 客户端
func DoubaoClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://ark.cn-beijing.volces.com/api/v3"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// MistralClient 创建 Mistral API 客户端
func MistralClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://api.mistral.ai/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// CohereClient 创建 Cohere API 客户端
func CohereClient(apiKey string) *Client {
	return NewClient(
		WithBaseURL("https://api.cohere.ai/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// ============== 自定义 API 客户端 ==============

// CustomAIClient 创建自定义 AI API 客户端
func CustomAIClient(baseURL, apiKey string) *Client {
	return NewClient(
		WithBaseURL(baseURL),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// CustomAIClientWithHeaders 创建带自定义请求头的 AI API 客户端
func CustomAIClientWithHeaders(baseURL string, headers map[string]string) *Client {
	return NewClient(
		WithBaseURL(baseURL),
		WithHeaders(headers),
		WithTimeout(120*time.Second),
	)
}

// ============== 便捷请求方法 ==============

// AIRequest AI API 请求参数
type AIRequest struct {
	Model       string       `json:"model"`
	Messages    []AIMessage  `json:"messages"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
	Stream      bool         `json:"stream,omitempty"`
	Tools       []AITool     `json:"tools,omitempty"`
	ToolChoice  any          `json:"tool_choice,omitempty"`
	TopP        float64      `json:"top_p,omitempty"`
	Stop        []string     `json:"stop,omitempty"`
	N           int          `json:"n,omitempty"`
	Seed        int          `json:"seed,omitempty"`
	User        string       `json:"user,omitempty"`
}

// AIMessage AI 消息
type AIMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content"` // string 或 []ContentPart
	Name       string        `json:"name,omitempty"`
	ToolCalls  []AIToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

// AIContentPart 内容部分（多模态）
type AIContentPart struct {
	Type     string       `json:"type"` // text, image_url
	Text     string       `json:"text,omitempty"`
	ImageURL *AIImageURL  `json:"image_url,omitempty"`
}

// AIImageURL 图片 URL
type AIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // low, high, auto
}

// AITool AI 工具定义
type AITool struct {
	Type     string      `json:"type"` // function
	Function AIFunction  `json:"function"`
}

// AIFunction 函数定义
type AIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"` // JSON Schema
}

// AIToolCall 工具调用
type AIToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"` // function
	Function AIFunctionCall  `json:"function"`
}

// AIFunctionCall 函数调用
type AIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// AIResponse AI API 响应
type AIResponse struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Created int64      `json:"created"`
	Model   string     `json:"model"`
	Choices []AIChoice `json:"choices"`
	Usage   AIUsage    `json:"usage"`
}

// AIChoice 响应选项
type AIChoice struct {
	Index        int       `json:"index"`
	Message      AIMessage `json:"message"`
	FinishReason string    `json:"finish_reason"`
}

// AIUsage Token 使用统计
type AIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletion 发送聊天补全请求
func (c *Client) ChatCompletion(req *AIRequest) (*AIResponse, error) {
	resp, err := c.R().SetJSONBody(req).Post("/chat/completions")
	if err != nil {
		return nil, err
	}

	if resp.IsError() {
		return nil, &AIError{
			StatusCode: resp.StatusCode,
			Body:       string(resp.Body),
		}
	}

	var result AIResponse
	if err := resp.JSON(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ChatCompletionStream 发送流式聊天补全请求
func (c *Client) ChatCompletionStream(req *AIRequest) (*StreamResponse, error) {
	req.Stream = true
	return c.R().SetJSONBody(req).PostStream("/chat/completions")
}

// AIError AI API 错误
type AIError struct {
	StatusCode int
	Body       string
}

func (e *AIError) Error() string {
	return "AI API error: " + e.Body
}
