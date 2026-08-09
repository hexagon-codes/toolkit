package reflectx

import (
	"testing"
)

// ========== util.go 测试 ==========

func TestIsZero(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{"nil", nil, true},
		{"zero int", 0, true},
		{"non-zero int", 1, false},
		{"empty string", "", true},
		{"non-empty string", "hello", false},
		{"zero float", 0.0, true},
		{"non-zero float", 1.5, false},
		{"false bool", false, true},
		{"true bool", true, false},
		{"nil slice", []int(nil), true},
		{"empty slice", []int{}, false}, // 空切片不是零值
		{"nil map", map[string]int(nil), true},
		{"nil pointer", (*int)(nil), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsZero(tt.value); got != tt.expected {
				t.Errorf("IsZero(%v) = %v, want %v", tt.value, got, tt.expected)
			}
		})
	}
}

func TestIsNil(t *testing.T) {
	var nilPtr *int
	var nilSlice []int
	var nilMap map[string]int
	var nilChan chan int
	var nilFunc func()
	nonNilPtr := new(int)

	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{"nil", nil, true},
		{"nil pointer", nilPtr, true},
		{"non-nil pointer", nonNilPtr, false},
		{"nil slice", nilSlice, true},
		{"nil map", nilMap, true},
		{"nil chan", nilChan, true},
		{"nil func", nilFunc, true},
		{"int (not nilable)", 42, false},
		{"string (not nilable)", "hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNil(tt.value); got != tt.expected {
				t.Errorf("IsNil(%v) = %v, want %v", tt.value, got, tt.expected)
			}
		})
	}
}

func TestTypeName(t *testing.T) {
	type User struct{ Name string }

	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"nil", nil, "nil"},
		{"int", 42, "int"},
		{"string", "hello", "string"},
		{"struct", User{}, "User"},
		{"pointer to struct", &User{}, "*User"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TypeName(tt.value); got != tt.expected {
				t.Errorf("TypeName(%v) = %v, want %v", tt.value, got, tt.expected)
			}
		})
	}
}

func TestFullTypeName(t *testing.T) {
	if got := FullTypeName(nil); got != "nil" {
		t.Errorf("FullTypeName(nil) = %v, want nil", got)
	}
	if got := FullTypeName(42); got != "int" {
		t.Errorf("FullTypeName(42) = %v, want int", got)
	}
}

func TestKindOf(t *testing.T) {
	if got := KindOf(nil); got.String() != "invalid" {
		t.Errorf("KindOf(nil) = %v, want invalid", got)
	}
	if got := KindOf(42); got.String() != "int" {
		t.Errorf("KindOf(42) = %v, want int", got)
	}
}

func TestIsPtr(t *testing.T) {
	var p *int
	i := 42

	if IsPtr(nil) {
		t.Error("IsPtr(nil) should be false")
	}
	if !IsPtr(p) {
		t.Error("IsPtr(*int) should be true")
	}
	if !IsPtr(&i) {
		t.Error("IsPtr(&int) should be true")
	}
	if IsPtr(i) {
		t.Error("IsPtr(int) should be false")
	}
}

func TestIsStruct(t *testing.T) {
	type User struct{ Name string }

	if IsStruct(nil) {
		t.Error("IsStruct(nil) should be false")
	}
	if !IsStruct(User{}) {
		t.Error("IsStruct(User{}) should be true")
	}
	if !IsStruct(&User{}) {
		t.Error("IsStruct(&User{}) should be true")
	}
	if IsStruct(42) {
		t.Error("IsStruct(42) should be false")
	}
}

func TestIsSlice(t *testing.T) {
	if IsSlice(nil) {
		t.Error("IsSlice(nil) should be false")
	}
	if !IsSlice([]int{1, 2, 3}) {
		t.Error("IsSlice([]int) should be true")
	}
	if IsSlice([3]int{1, 2, 3}) {
		t.Error("IsSlice([3]int) should be false (array)")
	}
}

