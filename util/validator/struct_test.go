package validator

import (
	"strings"
	"testing"
)

type testUserStruct struct {
	Name     string `validate:"required,min=2,max=50" json:"name"`
	Email    string `validate:"required,email" json:"email"`
	Age      int    `validate:"range=0,150"`
	Phone    string `validate:"phone"`
	Password string `validate:"password"`
	Role     string `validate:"oneof=admin,user,guest"`
	private  string // 私有字段，不验证
	Ignored  string `validate:"-"` // 忽略字段
}

func TestValidator_Struct_Success(t *testing.T) {
	v := NewValidator()

	user := testUserStruct{
		Name:     "Alice",
		Email:    "alice@example.com",
		Age:      25,
		Phone:    "13812345678",
		Password: "Abc12345",
		Role:     "admin",
		private:  "must be ignored",
	}

	err := v.Struct(user)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 指针也可以
	err = v.Struct(&user)
	if err != nil {
		t.Errorf("unexpected error with pointer: %v", err)
	}
	if user.private != "must be ignored" {
		t.Error("validator modified an unexported field")
	}
}

func TestValidator_Struct_RequiredFail(t *testing.T) {
	v := NewValidator()

	user := testUserStruct{
		Name:  "", // 必填但为空
		Email: "test@example.com",
		Age:   25,
	}

	err := v.Struct(user)
	if err == nil {
		t.Error("expected error for empty required field")
	}

	errors, ok := err.(ValidationErrors)
	if !ok {
		t.Fatal("expected ValidationErrors type")
	}

	found := false
	for _, e := range errors {
		if e.Field == "name" && e.Tag == "required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected required error for Name field")
	}
}

func TestValidator_Struct_EmailFail(t *testing.T) {
	v := NewValidator()

	user := testUserStruct{
		Name:  "Test",
		Email: "invalid-email",
		Age:   25,
	}

	err := v.Struct(user)
	if err == nil {
		t.Error("expected error for invalid email")
	}

	errors := err.(ValidationErrors)
	found := false
	for _, e := range errors {
		if e.Tag == "email" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected email error")
	}
}

func TestValidator_Struct_MinFail(t *testing.T) {
	v := NewValidator()

	user := testUserStruct{
		Name:  "A", // min=2
		Email: "test@example.com",
		Age:   25,
	}

	err := v.Struct(user)
	if err == nil {
		t.Error("expected error for min length")
	}

	errors := err.(ValidationErrors)
	found := false
	for _, e := range errors {
		if e.Tag == "min" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected min error")
	}
}

func TestValidator_Struct_MaxFail(t *testing.T) {
	v := NewValidator()

	user := testUserStruct{
		Name:  strings.Repeat("a", 51), // max=50
		Email: "test@example.com",
		Age:   25,
	}

	err := v.Struct(user)
	if err == nil {
		t.Error("expected error for max length")
	}

	errors := err.(ValidationErrors)
	found := false
	for _, e := range errors {
		if e.Tag == "max" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected max error")
	}
}

func TestValidator_Struct_RangeFail(t *testing.T) {
	v := NewValidator()

	user := testUserStruct{
		Name:  "Test",
		Email: "test@example.com",
		Age:   200, // range=0,150
	}

	err := v.Struct(user)
	if err == nil {
		t.Error("expected error for out of range")
	}

	errors := err.(ValidationErrors)
	found := false
	for _, e := range errors {
		if e.Tag == "range" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected range error")
	}
}

func TestUnsignedBoundsHandleNegativeRules(t *testing.T) {
	if !checkMin(uint64(0), -1) {
		t.Fatal("checkMin() rejected an unsigned value above a negative minimum")
	}
	if checkMax(uint64(0), -1) {
		t.Fatal("checkMax() accepted an unsigned value below a negative maximum")
	}
	if checkRange(uint64(0), -2, -1) {
		t.Fatal("checkRange() accepted an unsigned value in a negative range")
	}
	if !checkRange(uint64(1), -1, 2) {
		t.Fatal("checkRange() rejected a valid unsigned value")
	}
}

func TestValidator_Struct_PhoneFail(t *testing.T) {
	v := NewValidator()

	user := testUserStruct{
		Name:  "Test",
		Email: "test@example.com",
		Age:   25,
		Phone: "12345", // 无效手机号
	}

	err := v.Struct(user)
	if err == nil {
		t.Error("expected error for invalid phone")
	}

	errors := err.(ValidationErrors)
	found := false
	for _, e := range errors {
		if e.Tag == "phone" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected phone error")
	}
}

