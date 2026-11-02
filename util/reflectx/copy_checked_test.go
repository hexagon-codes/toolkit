package reflectx

import (
	"math/big"
	"testing"
)

func TestDeepCopyBigIntIsolation(t *testing.T) {
	original, ok := new(big.Int).SetString("123456789012345678901234567890", 10)
	if !ok {
		t.Fatal("invalid fixture")
	}
	want := original.String()
	input := map[string]any{"number": original, "alias": original}
	for name, clone := range map[string]func(map[string]any) (map[string]any, error){
		"checked":  DeepCopyChecked[map[string]any],
		"ordinary": func(value map[string]any) (map[string]any, error) { return DeepCopy(value), nil },
	} {
		t.Run(name, func(t *testing.T) {
			copied, err := clone(input)
			if err != nil {
				t.Fatal(err)
			}
			number := copied["number"].(*big.Int)
			if number == original || number != copied["alias"].(*big.Int) || number.String() != want {
				t.Fatal("integer value or alias identity was not preserved")
			}
			number.SetInt64(7)
			if original.String() != want {
				t.Fatal("copied integer changed original digits")
			}
		})
	}
	private := struct{ hidden big.Int }{hidden: *original}
	if _, err := DeepCopyChecked(private); err == nil {
		t.Fatal("private mutable integer must not be accepted as an immutable value")
	}
}

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
