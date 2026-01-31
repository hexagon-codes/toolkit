package json

import (
	"testing"
)

type TestStruct struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestMarshal(t *testing.T) {
	data := TestStruct{Name: "Alice", Age: 30}
	result := Marshal(data)

	expected := `{"name":"Alice","age":30}`
	if result != expected {
		t.Errorf("Marshal = %q, want %q", result, expected)
	}
}

func TestMustMarshal(t *testing.T) {
	data := TestStruct{Name: "Bob", Age: 25}
	result := MustMarshal(data)

	expected := `{"name":"Bob","age":25}`
	if result != expected {
		t.Errorf("MustMarshal = %q, want %q", result, expected)
	}
}

func TestMarshalIndent(t *testing.T) {
	data := TestStruct{Name: "Alice", Age: 30}
	result := MarshalIndent(data)

	if len(result) == 0 {
		t.Error("MarshalIndent returned empty string")
	}

	// Should contain newlines for pretty print
	if result[0] != '{' {
		t.Error("MarshalIndent should start with {")
	}
}

func TestUnmarshal(t *testing.T) {
	jsonStr := `{"name":"Alice","age":30}`
	var result TestStruct

	err := Unmarshal(jsonStr, &result)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("Unmarshal = %+v, want {Name:Alice Age:30}", result)
	}
}

func TestMustUnmarshal(t *testing.T) {
	jsonStr := `{"name":"Bob","age":25}`
	var result TestStruct

	MustUnmarshal(jsonStr, &result)

	if result.Name != "Bob" || result.Age != 25 {
		t.Errorf("MustUnmarshal = %+v, want {Name:Bob Age:25}", result)
	}
}

func TestMustUnmarshal_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustUnmarshal should panic on invalid JSON")
		}
	}()

	var result TestStruct
	MustUnmarshal("invalid json", &result)
}

func TestUnmarshalBytes(t *testing.T) {
	jsonBytes := []byte(`{"name":"Alice","age":30}`)
	var result TestStruct

	err := UnmarshalBytes(jsonBytes, &result)
	if err != nil {
		t.Fatalf("UnmarshalBytes failed: %v", err)
	}

	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("UnmarshalBytes = %+v, want {Name:Alice Age:30}", result)
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		data string
		want bool
	}{
		{`{"name":"Alice"}`, true},
		{`[1,2,3]`, true},
		{`"string"`, true},
		{`123`, true},
		{`invalid`, false},
		{`{"name":}`, false},
	}

	for _, tt := range tests {
		if got := Valid(tt.data); got != tt.want {
			t.Errorf("Valid(%q) = %v, want %v", tt.data, got, tt.want)
		}
	}
}

func TestValidBytes(t *testing.T) {
	if !ValidBytes([]byte(`{"key":"value"}`)) {
		t.Error("ValidBytes should return true for valid JSON")
	}

	if ValidBytes([]byte(`invalid`)) {
		t.Error("ValidBytes should return false for invalid JSON")
	}
}

func TestPretty(t *testing.T) {
	input := `{"name":"Alice","age":30}`
	result := Pretty(input)

	// Should contain indentation
	if result == input {
		t.Error("Pretty should add indentation")
	}
}

func TestPrettyBytes(t *testing.T) {
	input := []byte(`{"name":"Alice","age":30}`)
	result := PrettyBytes(input)

	// Should be different from input (formatted)
	if string(result) == string(input) {
		t.Error("PrettyBytes should add indentation")
	}
}

func TestCompact(t *testing.T) {
	input := `{
  "name": "Alice",
  "age": 30
}`
	result := Compact(input)
	expected := `{"name":"Alice","age":30}`

	if result != expected {
		t.Errorf("Compact = %q, want %q", result, expected)
	}
}

func TestCompactBytes(t *testing.T) {
	input := []byte(`{
  "name": "Alice",
  "age": 30
}`)
	result := CompactBytes(input)
	expected := `{"name":"Alice","age":30}`

	if string(result) != expected {
		t.Errorf("CompactBytes = %q, want %q", string(result), expected)
	}
}

func TestToMap(t *testing.T) {
	jsonStr := `{"name":"Alice","age":30}`
	result, err := ToMap(jsonStr)
	if err != nil {
		t.Fatalf("ToMap failed: %v", err)
	}

	if result["name"] != "Alice" {
		t.Errorf("ToMap[name] = %v, want Alice", result["name"])
	}

	// age is float64 in JSON
	if result["age"] != float64(30) {
		t.Errorf("ToMap[age] = %v, want 30", result["age"])
	}
}

func TestMustToMap(t *testing.T) {
	jsonStr := `{"key":"value"}`
	result := MustToMap(jsonStr)

	if result["key"] != "value" {
		t.Errorf("MustToMap[key] = %v, want value", result["key"])
	}
}

func TestMustToMap_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustToMap should panic on invalid JSON")
		}
	}()

	MustToMap("invalid")
}

func TestToSlice(t *testing.T) {
	jsonStr := `[1, 2, 3]`
	result, err := ToSlice(jsonStr)
	if err != nil {
		t.Fatalf("ToSlice failed: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("ToSlice returned %d items, want 3", len(result))
	}
}

func TestMustToSlice(t *testing.T) {
	jsonStr := `["a", "b", "c"]`
	result := MustToSlice(jsonStr)

	if len(result) != 3 {
		t.Errorf("MustToSlice returned %d items, want 3", len(result))
	}
}

func TestMustToSlice_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustToSlice should panic on invalid JSON")
		}
	}()

	MustToSlice("invalid")
}