func TestValidator_Struct_OneofFail(t *testing.T) {
	v := NewValidator()

	user := testUserStruct{
		Name:  "Test",
		Email: "test@example.com",
		Age:   25,
		Role:  "invalid", // oneof=admin,user,guest
	}

	err := v.Struct(user)
	if err == nil {
		t.Error("expected error for invalid oneof")
	}

	errors := err.(ValidationErrors)
	found := false
	for _, e := range errors {
		if e.Tag == "oneof" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected oneof error")
	}
}

func TestValidator_Struct_SkipEmptyNonRequired(t *testing.T) {
	v := NewValidator()

	user := testUserStruct{
		Name:  "Test",
		Email: "test@example.com",
		Age:   25,
		Phone: "", // 非必填，空值应跳过验证
		Role:  "", // 非必填，空值应跳过验证
	}

	err := v.Struct(user)
	if err != nil {
		t.Errorf("should skip empty non-required fields: %v", err)
	}
}

func TestValidator_Struct_NotStruct(t *testing.T) {
	v := NewValidator()

	err := v.Struct(42)
	if err == nil {
		t.Error("expected error for non-struct")
	}
}

func TestValidator_RegisterRule(t *testing.T) {
	v := NewValidator()

	// 注册自定义规则：偶数
	v.RegisterRule("even", func(value any, _ string) bool {
		if n, ok := value.(int); ok {
			return n%2 == 0
		}
		return false
	})
	v.RegisterMessage("even", "%s 必须是偶数")

	type Data struct {
		Number int `validate:"even"`
	}

	// 偶数通过
	err := v.Struct(Data{Number: 4})
	if err != nil {
		t.Errorf("even number should pass: %v", err)
	}

	// 奇数失败
	err = v.Struct(Data{Number: 3})
	if err == nil {
		t.Error("odd number should fail")
	}
}

func TestValidator_Var(t *testing.T) {
	v := NewValidator()

	// 测试单个变量验证
	err := v.Var("test@example.com", "required,email")
	if err != nil {
		t.Errorf("valid email should pass: %v", err)
	}

	err = v.Var("invalid", "email")
	if err == nil {
		t.Error("invalid email should fail")
	}

	err = v.Var("", "required")
	if err == nil {
		t.Error("empty required should fail")
	}
}

func TestValidator_SetTagName(t *testing.T) {
	v := NewValidator().SetTagName("v")

	type Data struct {
		Name string `v:"required"`
	}

	err := v.Struct(Data{Name: ""})
	if err == nil {
		t.Error("expected error with custom tag name")
	}
}

