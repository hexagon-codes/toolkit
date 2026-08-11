// Package validator 提供值与结构体校验工具。
package validator

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

var (
	// ErrInvalidObject 表示传入对象无法作为结构体验证。
	ErrInvalidObject = errors.New("validator: invalid object")
	// ErrUnknownRule 表示验证标签引用了未注册规则。
	ErrUnknownRule = errors.New("validator: unknown rule")
	// ErrInvalidRule 表示已注册规则无效。
	ErrInvalidRule = errors.New("validator: invalid rule")
	// ErrRulePanic 表示自定义规则执行时发生 panic。
	ErrRulePanic = errors.New("validator: rule panic")
)

// FieldError 表示字段验证错误
type FieldError struct {
	Field   string // 字段名
	Tag     string // 验证标签
	Message string // 错误消息
	Cause   error  // 错误原因
}

// Error 实现 error 接口
func (e FieldError) Error() string {
	return e.Message
}

// Unwrap 返回字段错误的稳定分类。
func (e FieldError) Unwrap() error {
	return e.Cause
}

// ValidationErrors 表示多个验证错误
type ValidationErrors []FieldError

// Error 实现 error 接口
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, err := range e {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(err.Message)
	}
	return sb.String()
}

// HasErrors 检查是否有错误
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// Unwrap 返回所有字段错误，支持 errors.Is 和 errors.As 聚合检查。
func (e ValidationErrors) Unwrap() []error {
	errors := make([]error, len(e))
	for index := range e {
		errors[index] = e[index]
	}
	return errors
}

// RuleFunc 验证规则函数类型
// 参数: value-字段值, param-规则参数
// 返回: 验证是否通过
type RuleFunc func(value any, param string) bool

// Validator 结构体验证器
type Validator struct {
	mu      sync.RWMutex
	tagName string              // 验证标签名，默认 "validate"
	rules   map[string]RuleFunc // 注册的验证规则
	msgs    map[string]string   // 错误消息模板
}

type validatorSnapshot struct {
	tagName string
	rules   map[string]RuleFunc
	msgs    map[string]string
}

// NewValidator 创建验证器
//
// 返回:
//   - *Validator: 带有默认规则的验证器
//
// 支持的内置规则:
//   - required: 必填
//   - email: 邮箱格式
//   - phone: 手机号（中国大陆）
//   - url: URL 格式
//   - ip: IP 地址
//   - min=n: 最小值/长度
//   - max=n: 最大值/长度
//   - len=n: 精确长度
//   - range=min,max: 范围
//   - regexp=pattern: 正则匹配
//   - oneof=a,b,c: 枚举值
//   - alpha: 纯字母
//   - alphanum: 字母数字
//   - numeric: 纯数字
//
// 示例:
//
//	type User struct {
//	    Name  string `validate:"required,min=2,max=50"`
//	    Email string `validate:"required,email"`
//	    Age   int    `validate:"range=0,150"`
//	}
//	v := validator.NewValidator()
//	err := v.Struct(&user)
func NewValidator() *Validator {
	v := &Validator{
		tagName: "validate",
		rules:   make(map[string]RuleFunc),
		msgs:    make(map[string]string),
	}
	v.registerDefaultRules()
	v.registerDefaultMessages()
	return v
}

