package httpx

import (
	"fmt"
	neturl "net/url"
	"strings"
	"time"
)

// ============== AI API 预设客户端 ==============

// OpenAIClient 创建 OpenAI API 客户端
func OpenAIClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://api.openai.com/v1", apiKey)
}

// OpenAIClientWithOrg 创建带组织 ID 的 OpenAI API 客户端
func OpenAIClientWithOrg(apiKey, orgID string) (*Client, error) {
	if err := validateRequiredAIConfig("API key", apiKey, "organization ID", orgID); err != nil {
		return nil, err
	}
	return NewClient(
		WithBaseURL("https://api.openai.com/v1"),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("OpenAI-Organization", orgID),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// AzureOpenAIClient 创建 Azure OpenAI API 客户端
func AzureOpenAIClient(endpoint, apiKey, apiVersion string) (*Client, error) {
	if err := validateRequiredAIConfig("endpoint", endpoint, "API key", apiKey, "API version", apiVersion); err != nil {
		return nil, err
	}
	parsedEndpoint, err := neturl.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: parse Azure endpoint: %w", ErrInvalidClientConfig, err)
	}
	query := parsedEndpoint.Query()
	query.Set("api-version", apiVersion)
	parsedEndpoint.RawQuery = query.Encode()
	return NewClient(
		WithBaseURL(parsedEndpoint.String()),
		WithHeader("api-key", apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// ClaudeClient 创建 Anthropic Claude API 客户端
func ClaudeClient(apiKey string) (*Client, error) {
	if err := validateRequiredAIConfig("API key", apiKey); err != nil {
		return nil, err
	}
	return NewClient(
		WithBaseURL("https://api.anthropic.com/v1"),
		WithHeader("x-api-key", apiKey),
		WithHeader("anthropic-version", "2023-06-01"),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// ClaudeClientWithVersion 创建指定版本的 Claude API 客户端
func ClaudeClientWithVersion(apiKey, version string) (*Client, error) {
	if err := validateRequiredAIConfig("API key", apiKey, "API version", version); err != nil {
		return nil, err
	}
	return NewClient(
		WithBaseURL("https://api.anthropic.com/v1"),
		WithHeader("x-api-key", apiKey),
		WithHeader("anthropic-version", version),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// GeminiClient 创建 Google Gemini API 客户端
func GeminiClient(apiKey string) (*Client, error) {
	if err := validateRequiredAIConfig("API key", apiKey); err != nil {
		return nil, err
	}
	return NewClient(
		WithBaseURL("https://generativelanguage.googleapis.com/v1beta"),
		WithHeader("x-goog-api-key", apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// VertexAIClient 创建 Google Vertex AI 客户端
func VertexAIClient(projectID, region, accessToken string) (*Client, error) {
	if err := validateRequiredAIConfig("project ID", projectID, "region", region, "access token", accessToken); err != nil {
		return nil, err
	}
	if err := validateAIEndpointIdentifier("project ID", projectID); err != nil {
		return nil, err
	}
	if err := validateAIEndpointIdentifier("region", region); err != nil {
		return nil, err
	}
	baseURL := "https://" + region + "-aiplatform.googleapis.com/v1/projects/" + projectID + "/locations/" + region
	return NewClient(
		WithBaseURL(baseURL),
		WithHeader("Authorization", "Bearer "+accessToken),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

// DeepSeekClient 创建 DeepSeek API 客户端
func DeepSeekClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://api.deepseek.com/v1", apiKey)
}

// QwenClient 创建通义千问 API 客户端
func QwenClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://dashscope.aliyuncs.com/api/v1", apiKey)
}

// ZhipuClient 创建智谱 GLM API 客户端
func ZhipuClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://open.bigmodel.cn/api/paas/v4", apiKey)
}

// BaichuanClient 创建百川 API 客户端
func BaichuanClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://api.baichuan-ai.com/v1", apiKey)
}

// MoonshotClient 创建月之暗面 Kimi API 客户端
func MoonshotClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://api.moonshot.cn/v1", apiKey)
}

// SparkClient 创建讯飞星火 API 客户端
func SparkClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://spark-api-open.xf-yun.com/v1", apiKey)
}

// DoubaoClient 创建字节豆包 API 客户端
func DoubaoClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://ark.cn-beijing.volces.com/api/v3", apiKey)
}

// MistralClient 创建 Mistral API 客户端
func MistralClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://api.mistral.ai/v1", apiKey)
}

// CohereClient 创建 Cohere API 客户端
func CohereClient(apiKey string) (*Client, error) {
	return newBearerAIClient("https://api.cohere.ai/v1", apiKey)
}

// ============== 自定义 API 客户端 ==============

// CustomAIClient 创建自定义 AI API 客户端
func CustomAIClient(baseURL, apiKey string) (*Client, error) {
	return newBearerAIClient(baseURL, apiKey)
}

// CustomAIClientWithHeaders 创建带自定义请求头的 AI API 客户端
func CustomAIClientWithHeaders(baseURL string, headers map[string]string) (*Client, error) {
	if err := validateRequiredAIConfig("base URL", baseURL); err != nil {
		return nil, err
	}
	return NewClient(
		WithBaseURL(baseURL),
		WithHeaders(headers),
		WithTimeout(120*time.Second),
	)
}

func newBearerAIClient(baseURL, apiKey string) (*Client, error) {
	if err := validateRequiredAIConfig("base URL", baseURL, "API key", apiKey); err != nil {
		return nil, err
	}
	return NewClient(
		WithBaseURL(baseURL),
		WithHeader("Authorization", "Bearer "+apiKey),
		WithHeader("Content-Type", "application/json"),
		WithTimeout(120*time.Second),
	)
}

func validateRequiredAIConfig(fields ...string) error {
	for index := 0; index+1 < len(fields); index += 2 {
		if strings.TrimSpace(fields[index+1]) == "" {
			return fmt.Errorf("%w: %s must not be empty", ErrInvalidClientConfig, fields[index])
		}
	}
	return nil
}

// validateAIEndpointIdentifier 防止配置值改变预设端点的主机或路径结构。
func validateAIEndpointIdentifier(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value || value[0] == '-' || value[len(value)-1] == '-' {
		return fmt.Errorf("%w: %s is not a valid endpoint identifier", ErrInvalidClientConfig, name)
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return fmt.Errorf("%w: %s is not a valid endpoint identifier", ErrInvalidClientConfig, name)
	}
	return nil
}

// ============== 便捷请求方法 ==============

// AIRequest AI API 请求参数
type AIRequest struct {
	Model       string      `json:"model"`
	Messages    []AIMessage `json:"messages"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	Stream      bool        `json:"stream,omitempty"`
	Tools       []AITool    `json:"tools,omitempty"`
	ToolChoice  any         `json:"tool_choice,omitempty"`
	TopP        float64     `json:"top_p,omitempty"`
	Stop        []string    `json:"stop,omitempty"`
	N           int         `json:"n,omitempty"`
	Seed        int         `json:"seed,omitempty"`
	User        string      `json:"user,omitempty"`
}

// AIMessage AI 消息
type AIMessage struct {
	Role       string       `json:"role"`
	Content    any          `json:"content"` // string 或 []ContentPart
	Name       string       `json:"name,omitempty"`
	ToolCalls  []AIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}

// AIContentPart 内容部分（多模态）
type AIContentPart struct {
	Type     string      `json:"type"` // text, image_url
	Text     string      `json:"text,omitempty"`
	ImageURL *AIImageURL `json:"image_url,omitempty"`
}

// AIImageURL 图片 URL
type AIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // low, high, auto
}

// AITool AI 工具定义
type AITool struct {
	Type     string     `json:"type"` // function
	Function AIFunction `json:"function"`
}

// AIFunction 函数定义
type AIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"` // JSON Schema
}

// AIToolCall 工具调用
type AIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"` // function
	Function AIFunctionCall `json:"function"`
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
	if req == nil {
		return nil, fmt.Errorf("%w: AI request must not be nil", ErrInvalidRequest)
	}
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
	if req == nil {
		return nil, fmt.Errorf("%w: AI request must not be nil", ErrInvalidRequest)
	}
	streamRequest := *req
	streamRequest.Stream = true
	return c.R().SetJSONBody(&streamRequest).PostStream("/chat/completions")
}

// AIError AI API 错误
type AIError struct {
	StatusCode int
	Body       string
}

func (e *AIError) Error() string {
	return "AI API error: " + e.Body
}
