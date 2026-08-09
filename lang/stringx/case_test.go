package stringx

import "testing"

func TestCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello_world", "helloWorld"},
		{"HELLO_WORLD", "helloWorld"},
		{"hello-world", "helloWorld"},
		{"Hello World", "helloWorld"},
		{"HelloWorld", "helloWorld"},
		{"hello", "hello"},
		{"HELLO", "hello"},
		{"", ""},
		{"__hello__world__", "helloWorld"},
		{"XMLHttpRequest", "xmlHttpRequest"},
		{"user_id", "userId"},
		{"ID", "id"},
	}

	for _, tt := range tests {
		result := CamelCase(tt.input)
		if result != tt.expected {
			t.Errorf("CamelCase(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestPascalCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello_world", "HelloWorld"},
		{"HELLO_WORLD", "HelloWorld"},
		{"hello-world", "HelloWorld"},
		{"Hello World", "HelloWorld"},
		{"helloWorld", "HelloWorld"},
		{"hello", "Hello"},
		{"", ""},
		{"user_id", "UserId"},
	}

	for _, tt := range tests {
		result := PascalCase(tt.input)
		if result != tt.expected {
			t.Errorf("PascalCase(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HelloWorld", "hello_world"},
		{"helloWorld", "hello_world"},
		{"Hello World", "hello_world"},
		{"hello-world", "hello_world"},
		{"HELLO_WORLD", "hello_world"},
		{"hello", "hello"},
		{"", ""},
		{"XMLHttpRequest", "xml_http_request"},
		{"userId", "user_id"},
		{"ID", "id"},
	}

	for _, tt := range tests {
		result := SnakeCase(tt.input)
		if result != tt.expected {
			t.Errorf("SnakeCase(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestKebabCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HelloWorld", "hello-world"},
		{"helloWorld", "hello-world"},
		{"Hello World", "hello-world"},
		{"hello_world", "hello-world"},
		{"HELLO_WORLD", "hello-world"},
		{"hello", "hello"},
		{"", ""},
		{"userId", "user-id"},
	}

	for _, tt := range tests {
		result := KebabCase(tt.input)
		if result != tt.expected {
			t.Errorf("KebabCase(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestScreamingSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"HelloWorld", "HELLO_WORLD"},
		{"helloWorld", "HELLO_WORLD"},
		{"hello_world", "HELLO_WORLD"},
		{"hello", "HELLO"},
		{"", ""},
	}

	for _, tt := range tests {
		result := ScreamingSnakeCase(tt.input)
		if result != tt.expected {
			t.Errorf("ScreamingSnakeCase(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello_world", "Hello World"},
		{"helloWorld", "Hello World"},
		{"hello-world", "Hello World"},
		{"HELLO_WORLD", "Hello World"},
		{"hello", "Hello"},
		{"", ""},
	}

	for _, tt := range tests {
		result := TitleCase(tt.input)
		if result != tt.expected {
			t.Errorf("TitleCase(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCaseConversionsPreserveUnicodeInitial(t *testing.T) {
	tests := []struct {
		name  string
		apply func(string) string
		input string
		want  string
	}{
		{name: "camel case", apply: CamelCase, input: "hello_éclair", want: "helloÉclair"},
		{name: "pascal case", apply: PascalCase, input: "éclair", want: "Éclair"},
		{name: "title case", apply: TitleCase, input: "hello_éclair", want: "Hello Éclair"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.apply(tt.input); got != tt.want {
				t.Fatalf("conversion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCaseConversionsDoNotSplitSingleUnicodeUppercaseRune(t *testing.T) {
	tests := []struct {
		name  string
		apply func(string) string
		want  string
	}{
		{name: "camel case", apply: CamelCase, want: "éclair"},
		{name: "pascal case", apply: PascalCase, want: "Éclair"},
		{name: "snake case", apply: SnakeCase, want: "éclair"},
		{name: "kebab case", apply: KebabCase, want: "éclair"},
		{name: "screaming snake case", apply: ScreamingSnakeCase, want: "ÉCLAIR"},
		{name: "title case", apply: TitleCase, want: "Éclair"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.apply("Éclair"); got != tt.want {
				t.Fatalf("conversion(%q) = %q, want %q", "Éclair", got, tt.want)
			}
		})
	}
}

func TestSplitWords(t *testing.T) {
	tests := []struct {
		input    string
		expected int // number of words
	}{
		{"hello_world", 2},
		{"HelloWorld", 2},
		{"hello-world", 2},
		{"hello world", 2},
		{"XMLHttpRequest", 3},
		{"hello", 1},
		{"", 0},
	}

	for _, tt := range tests {
		result := splitWords(tt.input)
		if len(result) != tt.expected {
			t.Errorf("splitWords(%q) = %d words, want %d words", tt.input, len(result), tt.expected)
		}
	}
}