// registerDefaultRules 注册默认验证规则
func (v *Validator) registerDefaultRules() {
	// required - 必填
	v.rules["required"] = func(value any, _ string) bool {
		return !isEmpty(value)
	}

	// email - 邮箱
	v.rules["email"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return Email(str)
	}

	// phone - 手机号
	v.rules["phone"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return Phone(str)
	}

	// url - URL
	v.rules["url"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return URL(str)
	}

	// ip - IP地址
	v.rules["ip"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return IP(str)
	}

	// min - 最小值/长度
	v.rules["min"] = func(value any, param string) bool {
		minVal, err := strconv.Atoi(param)
		if err != nil {
			return false
		}
		return checkMin(value, minVal)
	}

	// max - 最大值/长度
	v.rules["max"] = func(value any, param string) bool {
		maxVal, err := strconv.Atoi(param)
		if err != nil {
			return false
		}
		return checkMax(value, maxVal)
	}

	// len - 精确长度
	v.rules["len"] = func(value any, param string) bool {
		length, err := strconv.Atoi(param)
		if err != nil {
			return false
		}
		return checkLen(value, length)
	}

	// range - 范围
	v.rules["range"] = func(value any, param string) bool {
		parts := strings.Split(param, ",")
		if len(parts) != 2 {
			return false
		}
		minVal, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		maxVal, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			return false
		}
		return checkRange(value, minVal, maxVal)
	}

	// regexp - 正则匹配（使用缓存的 Match 函数提高性能）
	v.rules["regexp"] = func(value any, param string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return Match(str, param)
	}

	// oneof - 枚举值
	v.rules["oneof"] = func(value any, param string) bool {
		options := strings.Split(param, ",")
		strVal := fmt.Sprintf("%v", value)
		for _, opt := range options {
			if strings.TrimSpace(opt) == strVal {
				return true
			}
		}
		return false
	}

	// alpha - 纯字母
	v.rules["alpha"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return IsAlpha(str)
	}

	// alphanum - 字母数字
	v.rules["alphanum"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return IsAlphaNumeric(str)
	}

	// numeric - 纯数字字符串
	v.rules["numeric"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return IsNumeric(str)
	}

	// password - 密码强度
	v.rules["password"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return Password(str)
	}

	// username - 用户名格式
	v.rules["username"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return Username(str)
	}

	// idcard - 身份证号
	v.rules["idcard"] = func(value any, _ string) bool {
		str, ok := value.(string)
		if !ok {
			return false
		}
		return IDCard(str)
	}
}

// registerDefaultMessages 注册默认错误消息
func (v *Validator) registerDefaultMessages() {
	v.msgs["required"] = "%s 是必填字段"
	v.msgs["email"] = "%s 必须是有效的邮箱地址"
	v.msgs["phone"] = "%s 必须是有效的手机号"
	v.msgs["url"] = "%s 必须是有效的 URL"
	v.msgs["ip"] = "%s 必须是有效的 IP 地址"
	v.msgs["min"] = "%s 不能小于 %s"
	v.msgs["max"] = "%s 不能大于 %s"
	v.msgs["len"] = "%s 长度必须是 %s"
	v.msgs["range"] = "%s 必须在 [%s] 范围内"
	v.msgs["regexp"] = "%s 格式不正确"
	v.msgs["oneof"] = "%s 必须是 [%s] 之一"
	v.msgs["alpha"] = "%s 只能包含字母"
	v.msgs["alphanum"] = "%s 只能包含字母和数字"
	v.msgs["numeric"] = "%s 只能包含数字"
	v.msgs["password"] = "%s 必须包含大小写字母和数字，至少8位"
	v.msgs["username"] = "%s 只能包含字母、数字和下划线，4-20位"
	v.msgs["idcard"] = "%s 必须是有效的身份证号"
}

// RegisterRule 注册自定义验证规则
//
// 参数:
//   - name: 规则名称
//   - fn: 验证函数
//
// 返回:
//   - *Validator: 返回自身以支持链式调用
//
// 示例:
//
//	v.RegisterRule("even", func(value any, _ string) bool {
//	    if n, ok := value.(int); ok {
//	        return n%2 == 0
//	    }
//	    return false
//	})
func (v *Validator) RegisterRule(name string, fn RuleFunc) *Validator {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.rules == nil {
		v.rules = make(map[string]RuleFunc)
	}
	v.rules[name] = fn
	return v
}

// RegisterMessage 注册自定义错误消息
//
// 参数:
//   - rule: 规则名称
//   - msg: 消息模板（%s 表示字段名）
//
// 返回:
//   - *Validator: 返回自身以支持链式调用
func (v *Validator) RegisterMessage(rule, msg string) *Validator {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.msgs == nil {
		v.msgs = make(map[string]string)
	}
	v.msgs[rule] = msg
	return v
}

// SetTagName 设置验证标签名
//
// 参数:
//   - tagName: 标签名，默认 "validate"
//
// 返回:
//   - *Validator: 返回自身以支持链式调用
func (v *Validator) SetTagName(tagName string) *Validator {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.tagName = tagName
	return v
}

