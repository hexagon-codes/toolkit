package reflectx

import "testing"

type checkedPointerNode struct {
	Value int
	Self  checkedNamedPointer
}

type checkedNamedPointer *checkedPointerNode

func TestDeepCopyCheckedPreservesDefinedPointers(t *testing.T) {
	original := checkedNamedPointer(&checkedPointerNode{Value: 7})
	original.Self = original
	copied, err := DeepCopyChecked(original)
	if err != nil || copied == nil || copied == original || copied.Value != 7 || copied.Self != copied {
		t.Fatalf("defined pointer copy failed: copy=%p error=%v", copied, err)
	}
	copied.Value = 8
	if original.Value != 7 {
		t.Fatal("defined pointer retained the original object")
	}
	unchecked := DeepCopy(original)
	if unchecked == nil || unchecked.Value != 7 || unchecked.Self != unchecked {
		t.Fatal("shared copy implementation lost the defined pointer")
	}
	input := map[string]any{"pointer": original, "nil": checkedNamedPointer(nil)}
	result, err := DeepCopyChecked(input)
	if err != nil {
		t.Fatal(err)
	}
	if pointer, ok := result["pointer"].(checkedNamedPointer); !ok || pointer == nil || pointer == original || pointer.Self != pointer {
		t.Fatal("interface copy lost the defined pointer or its cycle")
	}
	if pointer, ok := result["nil"].(checkedNamedPointer); !ok || pointer != nil {
		t.Fatal("interface copy lost the typed nil pointer")
	}
	var nilValue any
	if result, err := DeepCopyChecked(nilValue); result != nil || err != nil {
		t.Fatalf("nil interface copy=%v error=%v", result, err)
	}
}

func TestDeepCopyCheckedPreservesTypesAndCycles(t *testing.T) {
	type item struct{ Values []int }
	value := &item{Values: []int{7}}
	input := map[string]any{"typed": []int{1, 2}, "object": value}
	input["self"] = input
	got, err := DeepCopyChecked(input)
	if err != nil {
		t.Fatal(err)
	}
	got["typed"].([]int)[0] = 9
	got["object"].(*item).Values[0] = 11
	if input["typed"].([]int)[0] != 1 || value.Values[0] != 7 {
		t.Fatal("copy retained mutable references to source")
	}
	got["self"].(map[string]any)["cycle"] = true
	if got["cycle"] != true || input["cycle"] != nil {
		t.Fatal("cycle was not copied into the new object graph")
	}
}

func TestDeepCopyCheckedRejectsSharedResources(t *testing.T) {
	type private struct{ values []int }
	for name, value := range map[string]any{
		"channel":           make(chan int),
		"function":          func() {},
		"private_reference": private{values: []int{1}},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := DeepCopyChecked(value); err == nil || got != nil {
				t.Fatalf("copy = %v, error = %v; want explicit failure", got, err)
			}
		})
	}
}