func TestIsMap(t *testing.T) {
	if IsMap(nil) {
		t.Error("IsMap(nil) should be false")
	}
	if !IsMap(map[string]int{}) {
		t.Error("IsMap(map[string]int) should be true")
	}
	if IsMap([]int{}) {
		t.Error("IsMap([]int) should be false")
	}
}

func TestIsFunc(t *testing.T) {
	fn := func() {}
	if IsFunc(nil) {
		t.Error("IsFunc(nil) should be false")
	}
	if !IsFunc(fn) {
		t.Error("IsFunc(func) should be true")
	}
	if IsFunc(42) {
		t.Error("IsFunc(42) should be false")
	}
}

func TestIsChan(t *testing.T) {
	ch := make(chan int)
	if IsChan(nil) {
		t.Error("IsChan(nil) should be false")
	}
	if !IsChan(ch) {
		t.Error("IsChan(chan) should be true")
	}
	if IsChan(42) {
		t.Error("IsChan(42) should be false")
	}
}

func TestIndirect(t *testing.T) {
	i := 42
	p := &i
	pp := &p

	if Indirect(nil) != nil {
		t.Error("Indirect(nil) should be nil")
	}
	if Indirect(i) != 42 {
		t.Error("Indirect(42) should be 42")
	}
	if Indirect(p) != 42 {
		t.Error("Indirect(&42) should be 42")
	}
	if Indirect(pp) != 42 {
		t.Error("Indirect(&&42) should be 42")
	}

	var nilPtr *int
	if Indirect(nilPtr) != nil {
		t.Error("Indirect(nilPtr) should be nil")
	}
}

func TestNew(t *testing.T) {
	type User struct{ Name string }

	if New(nil) != nil {
		t.Error("New(nil) should be nil")
	}

	u := New(User{})
	if _, ok := u.(*User); !ok {
		t.Error("New(User{}) should return *User")
	}

	up := New(&User{})
	if _, ok := up.(*User); !ok {
		t.Error("New(&User{}) should return *User")
	}
}

func TestSliceLen(t *testing.T) {
	if SliceLen(nil) != -1 {
		t.Error("SliceLen(nil) should be -1")
	}
	if SliceLen(42) != -1 {
		t.Error("SliceLen(42) should be -1")
	}
	if SliceLen([]int{1, 2, 3}) != 3 {
		t.Error("SliceLen([]int{1,2,3}) should be 3")
	}
}

func TestMapLen(t *testing.T) {
	if MapLen(nil) != -1 {
		t.Error("MapLen(nil) should be -1")
	}
	if MapLen(42) != -1 {
		t.Error("MapLen(42) should be -1")
	}
	if MapLen(map[string]int{"a": 1, "b": 2}) != 2 {
		t.Error("MapLen(map) should be 2")
	}
}

// ========== struct.go 测试 ==========

type testUser struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	Email   string `json:"email,omitempty"`
	private string
	Ignored string `json:"-"`
}

func TestStructToMap(t *testing.T) {
	user := testUser{Name: "Alice", Age: 20, Email: "alice@example.com", private: "secret"}
	m := StructToMap(user)

	if m["Name"] != "Alice" {
		t.Errorf("expected Name=Alice, got %v", m["Name"])
	}
	if m["Age"] != 20 {
		t.Errorf("expected Age=20, got %v", m["Age"])
	}
	if _, ok := m["private"]; ok {
		t.Error("private field should not be exported")
	}

	// 测试非结构体
	if StructToMap(42) != nil {
		t.Error("StructToMap(42) should return nil")
	}
}

func TestStructToMapWithTag(t *testing.T) {
	user := testUser{Name: "Alice", Age: 20, Ignored: "should not appear"}
	m := StructToMapWithTag(user, "json")

	if m["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", m["name"])
	}
	if m["age"] != 20 {
		t.Errorf("expected age=20, got %v", m["age"])
	}
	if _, ok := m["Ignored"]; ok {
		t.Error("Ignored field should not appear")
	}

	// 测试指针
	m2 := StructToMapWithTag(&user, "json")
	if m2["name"] != "Alice" {
		t.Error("should work with pointer")
	}
}

