package template

import (
	"testing"
)

func TestTemplate_Render(t *testing.T) {
	tmpl := New("test").
		System("You are a helpful assistant.").
		User("Hello, my name is {{.Name}}!")

	messages, err := tmpl.Render(map[string]any{
		"Name": "Alice",
	})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	if messages[0].Role != RoleSystem {
		t.Errorf("expected system role, got %s", messages[0].Role)
	}
	if messages[0].Content != "You are a helpful assistant." {
		t.Errorf("unexpected system content: %s", messages[0].Content)
	}

	if messages[1].Role != RoleUser {
		t.Errorf("expected user role, got %s", messages[1].Role)
	}
	if messages[1].Content != "Hello, my name is Alice!" {
		t.Errorf("unexpected user content: %s", messages[1].Content)
	}
}

func TestTemplate_Conditional(t *testing.T) {
	tmpl := New("conditional").
		System("You are {{if .Expert}}an expert{{else}}a helpful{{end}} assistant.")

	// Expert = true
	messages1, err := tmpl.Render(map[string]any{"Expert": true})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if messages1[0].Content != "You are an expert assistant." {
		t.Errorf("unexpected content: %s", messages1[0].Content)
	}

	// Expert = false
	messages2, err := tmpl.Render(map[string]any{"Expert": false})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}
	if messages2[0].Content != "You are a helpful assistant." {
		t.Errorf("unexpected content: %s", messages2[0].Content)
	}
}

