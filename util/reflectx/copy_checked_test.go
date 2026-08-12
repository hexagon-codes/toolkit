package reflectx

import "testing"

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