func TestMapToStruct(t *testing.T) {
	data := map[string]any{"Name": "Bob", "Age": 25, "Email": "bob@example.com"}
	var user testUser
	err := MapToStruct(data, &user)
	if err != nil {
		t.Fatalf("MapToStruct error: %v", err)
	}
	if user.Name != "Bob" {
		t.Errorf("expected Name=Bob, got %v", user.Name)
	}
	if user.Age != 25 {
		t.Errorf("expected Age=25, got %v", user.Age)
	}

	// 大小写不敏感
	data2 := map[string]any{"name": "Carol", "age": 30}
	var user2 testUser
	err = MapToStruct(data2, &user2)
	if err != nil {
		t.Fatalf("MapToStruct error: %v", err)
	}
	if user2.Name != "Carol" {
		t.Errorf("expected Name=Carol, got %v", user2.Name)
	}
}

func TestMapToStruct_Errors(t *testing.T) {
	data := map[string]any{"Name": "Test"}

	// 非指针
	var user testUser
	err := MapToStruct(data, user)
	if err == nil {
		t.Error("expected error for non-pointer")
	}

	// nil 指针
	var nilPtr *testUser
	err = MapToStruct(data, nilPtr)
	if err == nil {
		t.Error("expected error for nil pointer")
	}

	// 非结构体指针
	var i int
	err = MapToStruct(data, &i)
	if err == nil {
		t.Error("expected error for non-struct pointer")
	}
}

func TestMapToStructWithTag(t *testing.T) {
	data := map[string]any{"name": "Dave", "age": 35}
	var user testUser
	err := MapToStructWithTag(data, &user, "json")
	if err != nil {
		t.Fatalf("MapToStructWithTag error: %v", err)
	}
	if user.Name != "Dave" {
		t.Errorf("expected Name=Dave, got %v", user.Name)
	}
}

func TestGetField(t *testing.T) {
	user := testUser{Name: "Eve", Age: 28}

	name, ok := GetField(user, "Name")
	if !ok || name != "Eve" {
		t.Errorf("expected Eve, got %v", name)
	}

	// 不存在的字段
	_, ok = GetField(user, "NotExist")
	if ok {
		t.Error("should return false for non-existent field")
	}

	// 指针
	name2, ok := GetField(&user, "Name")
	if !ok || name2 != "Eve" {
		t.Error("should work with pointer")
	}

	// 非结构体
	_, ok = GetField(42, "Field")
	if ok {
		t.Error("should return false for non-struct")
	}
}

func TestGetFieldValue(t *testing.T) {
	user := testUser{Name: "Frank", Age: 40}

	name, ok := GetFieldValue[string](user, "Name")
	if !ok || name != "Frank" {
		t.Errorf("expected Frank, got %v", name)
	}

	// 类型不匹配
	_, ok = GetFieldValue[int](user, "Name")
	if ok {
		t.Error("should return false for type mismatch")
	}
}

func TestSetField(t *testing.T) {
	user := &testUser{Name: "Grace", Age: 45}

	err := SetField(user, "Age", 46)
	if err != nil {
		t.Fatalf("SetField error: %v", err)
	}
	if user.Age != 46 {
		t.Errorf("expected Age=46, got %v", user.Age)
	}

	// 非指针
	err = SetField(testUser{}, "Age", 50)
	if err == nil {
		t.Error("expected error for non-pointer")
	}

	// 不存在的字段
	err = SetField(user, "NotExist", "value")
	if err == nil {
		t.Error("expected error for non-existent field")
	}
}

func TestHasField(t *testing.T) {
	user := testUser{Name: "Henry"}

	if !HasField(user, "Name") {
		t.Error("should have Name field")
	}
	if HasField(user, "NotExist") {
		t.Error("should not have NotExist field")
	}
	if HasField(42, "Field") {
		t.Error("should return false for non-struct")
	}
}

