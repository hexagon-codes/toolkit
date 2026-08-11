package errorx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ============================================================
// 错误码域定义
// ============================================================

// 通用域
const (
	DomainGeneral = "GENERAL"
	// CodeOK 成功
	CodeOK = 0
	// CodeUnknown 未知错误
	CodeUnknown = 1000
	// CodeInvalidInput 无效输入
	CodeInvalidInput = 1001
	// CodeNotFound 资源未找到
	CodeNotFound = 1002
	// CodeConflict 资源冲突
	CodeConflict = 1003
	// CodeTimeout 操作超时
	CodeTimeout = 1004
	// CodeUnavailable 服务不可用
	CodeUnavailable = 1005
	// CodeUnauthorized 未认证
	CodeUnauthorized = 1006
	// CodeForbidden 无权限
	CodeForbidden = 1007
	// CodeRateLimit 请求频率超限
	CodeRateLimit = 1008
	// CodeInternal 内部错误
	CodeInternal = 1009
)

// AI 域
const (
	DomainAI = "AI"
	// CodeLLMError LLM 调用失败
	CodeLLMError = 2000
	// CodeTokenLimit Token 数量超限
	CodeTokenLimit = 2001
	// CodeModelNotFound 模型未找到
	CodeModelNotFound = 2002
	// CodeBudgetExceeded 预算超限
	CodeBudgetExceeded = 2003
	// CodeContentFiltered 内容被过滤
	CodeContentFiltered = 2004
)

// Agent 域
const (
	DomainAgent = "AGENT"
	// CodeAgentError Agent 执行错误
	CodeAgentError = 3000
	// CodeToolError 工具执行错误
	CodeToolError = 3001
	// CodePlanError 规划失败
	CodePlanError = 3002
	// CodeHandoffError 交接失败
	CodeHandoffError = 3003
	// CodeSkillNotFound Skill 未找到
	CodeSkillNotFound = 3004
	// CodeSkillDisabled Skill 已禁用
	CodeSkillDisabled = 3005
)

// 安全域
const (
	DomainSecurity = "SECURITY"
	// CodeInjectionDetected 检测到注入攻击
	CodeInjectionDetected = 4000
	// CodePIIDetected 检测到 PII 信息
	CodePIIDetected = 4001
	// CodeSignatureInvalid 签名无效
	CodeSignatureInvalid = 4002
	// CodePermissionDenied 权限被拒绝
	CodePermissionDenied = 4003
)

// ============================================================
// CodedError 结构化错误
// ============================================================

// CodedError 带错误码的结构化错误
//
// 提供域+错误码+消息+详情的完整错误信息，
// 支持链式构造、errors.Is/As、HTTP 状态码映射和 JSON 序列化。
//
// 使用示例:
//
//	err := errorx.NewCodedError(errorx.CodeNotFound, errorx.DomainGeneral, "用户不存在").
//	    WithDetails("user_id", "123")
//	fmt.Println(err)        // [GENERAL-1002] 用户不存在
//	fmt.Println(err.HTTPStatus()) // 404
type CodedError struct {
	// Code 错误码
	Code int `json:"code"`
	// Domain 错误域（如 GENERAL, AI, AGENT, SECURITY）
	Domain string `json:"domain"`
	// Message 错误消息
	Message string `json:"message"`
	// Details 附加详情
	Details map[string]any `json:"details,omitempty"`
	// cause 底层错误
	cause error
}

// NewCodedError 创建结构化错误
func NewCodedError(code int, domain, message string) *CodedError {
	return &CodedError{
		Code:    code,
		Domain:  domain,
		Message: message,
	}
}

// WithDetails 链式添加详情
func (e *CodedError) WithDetails(key string, val any) *CodedError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = val
	return e
}

// WithCause 包装底层错误
func (e *CodedError) WithCause(err error) *CodedError {
	if isNilError(err) {
		e.cause = nil
	} else {
		e.cause = err
	}
	return e
}