// Struct 验证结构体
//
// 参数:
//   - obj: 结构体或结构体指针
//
// 返回:
//   - error: 验证错误，无错误返回 nil
//
// 示例:
//
//	type User struct {
//	    Name  string `validate:"required,min=2"`
//	    Email string `validate:"required,email"`
//	}
//	user := User{Name: "A", Email: "invalid"}
//	err := v.Struct(user)
//	if err != nil {
//	    for _, e := range err.(ValidationErrors) {
//	        fmt.Println(e.Field, e.Tag, e.Message)
//	    }
//	}
func (v *Validator) Struct(obj any) error {
	rv := reflect.ValueOf(obj)
	for rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return fmt.Errorf("%w: expected a non-nil struct", ErrInvalidObject)
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("%w: expected a struct or pointer to struct", ErrInvalidObject)
	}

	snapshot := v.snapshot()
	var validationErrors ValidationErrors
	rt := rv.Type()

	for i := range rv.NumField() {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get(snapshot.tagName)
		if tag == "" || tag == "-" {
			continue
		}

		fieldValue := rv.Field(i).Interface()
		fieldName := getFieldName(field)

		fieldErrors := validateField(snapshot, fieldName, fieldValue, tag)
		validationErrors = append(validationErrors, fieldErrors...)
	}

	if len(validationErrors) > 0 {
		return validationErrors
	}
	return nil
}

// validateField 验证单个字段
func validateField(snapshot validatorSnapshot, fieldName string, value any, tag string) []FieldError {
	var errors []FieldError
	rules := parseTag(tag)

	for _, rule := range rules {
		ruleName, param := parseRule(rule)

		fn, ok := snapshot.rules[ruleName]
		if !ok {
			errors = append(errors, FieldError{
				Field:   fieldName,
				Tag:     ruleName,
				Message: fmt.Sprintf("%s validation failed: unknown rule %s", fieldName, ruleName),
				Cause:   ErrUnknownRule,
			})
			continue
		}

		// 只有已注册的非必填规则才能跳过空值。
		if ruleName != "required" && isEmpty(value) {
			continue
		}

		valid, cause := executeRule(fn, value, param)
		if !valid {
			msg := formatMessage(snapshot, ruleName, fieldName, param)
			if cause != nil {
				msg = fmt.Sprintf("%s validation failed for rule %s", fieldName, ruleName)
			}
			errors = append(errors, FieldError{
				Field:   fieldName,
				Tag:     ruleName,
				Message: msg,
				Cause:   cause,
			})
		}
	}

	return errors
}

// formatMessage 格式化错误消息
func formatMessage(snapshot validatorSnapshot, rule, field, param string) string {
	msg, ok := snapshot.msgs[rule]
	if !ok {
		return fmt.Sprintf("%s 验证失败: %s", field, rule)
	}
	if param != "" {
		return fmt.Sprintf(msg, field, param)
	}
	return fmt.Sprintf(msg, field)
}

func executeRule(fn RuleFunc, value any, param string) (valid bool, cause error) {
	if fn == nil {
		return false, ErrInvalidRule
	}
	defer func() {
		if recover() != nil {
			valid = false
			cause = ErrRulePanic
		}
	}()
	return fn(value, param), nil
}

func (v *Validator) snapshot() validatorSnapshot {
	v.mu.RLock()
	defer v.mu.RUnlock()
	rules := make(map[string]RuleFunc, len(v.rules))
	for name, rule := range v.rules {
		rules[name] = rule
	}
	messages := make(map[string]string, len(v.msgs))
	for name, message := range v.msgs {
		messages[name] = message
	}
	return validatorSnapshot{tagName: v.tagName, rules: rules, msgs: messages}
}

// parseTag 解析验证标签
// 处理参数中可能包含逗号的情况，如 range=0,150
// 规则：逗号分隔规则，但参数值内的逗号不分隔
// 示例：
//   - "required,email" -> ["required", "email"]
//   - "required,range=0,150" -> ["required", "range=0,150"]
//   - "min=2,max=10" -> ["min=2", "max=10"]
func parseTag(tag string) []string {
	parts := strings.Split(tag, ",")
	rules := make([]string, 0, len(parts))
	for index := 0; index < len(parts); index++ {
		current := strings.TrimSpace(parts[index])
		if current == "" {
			continue
		}
		name, _ := parseRule(current)
		switch name {
		case "range":
			if index+1 < len(parts) {
				current += "," + strings.TrimSpace(parts[index+1])
				index++
			}
		case "oneof":
			// oneof 的枚举值使用逗号分隔，因此该规则必须位于标签末尾。
			for index+1 < len(parts) {
				current += "," + strings.TrimSpace(parts[index+1])
				index++
			}
		}
		rules = append(rules, current)
	}
	return rules
}

