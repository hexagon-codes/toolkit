package stringx

import "testing"

func TestReverse(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "olleh"},
		{"世界", "界世"},
		{"hello世界", "界世olleh"},
		{"", ""},
		{"a", "a"},
		{"ab", "ba"},
	}

	for _, tt := range tests {
		result := Reverse(tt.input)
		if result != tt.expected {
			t.Errorf("Reverse(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello world", 8, "hello..."}, // truncates with "..."
		{"hello", 10, "hello"},         // no truncation needed
		{"hello", 5, "hello"},          // exact length
		{"", 5, ""},                    // empty string
		{"hello", 0, ""},               // zero max length
		{"hello", -1, ""},              // negative max length
		{"hello", 3, "hel"},            // maxLen <= 3, no suffix
	}

	for _, tt := range tests {
		result := Truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestTruncateWithSuffix(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		suffix   string
		expected string
	}{
		{"hello world", 8, "...", "hello..."},
		{"hello", 10, "...", "hello"},
		{"hello", 5, "...", "hello"},
		{"hello world", 5, "...", "he..."},
		{"hi", 5, "...", "hi"},
	}

	for _, tt := range tests {
		result := TruncateWithSuffix(tt.input, tt.maxLen, tt.suffix)
		if result != tt.expected {
			t.Errorf("TruncateWithSuffix(%q, %d, %q) = %q, want %q", tt.input, tt.maxLen, tt.suffix, result, tt.expected)
		}
	}
}

func TestPadLeft(t *testing.T) {
	tests := []struct {
		input    string
		length   int
		pad      string
		expected string
	}{
		{"hello", 10, " ", "     hello"},
		{"hello", 10, "*", "*****hello"},
		{"hello", 5, " ", "hello"},
		{"hello", 3, " ", "hello"},
		{"hi", 5, "ab", "abahi"},
	}

	for _, tt := range tests {
		result := PadLeft(tt.input, tt.length, tt.pad)
		if result != tt.expected {
			t.Errorf("PadLeft(%q, %d, %q) = %q, want %q", tt.input, tt.length, tt.pad, result, tt.expected)
		}
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input    string
		length   int
		pad      string
		expected string
	}{
		{"hello", 10, " ", "hello     "},
		{"hello", 10, "*", "hello*****"},
		{"hello", 5, " ", "hello"},
		{"hello", 3, " ", "hello"},
	}

	for _, tt := range tests {
		result := PadRight(tt.input, tt.length, tt.pad)
		if result != tt.expected {
			t.Errorf("PadRight(%q, %d, %q) = %q, want %q", tt.input, tt.length, tt.pad, result, tt.expected)
		}
	}
}

func TestPadCenter(t *testing.T) {
	tests := []struct {
		input    string
		length   int
		pad      string
		expected string
	}{
		{"hello", 11, " ", "   hello   "},
		{"hi", 6, "*", "**hi**"},
		{"hello", 5, " ", "hello"},
	}

	for _, tt := range tests {
		result := PadCenter(tt.input, tt.length, tt.pad)
		if result != tt.expected {
			t.Errorf("PadCenter(%q, %d, %q) = %q, want %q", tt.input, tt.length, tt.pad, result, tt.expected)
		}
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"", true},
		{" ", false},
		{"hello", false},
	}

	for _, tt := range tests {
		result := IsEmpty(tt.input)
		if result != tt.expected {
			t.Errorf("IsEmpty(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestIsBlank(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"", true},
		{" ", true},
		{"  \t\n", true},
		{"hello", false},
		{" hello ", false},
	}

	for _, tt := range tests {
		result := IsBlank(tt.input)
		if result != tt.expected {
			t.Errorf("IsBlank(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestDefaultIfEmpty(t *testing.T) {
	if DefaultIfEmpty("", "default") != "default" {
		t.Error("DefaultIfEmpty should return default for empty string")
	}

	if DefaultIfEmpty("hello", "default") != "hello" {
		t.Error("DefaultIfEmpty should return original for non-empty string")
	}
}

func TestDefaultIfBlank(t *testing.T) {
	if DefaultIfBlank("", "default") != "default" {
		t.Error("DefaultIfBlank should return default for empty string")
	}

	if DefaultIfBlank("  ", "default") != "default" {
		t.Error("DefaultIfBlank should return default for blank string")
	}

	if DefaultIfBlank("hello", "default") != "hello" {
		t.Error("DefaultIfBlank should return original for non-blank string")
	}
}

func TestSubString(t *testing.T) {
	tests := []struct {
		input    string
		start    int
		end      int
		expected string
	}{
		{"hello world", 0, 5, "hello"},
		{"hello world", 6, 11, "world"},
		{"hello", 0, 100, "hello"},
		{"hello", -1, 3, "hel"},
		{"hello", 2, 2, ""},
		{"", 0, 5, ""},
	}

	for _, tt := range tests {
		result := SubString(tt.input, tt.start, tt.end)
		if result != tt.expected {
			t.Errorf("SubString(%q, %d, %d) = %q, want %q", tt.input, tt.start, tt.end, result, tt.expected)
		}
	}
}

func TestContainsAny(t *testing.T) {
	if !ContainsAny("hello world", "world", "foo") {
		t.Error("ContainsAny should return true")
	}

	if ContainsAny("hello world", "foo", "bar") {
		t.Error("ContainsAny should return false")
	}

	if ContainsAny("hello" /* empty */) {
		t.Error("ContainsAny with no subs should return false")
	}
}

func TestContainsAll(t *testing.T) {
	if !ContainsAll("hello world", "hello", "world") {
		t.Error("ContainsAll should return true")
	}

	if ContainsAll("hello world", "hello", "foo") {
		t.Error("ContainsAll should return false")
	}

	if !ContainsAll("hello" /* empty */) {
		t.Error("ContainsAll with no subs should return true")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if FirstNonEmpty("", "", "hello", "world") != "hello" {
		t.Error("FirstNonEmpty should return first non-empty string")
	}

	if FirstNonEmpty("", "") != "" {
		t.Error("FirstNonEmpty should return empty if all empty")
	}

	if FirstNonEmpty() != "" {
		t.Error("FirstNonEmpty with no args should return empty")
	}
}

func TestRepeat(t *testing.T) {
	if Repeat("ab", 3) != "ababab" {
		t.Error("Repeat should repeat string")
	}

	if Repeat("ab", 0) != "" {
		t.Error("Repeat 0 times should return empty")
	}

	if Repeat("ab", -1) != "" {
		t.Error("Repeat negative times should return empty")
	}
}

func TestRemovePrefix(t *testing.T) {
	if RemovePrefix("hello world", "hello ") != "world" {
		t.Error("RemovePrefix should remove prefix")
	}

	if RemovePrefix("hello world", "foo") != "hello world" {
		t.Error("RemovePrefix should not modify if no match")
	}
}

func TestRemoveSuffix(t *testing.T) {
	if RemoveSuffix("hello world", " world") != "hello" {
		t.Error("RemoveSuffix should remove suffix")
	}

	if RemoveSuffix("hello world", "foo") != "hello world" {
		t.Error("RemoveSuffix should not modify if no match")
	}
}

func TestEnsurePrefix(t *testing.T) {
	if EnsurePrefix("world", "hello ") != "hello world" {
		t.Error("EnsurePrefix should add prefix")
	}

	if EnsurePrefix("hello world", "hello ") != "hello world" {
		t.Error("EnsurePrefix should not add if already present")
	}
}

func TestEnsureSuffix(t *testing.T) {
	if EnsureSuffix("hello", " world") != "hello world" {
		t.Error("EnsureSuffix should add suffix")
	}

	if EnsureSuffix("hello world", " world") != "hello world" {
		t.Error("EnsureSuffix should not add if already present")
	}
}

func TestCountSubstring(t *testing.T) {
	if CountSubstring("abcabc", "abc") != 2 {
		t.Error("CountSubstring should count occurrences")
	}

	if CountSubstring("hello", "x") != 0 {
		t.Error("CountSubstring should return 0 for no matches")
	}
}

func TestSplitAndTrim(t *testing.T) {
	result := SplitAndTrim(" a , b , c ", ",")
	if len(result) != 3 || result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Error("SplitAndTrim should split and trim")
	}

	// With empty parts
	result = SplitAndTrim("a,,b", ",")
	if len(result) != 2 {
		t.Error("SplitAndTrim should skip empty parts")
	}
}
