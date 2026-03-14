package validator

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// FieldError 表示字段验证错误
type FieldError struct {
	Field   string // 字段名
	Tag     string // 验证标签
	Value   any    // 字段值
	Message string // 错误消息
}

// Error 实现 error 接口
func (e FieldError) Error() string {
	return e.Message
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

// RuleFunc 验证规则函数类型
// 参数: value-字段值, param-规则参数
// 返回: 验证是否通过
type RuleFunc func(value any, param string) bool

// Validator 结构体验证器
type Validator struct {
	tagName string               // 验证标签名，默认 "validate"
	rules   map[string]RuleFunc  // 注册的验证规则
	msgs    map[string]string    // 错误消息模板
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
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("obj must be a struct or pointer to struct")
	}

	var errors ValidationErrors
	rt := rv.Type()

	for i := range rv.NumField() {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get(v.tagName)
		if tag == "" || tag == "-" {
			continue
		}

		fieldValue := rv.Field(i).Interface()
		fieldName := getFieldName(field)

		fieldErrors := v.validateField(fieldName, fieldValue, tag)
		errors = append(errors, fieldErrors...)
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

// validateField 验证单个字段
func (v *Validator) validateField(fieldName string, value any, tag string) []FieldError {
	var errors []FieldError
	rules := parseTag(tag)

	for _, rule := range rules {
		ruleName, param := parseRule(rule)

		// 跳过非必填且为空的字段
		if ruleName != "required" && isEmpty(value) {
			continue
		}

		fn, ok := v.rules[ruleName]
		if !ok {
			continue
		}

		if !fn(value, param) {
			msg := v.formatMessage(ruleName, fieldName, param)
			errors = append(errors, FieldError{
				Field:   fieldName,
				Tag:     ruleName,
				Value:   value,
				Message: msg,
			})
		}
	}

	return errors
}

// formatMessage 格式化错误消息
func (v *Validator) formatMessage(rule, field, param string) string {
	msg, ok := v.msgs[rule]
	if !ok {
		return fmt.Sprintf("%s 验证失败: %s", field, rule)
	}
	if param != "" {
		return fmt.Sprintf(msg, field, param)
	}
	return fmt.Sprintf(msg, field)
}

// parseTag 解析验证标签
// 处理参数中可能包含逗号的情况，如 range=0,150
// 规则：逗号分隔规则，但参数值内的逗号不分隔
// 示例：
//   - "required,email" -> ["required", "email"]
//   - "required,range=0,150" -> ["required", "range=0,150"]
//   - "min=2,max=10" -> ["min=2", "max=10"]
func parseTag(tag string) []string {
	var rules []string
	i := 0
	for i < len(tag) {
		// 找下一个规则的起始位置
		start := i

		// 跳过可能的空格
		for start < len(tag) && tag[start] == ' ' {
			start++
		}
		if start >= len(tag) {
			break
		}

		// 找规则结束位置
		end := start
		for end < len(tag) {
			if tag[end] == '=' {
				// 进入参数部分，需要判断后面有几个逗号分隔的参数
				end++
				// 检查是否是 range 规则（参数内有逗号）
				// 找到下一个真正的规则分隔符：规则名=值 后的逗号
				for end < len(tag) && tag[end] != ',' {
					end++
				}
				// 检查下一个逗号后面是否是新规则（包含 = 或是已知规则名）
				if end < len(tag) && tag[end] == ',' {
					// 看看逗号后面是数字还是规则名
					next := end + 1
					for next < len(tag) && tag[next] == ' ' {
						next++
					}
					// 如果后面是数字，说明这个逗号属于参数
					if next < len(tag) && (tag[next] >= '0' && tag[next] <= '9' || tag[next] == '-') {
						// 继续找到真正的规则分隔符
						end = next
						for end < len(tag) && tag[end] != ',' {
							end++
						}
					}
				}
				break
			} else if tag[end] == ',' {
				break
			}
			end++
		}

		if end > start {
			rules = append(rules, strings.TrimSpace(tag[start:end]))
		}
		i = end + 1
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
	case reflect.Ptr, reflect.Interface:
		return rv.IsNil()
	// 数字类型零值不算空，应该正常验证
	}
	return false
}

// checkMin 检查最小值/长度
func checkMin(value any, min int) bool {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		return len([]rune(rv.String())) >= min
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() >= min
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() >= int64(min)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() >= uint64(min)
	case reflect.Float32, reflect.Float64:
		return rv.Float() >= float64(min)
	}
	return false
}

// checkMax 检查最大值/长度
func checkMax(value any, max int) bool {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		return len([]rune(rv.String())) <= max
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() <= max
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() <= int64(max)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() <= uint64(max)
	case reflect.Float32, reflect.Float64:
		return rv.Float() <= float64(max)
	}
	return false
}

// checkLen 检查精确长度
func checkLen(value any, length int) bool {
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
func checkRange(value any, min, max int) bool {
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String:
		length := len([]rune(rv.String()))
		return length >= min && length <= max
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() >= min && rv.Len() <= max
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v := rv.Int()
		return v >= int64(min) && v <= int64(max)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v := rv.Uint()
		return v >= uint64(min) && v <= uint64(max)
	case reflect.Float32, reflect.Float64:
		v := rv.Float()
		return v >= float64(min) && v <= float64(max)
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
	errors := v.validateField("value", value, tag)
	if len(errors) > 0 {
		return ValidationErrors(errors)
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
