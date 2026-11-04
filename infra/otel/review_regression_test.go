package otel

import "testing"

func TestJaegerPayloadGroupsSpansByTrace(t *testing.T) {
	exporter := &JaegerExporter{}
	payload := exporter.toJaegerPayload([]*SpanData{
		{TraceID: "trace-a", SpanID: "span-a1", Name: "a1"},
		{TraceID: "trace-b", SpanID: "span-b1", Name: "b1"},
		{TraceID: "trace-a", SpanID: "span-a2", Name: "a2"},
	})

	data, ok := payload["data"].([]map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want []map[string]any", payload["data"])
	}
	if len(data) != 2 {
		t.Fatalf("data entries = %d, want 2", len(data))
	}

	byTrace := make(map[string]int)
	for _, entry := range data {
		traceID, _ := entry["traceID"].(string)
		spans, _ := entry["spans"].([]map[string]any)
		byTrace[traceID] = len(spans)
	}
	if byTrace["trace-a"] != 2 || byTrace["trace-b"] != 1 {
		t.Fatalf("grouped spans = %#v, want trace-a:2 trace-b:1", byTrace)
	}
}