func TestFieldNames(t *testing.T) {
	user := testUser{Name: "Ivy"}
	names := FieldNames(user)

	expected := map[string]bool{"Name": true, "Age": true, "Email": true, "Ignored": true}
	for _, name := range names {
		if !expected[name] {
			t.Errorf("unexpected field: %s", name)
		}
	}

	// 非结构体
	if FieldNames(42) != nil {
		t.Error("should return nil for non-struct")
	}
}

func TestFieldTags(t *testing.T) {
	user := testUser{Name: "Jack"}
	tags := FieldTags(user, "json")

	if tags["Name"] != "name" {
		t.Errorf("expected name tag, got %v", tags["Name"])
	}
	if tags["Age"] != "age" {
		t.Errorf("expected age tag, got %v", tags["Age"])
	}

	// 非结构体
	if FieldTags(42, "json") != nil {
		t.Error("should return nil for non-struct")
	}
}

// ========== copy.go 测试 ==========

func TestDeepCopy_BasicTypes(t *testing.T) {
	// int
	i := 42
	iCopy := DeepCopy(i)
	if iCopy != i {
		t.Errorf("expected %d, got %d", i, iCopy)
	}

	// string
	s := "hello"
	sCopy := DeepCopy(s)
	if sCopy != s {
		t.Errorf("expected %s, got %s", s, sCopy)
	}

	// float
	f := 3.14
	fCopy := DeepCopy(f)
	if fCopy != f {
		t.Errorf("expected %f, got %f", f, fCopy)
	}
}

func TestDeepCopy_Slice(t *testing.T) {
	original := []int{1, 2, 3}
	copied := DeepCopy(original)

	// 验证值相等
	if len(copied) != len(original) {
		t.Errorf("expected len %d, got %d", len(original), len(copied))
	}
	for i := range original {
		if copied[i] != original[i] {
			t.Errorf("index %d: expected %d, got %d", i, original[i], copied[i])
		}
	}

	// 验证是独立副本
	copied[0] = 100
	if original[0] == 100 {
		t.Error("modifying copy should not affect original")
	}

	// nil 切片
	var nilSlice []int
	nilCopy := DeepCopy(nilSlice)
	if nilCopy != nil {
		t.Error("copy of nil slice should be nil")
	}
}

func TestDeepCopy_Map(t *testing.T) {
	original := map[string]int{"a": 1, "b": 2}
	copied := DeepCopy(original)

	// 验证值相等
	if len(copied) != len(original) {
		t.Errorf("expected len %d, got %d", len(original), len(copied))
	}
	for k, v := range original {
		if copied[k] != v {
			t.Errorf("key %s: expected %d, got %d", k, v, copied[k])
		}
	}

	// 验证是独立副本
	copied["a"] = 100
	if original["a"] == 100 {
		t.Error("modifying copy should not affect original")
	}

	// nil map
	var nilMap map[string]int
	nilCopy := DeepCopy(nilMap)
	if nilCopy != nil {
		t.Error("copy of nil map should be nil")
	}
}

func TestDeepCopy_Struct(t *testing.T) {
	type Inner struct {
		Value int
	}
	type Outer struct {
		Name  string
		Inner Inner
		Ptr   *int
	}

	val := 42
	original := Outer{
		Name:  "test",
		Inner: Inner{Value: 100},
		Ptr:   &val,
	}
	copied := DeepCopy(original)

	// 验证值相等
	if copied.Name != original.Name {
		t.Errorf("expected Name %s, got %s", original.Name, copied.Name)
	}
	if copied.Inner.Value != original.Inner.Value {
		t.Errorf("expected Inner.Value %d, got %d", original.Inner.Value, copied.Inner.Value)
	}
	if *copied.Ptr != *original.Ptr {
		t.Errorf("expected *Ptr %d, got %d", *original.Ptr, *copied.Ptr)
	}

	// 验证指针是独立副本
	*copied.Ptr = 200
	if *original.Ptr == 200 {
		t.Error("modifying copied pointer should not affect original")
	}
}