func TestTemplate_Funcs(t *testing.T) {
	tmpl := New("funcs").
		User("Name: {{upper .Name}}")

	messages, err := tmpl.Render(map[string]any{"Name": "alice"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if messages[0].Content != "Name: ALICE" {
		t.Errorf("unexpected content: %s", messages[0].Content)
	}
}

func TestTemplate_CustomFuncs(t *testing.T) {
	tmpl := New("custom").
		Funcs(map[string]any{
			"double": func(s string) string { return s + s },
		}).
		User("{{double .Word}}")

	messages, err := tmpl.Render(map[string]any{"Word": "hi"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if messages[0].Content != "hihi" {
		t.Errorf("unexpected content: %s", messages[0].Content)
	}
}

func TestTemplate_Delims(t *testing.T) {
	tmpl := New("delims").
		Delims("[[", "]]").
		User("Hello, [[.Name]]!")

	messages, err := tmpl.Render(map[string]any{"Name": "Bob"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if messages[0].Content != "Hello, Bob!" {
		t.Errorf("unexpected content: %s", messages[0].Content)
	}
}

func TestTemplate_AllRoles(t *testing.T) {
	tmpl := New("roles").
		System("System message").
		User("User message").
		Assistant("Assistant message").
		Tool("Tool message")

	messages, err := tmpl.Render(nil)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}

	expectedRoles := []Role{RoleSystem, RoleUser, RoleAssistant, RoleTool}
	for i, role := range expectedRoles {
		if messages[i].Role != role {
			t.Errorf("message %d: expected role %s, got %s", i, role, messages[i].Role)
		}
	}
}

func TestTemplate_UserWithName(t *testing.T) {
	tmpl := New("named").
		UserWithName("Alice", "Hello!")

	messages, err := tmpl.Render(nil)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if messages[0].Name != "Alice" {
		t.Errorf("expected name 'Alice', got '%s'", messages[0].Name)
	}
}

func TestTemplate_RenderToString(t *testing.T) {
	tmpl := New("string").
		System("System").
		User("User")

	str, err := tmpl.RenderToString(nil)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	expected := "system: System\n\nuser: User"
	if str != expected {
		t.Errorf("unexpected string: %s", str)
	}
}

func TestTemplate_Clone(t *testing.T) {
	original := New("original").
		System("System message")

	clone := original.Clone()
	clone.User("User message")

	originalMessages, _ := original.Render(nil)
	cloneMessages, _ := clone.Render(nil)

	if len(originalMessages) != 1 {
		t.Errorf("original should have 1 message, got %d", len(originalMessages))
	}
	if len(cloneMessages) != 2 {
		t.Errorf("clone should have 2 messages, got %d", len(cloneMessages))
	}
}

func TestTemplate_Name(t *testing.T) {
	tmpl := New("mytemplate")
	if tmpl.Name() != "mytemplate" {
		t.Errorf("expected name 'mytemplate', got '%s'", tmpl.Name())
	}
}

func TestRegistry(t *testing.T) {
	registry := NewRegistry()

	tmpl := New("test").System("Hello")
	err := registry.Register(tmpl)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}

	got, err := registry.Get("test")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if got.Name() != "test" {
		t.Errorf("unexpected name: %s", got.Name())
	}

	// 不存在的模板
	_, err = registry.Get("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// 列出模板
	names := registry.List()
	if len(names) != 1 || names[0] != "test" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestRegistry_EmptyName(t *testing.T) {
	registry := NewRegistry()
	tmpl := New("")

	err := registry.Register(tmpl)
	if err != ErrEmptyName {
		t.Errorf("expected ErrEmptyName, got %v", err)
	}
}

func TestGlobalRegistry(t *testing.T) {
	tmpl := New("global-test").System("Global")
	err := Register(tmpl)
	if err != nil {
		t.Fatalf("register error: %v", err)
	}

	got, err := Get("global-test")
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if got.Name() != "global-test" {
		t.Errorf("unexpected name: %s", got.Name())
	}
}

func TestMustGet(t *testing.T) {
	tmpl := New("must-get-test").System("Test")
	Register(tmpl)

	got := MustGet("must-get-test")
	if got.Name() != "must-get-test" {
		t.Errorf("unexpected name: %s", got.Name())
	}

	// MustGet 不存在应该 panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	MustGet("nonexistent-must-get")
}

func TestParse(t *testing.T) {
	content := `@name: greeting
@system: You are a helpful assistant.
@user: Hello, {{.Name}}!
`
	tmpl, err := Parse(content)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if tmpl.Name() != "greeting" {
		t.Errorf("expected name 'greeting', got '%s'", tmpl.Name())
	}

	messages, err := tmpl.Render(map[string]any{"Name": "Alice"})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	if messages[0].Content != "You are a helpful assistant." {
		t.Errorf("unexpected system content: %s", messages[0].Content)
	}
	if messages[1].Content != "Hello, Alice!" {
		t.Errorf("unexpected user content: %s", messages[1].Content)
	}
}

func TestParse_Multiline(t *testing.T) {
	content := `@name: multiline
@system: Line 1
Line 2
Line 3
@user: Question
`
	tmpl, err := Parse(content)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	messages, err := tmpl.Render(nil)
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	expected := "Line 1\nLine 2\nLine 3"
	if messages[0].Content != expected {
		t.Errorf("expected '%s', got '%s'", expected, messages[0].Content)
	}
}

func TestQuickRender(t *testing.T) {
	result, err := QuickRender("Hello, {{.Name}}!", map[string]any{"Name": "World"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if result != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got '%s'", result)
	}
}

func TestBuildMessages(t *testing.T) {
	messages := BuildMessages("System prompt", "User 1", "User 2")

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	if messages[0].Role != RoleSystem || messages[0].Content != "System prompt" {
		t.Errorf("unexpected system message: %+v", messages[0])
	}

	if messages[1].Role != RoleUser || messages[1].Content != "User 1" {
		t.Errorf("unexpected user message 1: %+v", messages[1])
	}
}

func TestBuildMessages_NoSystem(t *testing.T) {
	messages := BuildMessages("", "User only")

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if messages[0].Role != RoleUser {
		t.Errorf("expected user role, got %s", messages[0].Role)
	}
}

func TestChatHistory(t *testing.T) {
	history := NewChatHistory().
		System("System").
		User("Hello").
		Assistant("Hi there").
		User("How are you?")

	messages := history.Messages()

	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}

	if history.Len() != 4 {
		t.Errorf("Len() should be 4, got %d", history.Len())
	}
}

func TestChatHistory_Clone(t *testing.T) {
	original := NewChatHistory().User("Hello")
	clone := original.Clone()
	clone.Assistant("Hi")

	if original.Len() != 1 {
		t.Errorf("original should have 1 message, got %d", original.Len())
	}
	if clone.Len() != 2 {
		t.Errorf("clone should have 2 messages, got %d", clone.Len())
	}
}

func TestChatHistory_Last(t *testing.T) {
	history := NewChatHistory().
		User("1").
		Assistant("2").
		User("3").
		Assistant("4")

	last2 := history.Last(2)
	if len(last2) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(last2))
	}
	if last2[0].Content != "3" || last2[1].Content != "4" {
		t.Errorf("unexpected messages: %v", last2)
	}

	// 请求超过总数
	lastAll := history.Last(10)
	if len(lastAll) != 4 {
		t.Errorf("expected 4 messages, got %d", len(lastAll))
	}
}

func TestChatHistory_Clear(t *testing.T) {
	history := NewChatHistory().User("Hello").Assistant("Hi")
	history.Clear()

	if history.Len() != 0 {
		t.Errorf("expected 0 messages after clear, got %d", history.Len())
	}
}

func TestChatHistory_Add(t *testing.T) {
	history := NewChatHistory().Add(RoleTool, "Tool result")

	messages := history.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Role != RoleTool {
		t.Errorf("expected tool role, got %s", messages[0].Role)
	}
}

func TestDefaultFuncs_Json(t *testing.T) {
	tmpl := New("json").User("Data: {{json .Data}}")

	messages, err := tmpl.Render(map[string]any{
		"Data": map[string]string{"key": "value"},
	})
	if err != nil {
		t.Fatalf("render error: %v", err)
	}

	expected := `Data: {"key":"value"}`
	if messages[0].Content != expected {
		t.Errorf("expected '%s', got '%s'", expected, messages[0].Content)
	}
}

func TestDefaultFuncs_Default(t *testing.T) {
	tmpl := New("default").User("Name: {{default \"Unknown\" .Name}}")

	// 空值使用默认
	messages1, _ := tmpl.Render(map[string]any{"Name": ""})
	if messages1[0].Content != "Name: Unknown" {
		t.Errorf("expected 'Name: Unknown', got '%s'", messages1[0].Content)
	}

	// 有值使用提供的值
	messages2, _ := tmpl.Render(map[string]any{"Name": "Alice"})
	if messages2[0].Content != "Name: Alice" {
		t.Errorf("expected 'Name: Alice', got '%s'", messages2[0].Content)
	}
}

func TestDefaultFuncs_Truncate(t *testing.T) {
	tmpl := New("truncate").User("Text: {{truncate 5 .Text}}")

	messages, _ := tmpl.Render(map[string]any{"Text": "Hello World"})
	if messages[0].Content != "Text: Hello..." {
		t.Errorf("expected 'Text: Hello...', got '%s'", messages[0].Content)
	}

	// 短文本不截断
	messages2, _ := tmpl.Render(map[string]any{"Text": "Hi"})
	if messages2[0].Content != "Text: Hi" {
		t.Errorf("expected 'Text: Hi', got '%s'", messages2[0].Content)
	}
}