// Error 实现 error 接口
//
// 格式: [DOMAIN-CODE] message
func (e *CodedError) Error() string {
	msg := fmt.Sprintf("[%s-%d] %s", e.Domain, e.Code, e.Message)
	if e.cause != nil {
		msg += ": " + e.cause.Error()
	}
	return msg
}

// Unwrap 实现 errors.Unwrap 接口，支持 errors.Is/As
func (e *CodedError) Unwrap() error {
	return e.cause
}

// HTTPStatus 映射到 HTTP 状态码
//
// 根据错误码自动映射合适的 HTTP 状态码，
// 用于 REST API 响应。
func (e *CodedError) HTTPStatus() int {
	switch e.Code {
	case CodeOK:
		return http.StatusOK
	case CodeInvalidInput:
		return http.StatusBadRequest
	case CodeNotFound, CodeModelNotFound, CodeSkillNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeTimeout:
		return http.StatusGatewayTimeout
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden, CodePermissionDenied, CodeSkillDisabled:
		return http.StatusForbidden
	case CodeRateLimit:
		return http.StatusTooManyRequests
	case CodeBudgetExceeded:
		return http.StatusPaymentRequired
	case CodeContentFiltered, CodeInjectionDetected, CodePIIDetected:
		return http.StatusUnprocessableEntity
	case CodeSignatureInvalid:
		return http.StatusUnauthorized
	case CodeTokenLimit:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusInternalServerError
	}
}

// ToJSON 序列化为 JSON 友好格式
func (e *CodedError) ToJSON() map[string]any {
	m := map[string]any{
		"code":    e.Code,
		"domain":  e.Domain,
		"message": e.Message,
	}
	if e.Details != nil {
		m["details"] = e.Details
	}
	if e.cause != nil {
		m["cause"] = e.cause.Error()
	}
	return m
}

// MarshalJSON 实现 json.Marshaler 接口
func (e *CodedError) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.ToJSON())
}

// IsCodedError 检查错误是否为 CodedError 类型
//
// 支持通过 errors.As 解包嵌套错误。
func IsCodedError(err error) (*CodedError, bool) {
	var ce *CodedError
	if errors.As(err, &ce) {
		return ce, true
	}
	return nil, false
}

// ============================================================
// 便捷构造函数
// ============================================================

// ErrInvalidInput 创建无效输入错误
func ErrInvalidInput(msg string) *CodedError {
	return NewCodedError(CodeInvalidInput, DomainGeneral, msg)
}

// ErrNotFound 创建资源未找到错误
func ErrNotFound(msg string) *CodedError {
	return NewCodedError(CodeNotFound, DomainGeneral, msg)
}

// ErrUnauthorized 创建未认证错误
func ErrUnauthorized(msg string) *CodedError {
	return NewCodedError(CodeUnauthorized, DomainGeneral, msg)
}

// ErrForbidden 创建无权限错误
func ErrForbidden(msg string) *CodedError {
	return NewCodedError(CodeForbidden, DomainGeneral, msg)
}

// ErrInternal 创建内部错误
func ErrInternal(msg string) *CodedError {
	return NewCodedError(CodeInternal, DomainGeneral, msg)
}

// ErrTimeout 创建超时错误
func ErrTimeout(msg string) *CodedError {
	return NewCodedError(CodeTimeout, DomainGeneral, msg)
}

// ErrLLM 创建 LLM 调用错误
func ErrLLM(msg string) *CodedError {
	return NewCodedError(CodeLLMError, DomainAI, msg)
}

// ErrBudgetExceeded 创建预算超限错误
func ErrBudgetExceeded(msg string) *CodedError {
	return NewCodedError(CodeBudgetExceeded, DomainAI, msg)
}

// ErrToolError 创建工具执行错误
func ErrToolError(msg string) *CodedError {
	return NewCodedError(CodeToolError, DomainAgent, msg)
}

// ErrSkillNotFound 创建 Skill 未找到错误
func ErrSkillNotFound(msg string) *CodedError {
	return NewCodedError(CodeSkillNotFound, DomainAgent, msg)
}
