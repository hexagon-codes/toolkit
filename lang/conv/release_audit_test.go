package conv

import (
	"encoding/json"
	"math"
	"testing"
)

type auditStringer struct{}

func (*auditStringer) String() string {
	panic("typed nil String method called")
}

type auditFloat32er struct{}

func (*auditFloat32er) Float32() float32 {
	panic("typed nil Float32 method called")
}

func TestConversionsTreatTypedNilAsNil(t *testing.T) {
	var stringer *auditStringer
	if got := String(stringer); got != "" {
		t.Fatalf("String(typed nil) = %q, want empty string", got)
	}

	var floater *auditFloat32er
	if got := Float32(floater); got != 0 {
		t.Fatalf("Float32(typed nil) = %v, want 0", got)
	}
}

func TestFloat32RejectsOutOfRangeText(t *testing.T) {
	if got := Float32("1e100"); got != 0 || math.IsInf(float64(got), 0) {
		t.Fatalf("Float32(1e100) = %v, want conversion failure zero", got)
	}
}

func TestJSONToMapPreservesNumberPrecisionAndRejectsTrailingValues(t *testing.T) {
	result, err := JSONToMap(`{"id":9007199254740993}`)
	if err != nil {
		t.Fatalf("JSONToMap() error = %v", err)
	}
	if got, ok := result["id"].(json.Number); !ok || got.String() != "9007199254740993" {
		t.Fatalf("decoded id = %#v, want exact json.Number", result["id"])
	}

	if _, err := JSONToMap(`{} {}`); err == nil {
		t.Fatal("JSONToMap accepted multiple JSON values")
	}
}