// parseRule 解析单个规则
func parseRule(rule string) (name, param string) {
	rule = strings.TrimSpace(rule)
	idx := strings.Index(rule, "=")
	if idx == -1 {
		return rule, ""
	}
	return rule[:idx], rule[idx+1:]
}

// getFieldName 获取字段显示名称
func getFieldName(field reflect.StructField) string {
	// 优先使用 json tag
	if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
		if idx := strings.Index(jsonTag, ","); idx != -1 {
			jsonTag = jsonTag[:idx]
		}
		if jsonTag != "" {
			return jsonTag
		}
	}
	// 其次使用 label tag
	if label := field.Tag.Get("label"); label != "" {
		return label
	}
	return field.Name
}

// isEmpty 检查值是否为空（仅用于判断是否跳过非必填字段）
// 注意: 数字零值不算空，只有字符串空、nil、空切片/map 才算空
func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		return strings.TrimSpace(rv.String()) == ""
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Pointer, reflect.UnsafePointer:
		return rv.IsNil()
		// 数字类型零值不算空，应该正常验证
	}
	return false
}

// checkMin 检查最小值/长度
func checkMin(value any, minimum int) bool {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		if minimum < 0 {
			return false
		}
		return len([]rune(rv.String())) >= minimum
	case reflect.Slice, reflect.Map, reflect.Array:
		if minimum < 0 {
			return false
		}
		return rv.Len() >= minimum
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() >= int64(minimum)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if minimum <= 0 {
			return true
		}
		return rv.Uint() >= uint64(minimum)
	case reflect.Float32, reflect.Float64:
		return rv.Float() >= float64(minimum)
	}
	return false
}

// checkMax 检查最大值/长度
func checkMax(value any, maximum int) bool {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		return len([]rune(rv.String())) <= maximum
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() <= maximum
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() <= int64(maximum)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if maximum < 0 {
			return false
		}
		return rv.Uint() <= uint64(maximum)
	case reflect.Float32, reflect.Float64:
		return rv.Float() <= float64(maximum)
	}
	return false
}

// checkLen 检查精确长度
func checkLen(value any, length int) bool {
	if length < 0 {
		return false
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		return len([]rune(rv.String())) == length
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == length
	}
	return false
}

// checkRange 检查值是否在范围内
func checkRange(value any, minimum, maximum int) bool {
	if minimum > maximum {
		return false
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		if minimum < 0 {
			return false
		}
		length := len([]rune(rv.String()))
		return length >= minimum && length <= maximum
	case reflect.Slice, reflect.Map, reflect.Array:
		if minimum < 0 {
			return false
		}
		return rv.Len() >= minimum && rv.Len() <= maximum
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v := rv.Int()
		return v >= int64(minimum) && v <= int64(maximum)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if maximum < 0 {
			return false
		}
		v := rv.Uint()
		if minimum > 0 && v < uint64(minimum) {
			return false
		}
		return v <= uint64(maximum)
	case reflect.Float32, reflect.Float64:
		v := rv.Float()
		return v >= float64(minimum) && v <= float64(maximum)
	}
	return false
}

// Var 验证单个变量
//
// 参数:
//   - value: 要验证的值
//   - tag: 验证规则
//
// 返回:
//   - error: 验证错误
//
// 示例:
//
//	err := v.Var("test@example.com", "required,email")
func (v *Validator) Var(value any, tag string) error {
	validationErrors := validateField(v.snapshot(), "value", value, tag)
	if len(validationErrors) > 0 {
		return ValidationErrors(validationErrors)
	}
	return nil
}

// 全局默认验证器
var defaultValidator = NewValidator()

// Struct 使用默认验证器验证结构体
func Struct(obj any) error {
	return defaultValidator.Struct(obj)
}

// Var 使用默认验证器验证变量
func Var(value any, tag string) error {
	return defaultValidator.Var(value, tag)
}
