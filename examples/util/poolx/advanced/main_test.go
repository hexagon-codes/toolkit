package main

import (
	"reflect"
	"testing"
)

func TestPriorityQueueOrdersConfiguredTasks(t *testing.T) {
	got, err := priorityExecutionOrder()
	if err != nil {
		t.Fatalf("priorityExecutionOrder() error = %v", err)
	}
	want := []int{4, 3, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("priorityExecutionOrder() = %v, want %v", got, want)
	}
}