func TestDeepCopy_Pointer(t *testing.T) {
	val := 42
	original := &val
	copied := DeepCopy(original)

	// 验证值相等
	if *copied != *original {
		t.Errorf("expected %d, got %d", *original, *copied)
	}

	// 验证是独立副本
	*copied = 100
	if *original == 100 {
		t.Error("modifying copy should not affect original")
	}

	// nil 指针
	var nilPtr *int
	nilCopy := DeepCopy(nilPtr)
	if nilCopy != nil {
		t.Error("copy of nil pointer should be nil")
	}
}

func TestDeepCopy_NestedSlice(t *testing.T) {
	original := [][]int{{1, 2}, {3, 4}}
	copied := DeepCopy(original)

	// 验证是独立副本
	copied[0][0] = 100
	if original[0][0] == 100 {
		t.Error("modifying nested copy should not affect original")
	}
}

func TestDeepCopy_NestedMap(t *testing.T) {
	original := map[string]map[string]int{
		"outer": {"inner": 42},
	}
	copied := DeepCopy(original)

	// 验证是独立副本
	copied["outer"]["inner"] = 100
	if original["outer"]["inner"] == 100 {
		t.Error("modifying nested copy should not affect original")
	}
}

func TestDeepCopy_Array(t *testing.T) {
	original := [3]int{1, 2, 3}
	copied := DeepCopy(original)

	if copied != original {
		t.Errorf("expected %v, got %v", original, copied)
	}

	// 数组本身就是值类型，修改不会影响原值
	copied[0] = 100
	if original[0] == 100 {
		t.Error("array should be value type")
	}
}

func TestClone(t *testing.T) {
	// 基本类型
	i := 42
	iClone := Clone(i)
	if iClone != i {
		t.Errorf("expected %d, got %d", i, iClone)
	}

	// 结构体（浅拷贝）
	type Data struct {
		Values []int
	}
	original := Data{Values: []int{1, 2, 3}}
	cloned := Clone(original)

	// 浅拷贝，切片指向同一底层数组
	cloned.Values[0] = 100
	if original.Values[0] != 100 {
		t.Error("Clone should be shallow copy")
	}
}

// TestDeepCopy_CircularReference 测试循环引用处理
func TestDeepCopy_CircularReference(t *testing.T) {
	// 定义一个可以自引用的结构体
	type Node struct {
		Value int
		Next  *Node
	}

	// 创建一个循环引用链
	node1 := &Node{Value: 1}
	node2 := &Node{Value: 2}
	node3 := &Node{Value: 3}
	node1.Next = node2
	node2.Next = node3
	node3.Next = node1 // 循环引用

	// 执行深拷贝（不应该导致栈溢出）
	copied := DeepCopy(node1)

	// 验证拷贝成功
	if copied == nil {
		t.Fatal("DeepCopy returned nil")
		return
	}
	if copied.Value != 1 {
		t.Errorf("expected Value=1, got %d", copied.Value)
	}
	if copied.Next == nil {
		t.Fatal("Next node not copied")
	}
	if copied.Next.Value != 2 {
		t.Error("Next node copied with wrong value")
	}
	if copied.Next.Next == nil {
		t.Fatal("Next.Next node not copied")
	}
	if copied.Next.Next.Value != 3 {
		t.Error("Next.Next node copied with wrong value")
	}

	// 验证是独立副本（修改原始不影响拷贝）
	node1.Value = 100
	if copied.Value == 100 {
		t.Error("modifying original should not affect copy")
	}
}

// TestDeepCopy_SelfReference 测试自引用结构体
func TestDeepCopy_SelfReference(t *testing.T) {
	type Self struct {
		Value int
		Self  *Self
	}

	// 创建自引用结构
	s := &Self{Value: 42}
	s.Self = s // 自引用

	// 执行深拷贝（不应该导致栈溢出）
	copied := DeepCopy(s)

	// 验证拷贝成功
	if copied == nil {
		t.Fatal("DeepCopy returned nil")
		return
	}
	if copied.Value != 42 {
		t.Errorf("expected Value=42, got %d", copied.Value)
	}

	// 验证是独立副本
	s.Value = 100
	if copied.Value == 100 {
		t.Error("modifying original should not affect copy")
	}
}
