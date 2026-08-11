package reflectx

import (
	"reflect"
	"testing"
	"time"
)

func TestGetFieldRejectsUnexportedFieldWithoutPanic(t *testing.T) {
	type record struct {
		hidden string
	}

	if value, ok := GetField(record{hidden: "secret"}, "hidden"); ok || value != nil {
		t.Fatalf("GetField() = (%v, %v), want (nil, false)", value, ok)
	}
	if HasField(record{}, "hidden") {
		t.Fatal("HasField() exposed an unexported field")
	}
}

func TestDeepCopyPreservesOpaqueStructState(t *testing.T) {
	type record struct {
		Visible []int
		hidden  string
	}
	when := time.Date(2026, time.August, 11, 12, 34, 56, 789, time.FixedZone("audit", 8*60*60))
	original := struct {
		Record record
		When   time.Time
	}{
		Record: record{Visible: []int{1, 2, 3}, hidden: "preserve"},
		When:   when,
	}

	copied := DeepCopy(original)
	if copied.Record.hidden != original.Record.hidden {
		t.Errorf("unexported field = %q, want %q", copied.Record.hidden, original.Record.hidden)
	}
	if !copied.When.Equal(original.When) || copied.When.Location().String() != "audit" {
		t.Errorf("time value = %v, want %v", copied.When, original.When)
	}
	copied.Record.Visible[0] = 99
	if original.Record.Visible[0] == 99 {
		t.Fatal("exported slice still aliases the original")
	}
}

func TestDeepCopySeparatesVisitedValuesByTypeAndShape(t *testing.T) {
	original := struct {
		Ints    []int
		Strings []string
	}{
		Ints:    make([]int, 0),
		Strings: make([]string, 0),
	}

	copied := DeepCopy(original)
	if copied.Ints == nil || copied.Strings == nil {
		t.Fatalf("DeepCopy() changed non-nil empty slices: %#v", copied)
	}
}

func TestDeepCopyPreservesCycleIdentity(t *testing.T) {
	type node struct {
		Next *node
	}
	original := &node{}
	original.Next = original

	copied := DeepCopy(original)
	if copied == original {
		t.Fatal("DeepCopy() returned the original pointer")
	}
	if copied.Next != copied {
		t.Fatal("DeepCopy() did not preserve the self-reference")
	}
}

func TestMapToStructHandlesNilExplicitly(t *testing.T) {
	t.Run("nil clears nilable field", func(t *testing.T) {
		value := 42
		target := struct {
			Value *int
		}{Value: &value}
		if err := MapToStruct(map[string]any{"Value": nil}, &target); err != nil {
			t.Fatalf("MapToStruct() error = %v", err)
		}
		if target.Value != nil {
			t.Fatalf("Value = %v, want nil", *target.Value)
		}
	})

	t.Run("nil is rejected for scalar field", func(t *testing.T) {
		target := struct {
			Value int
		}{Value: 42}
		if err := MapToStruct(map[string]any{"Value": nil}, &target); err == nil {
			t.Fatal("MapToStruct() accepted nil for an int field")
		}
	})
}

func TestStructReadersHandleTypedNilPointers(t *testing.T) {
	type record struct {
		Value int
	}
	var input *record

	if got := StructToMap(input); got != nil {
		t.Fatalf("StructToMap() = %#v, want nil", got)
	}
	if value, ok := GetField(input, "Value"); ok || value != nil {
		t.Fatalf("GetField() = (%v, %v), want (nil, false)", value, ok)
	}
	if HasField(input, "Value") {
		t.Fatal("HasField() = true for a typed nil pointer")
	}
	if names := FieldNames(input); names != nil {
		t.Fatalf("FieldNames() = %#v, want nil", names)
	}
	if tags := FieldTags(input, "json"); tags != nil {
		t.Fatalf("FieldTags() = %#v, want nil", tags)
	}
}

func TestDeepCopyDoesNotChangeArrayShape(t *testing.T) {
	original := [2][]int{{1, 2}, {3, 4}}
	copied := DeepCopy(original)
	if !reflect.DeepEqual(copied, original) {
		t.Fatalf("DeepCopy() = %#v, want %#v", copied, original)
	}
}