func TestValidator_Rules(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name  string
		value any
		tag   string
		valid bool
	}{
		{"alpha valid", "hello", "alpha", true},
		{"alpha invalid", "hello123", "alpha", false},
		{"alphanum valid", "hello123", "alphanum", true},
		{"alphanum invalid", "hello-123", "alphanum", false},
		{"numeric valid", "12345", "numeric", true},
		{"numeric invalid", "123a45", "numeric", false},
		{"url valid", "https://example.com", "url", true},
		{"url invalid", "not-a-url", "url", false},
		{"ip valid", "192.168.1.1", "ip", true},
		{"ip invalid", "999.999.999.999", "ip", false},
		{"len valid", "hello", "len=5", true},
		{"len invalid", "hello", "len=3", false},
		{"regexp valid", "abc123", "regexp=^[a-z0-9]+$", true},
		{"regexp invalid", "ABC", "regexp=^[a-z]+$", false},
		{"password valid", "Abc12345", "password", true},
		{"password invalid", "abc123", "password", false},
		{"username valid", "user_123", "username", true},
		{"username invalid", "ab", "username", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Var(tt.value, tt.tag)
			if tt.valid && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestValidator_MinMax_Numbers(t *testing.T) {
	v := NewValidator()

	type Data struct {
		Int   int     `validate:"min=5,max=10"`
		Float float64 `validate:"min=0,max=100"`
	}

	// 有效
	err := v.Struct(Data{Int: 7, Float: 50.5})
	if err != nil {
		t.Errorf("valid numbers should pass: %v", err)
	}

	// Int 太小
	err = v.Struct(Data{Int: 3, Float: 50})
	if err == nil {
		t.Error("int below min should fail")
	}

	// Int 太大
	err = v.Struct(Data{Int: 15, Float: 50})
	if err == nil {
		t.Error("int above max should fail")
	}
}

func TestValidator_MinMax_Slice(t *testing.T) {
	v := NewValidator()

	type Data struct {
		Items []string `validate:"min=2,max=5"`
	}

	// 有效
	err := v.Struct(Data{Items: []string{"a", "b", "c"}})
	if err != nil {
		t.Errorf("valid slice should pass: %v", err)
	}

	// 太少
	err = v.Struct(Data{Items: []string{"a"}})
	if err == nil {
		t.Error("slice below min should fail")
	}

	// 太多
	err = v.Struct(Data{Items: []string{"a", "b", "c", "d", "e", "f"}})
	if err == nil {
		t.Error("slice above max should fail")
	}
}

func TestValidationErrors_Error(t *testing.T) {
	errors := ValidationErrors{
		{Field: "name", Tag: "required", Message: "name 是必填字段"},
		{Field: "email", Tag: "email", Message: "email 必须是有效的邮箱地址"},
	}

	errStr := errors.Error()
	if !strings.Contains(errStr, "name 是必填字段") {
		t.Error("error string should contain first error")
	}
	if !strings.Contains(errStr, "email 必须是有效的邮箱地址") {
		t.Error("error string should contain second error")
	}
}

func TestValidationErrors_HasErrors(t *testing.T) {
	var empty ValidationErrors
	if empty.HasErrors() {
		t.Error("empty should not have errors")
	}

	errors := ValidationErrors{{Field: "test"}}
	if !errors.HasErrors() {
		t.Error("should have errors")
	}
}

func TestGlobal_Struct(t *testing.T) {
	type Data struct {
		Name string `validate:"required"`
	}

	err := Struct(Data{Name: "test"})
	if err != nil {
		t.Errorf("global Struct should work: %v", err)
	}

	err = Struct(Data{Name: ""})
	if err == nil {
		t.Error("global Struct should validate")
	}
}

func TestGlobal_Var(t *testing.T) {
	err := Var("test@example.com", "email")
	if err != nil {
		t.Errorf("global Var should work: %v", err)
	}

	err = Var("invalid", "email")
	if err == nil {
		t.Error("global Var should validate")
	}
}

func TestValidator_IDCard(t *testing.T) {
	v := NewValidator()

	// 有效身份证号（使用正确校验位的测试号码）
	// 110101199003074510 校验位计算：
	// 权重因子：7,9,10,5,8,4,2,1,6,3,7,9,10,5,8,4,2
	// 1*7+1*9+0*10+1*5+0*8+1*4+1*2+9*1+9*6+0*3+0*7+3*9+0*10+7*5+4*8+5*4+1*2
	// = 7+9+0+5+0+4+2+9+54+0+0+27+0+35+32+20+2 = 206
	// 206 % 11 = 8，对应校验码 '4'
	err := v.Var("110101199003074514", "idcard")
	if err != nil {
		t.Errorf("valid idcard should pass: %v", err)
	}

	// 无效身份证号
	err = v.Var("123456789012345678", "idcard")
	if err == nil {
		t.Error("invalid idcard should fail")
	}
}

func TestFieldError_Error(t *testing.T) {
	e := FieldError{
		Field:   "name",
		Tag:     "required",
		Message: "name 是必填字段",
	}

	if e.Error() != "name 是必填字段" {
		t.Errorf("expected message, got: %s", e.Error())
	}
}

func TestValidator_FieldNameFromTag(t *testing.T) {
	v := NewValidator()

	type Data struct {
		UserName string `validate:"required" json:"user_name"`
	}

	err := v.Struct(Data{UserName: ""})
	if err == nil {
		t.Fatal("expected error")
	}

	errors := err.(ValidationErrors)
	// 应该使用 json tag 中的名称
	if errors[0].Field != "user_name" {
		t.Errorf("expected field name 'user_name', got '%s'", errors[0].Field)
	}
}

func TestValidator_FieldNameFromLabel(t *testing.T) {
	v := NewValidator()

	type Data struct {
		UserName string `validate:"required" label:"用户名"`
	}

	err := v.Struct(Data{UserName: ""})
	if err == nil {
		t.Fatal("expected error")
	}

	errors := err.(ValidationErrors)
	// 应该使用 label tag 中的名称
	if errors[0].Field != "用户名" {
		t.Errorf("expected field name '用户名', got '%s'", errors[0].Field)
	}
}
