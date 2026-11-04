package reflectx

import (
	"math/big"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

type noopLocker struct{}

func (noopLocker) Lock()   {}
func (noopLocker) Unlock() {}

func TestDeepCopyCheckedRejectsNonZeroSynchronizationState(t *testing.T) {
	type locked struct {
		mu    sync.Mutex
		Value int
	}
	src := locked{Value: 7}
	src.mu.Lock()
	defer src.mu.Unlock()
	if _, err := DeepCopyChecked(&src); err == nil {
		t.Fatal("copied a locked sync.Mutex without an error")
	}

	var counter atomic.Int64
	counter.Store(1)
	if _, err := DeepCopyChecked(&counter); err == nil {
		t.Fatal("copied non-zero sync/atomic state without an error")
	}

	var mutex sync.Mutex
	mutex.Lock()
	defer mutex.Unlock()
	type withLocker struct{ Locker sync.Locker }
	if _, err := DeepCopyChecked(withLocker{Locker: &mutex}); err == nil {
		t.Fatal("sync.Locker backed by a locked mutex should be rejected")
	}
	value := reflect.ValueOf(&mutex).Elem().Interface()
	if _, err := DeepCopyChecked(value); err == nil {
		t.Fatal("copied top-level locked sync.Mutex value without an error")
	}
}

func TestDeepCopyCheckedAllowsZeroSynchronizationState(t *testing.T) {
	var zero sync.Mutex
	if _, err := DeepCopyChecked(&zero); err != nil {
		t.Fatalf("zero sync.Mutex should be copyable: %v", err)
	}
	value := reflect.ValueOf(&zero).Elem().Interface()
	if _, err := DeepCopyChecked(value); err != nil {
		t.Fatalf("top-level zero sync.Mutex value should be copyable: %v", err)
	}

	var zeroAtomic atomic.Int64
	if _, err := DeepCopyChecked(&zeroAtomic); err != nil {
		t.Fatalf("zero sync/atomic state should be copyable: %v", err)
	}

	type withLocker struct{ Locker sync.Locker }
	if _, err := DeepCopyChecked(withLocker{}); err != nil {
		t.Fatalf("nil sync.Locker should be copyable: %v", err)
	}
	if _, err := DeepCopyChecked(withLocker{Locker: noopLocker{}}); err != nil {
		t.Fatalf("custom safe sync.Locker should be copyable: %v", err)
	}
}

func TestDeepCopyCheckedRejectsNamedSynchronizationState(t *testing.T) {
	type namedAtomic atomic.Int64
	var counter namedAtomic
	(*atomic.Int64)(&counter).Store(1)
	if _, err := DeepCopyChecked(&counter); err == nil {
		t.Fatal("copied non-zero named atomic state without an error")
	}

	type namedMutex sync.Mutex
	var mutex namedMutex
	(*sync.Mutex)(&mutex).Lock()
	defer (*sync.Mutex)(&mutex).Unlock()
	if _, err := DeepCopyChecked(&mutex); err == nil {
		t.Fatal("copied locked named mutex without an error")
	}
}

func TestDeepCopyCheckedAllowsZeroNamedSynchronizationState(t *testing.T) {
	type namedAtomic atomic.Int64
	var counter namedAtomic
	if _, err := DeepCopyChecked(&counter); err != nil {
		t.Fatalf("zero named atomic should be copyable: %v", err)
	}

	type namedMutex sync.Mutex
	var mutex namedMutex
	if _, err := DeepCopyChecked(&mutex); err != nil {
		t.Fatalf("zero named mutex should be copyable: %v", err)
	}
}

func TestDeepCopyCheckedPreservesTimeZoneValue(t *testing.T) {
	when := time.Date(2026, time.August, 11, 12, 34, 56, 789, time.FixedZone("audit", 8*60*60))
	type record struct {
		When   time.Time
		hidden time.Time
	}
	input := record{When: when, hidden: when}
	got, err := DeepCopyChecked(input)
	if err != nil {
		t.Fatal(err)
	}
	if !got.When.Equal(when) || got.When.Location().String() != "audit" {
		t.Fatalf("exported time = %v (%s), want %v (audit)", got.When, got.When.Location(), when)
	}
	if !got.hidden.Equal(when) || got.hidden.Location().String() != "audit" {
		t.Fatalf("private time = %v (%s), want %v (audit)", got.hidden, got.hidden.Location(), when)
	}

	top, err := DeepCopyChecked(when)
	if err != nil || !top.Equal(when) || top.Location().String() != "audit" {
		t.Fatalf("top-level time = %v (%s), error = %v", top, top.Location(), err)
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
