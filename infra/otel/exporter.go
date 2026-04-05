package otel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Exporter 导出器接口
type Exporter interface {
	// ExportSpans 导出 Span 数据
	ExportSpans(ctx context.Context, spans []*SpanData) error

	// Shutdown 关闭导出器
	Shutdown(ctx context.Context) error
}

// ============== Console Exporter ==============

// ConsoleExporter 控制台导出器（用于开发调试）
type ConsoleExporter struct {
	// Writer 输出写入器
	Writer io.Writer

	// Pretty 是否格式化输出
	Pretty bool
}

// NewConsoleExporter 创建控制台导出器
func NewConsoleExporter(w io.Writer) *ConsoleExporter {
	return &ConsoleExporter{
		Writer: w,
		Pretty: true,
	}
}

// ExportSpans 导出 Span
func (e *ConsoleExporter) ExportSpans(ctx context.Context, spans []*SpanData) error {
	for _, span := range spans {
		var data []byte
		var err error

		if e.Pretty {
			data, err = json.MarshalIndent(span, "", "  ")
		} else {
			data, err = json.Marshal(span)
		}

		if err != nil {
			return err
		}

		fmt.Fprintf(e.Writer, "%s\n", data)
	}
	return nil
}

// Shutdown 关闭导出器
func (e *ConsoleExporter) Shutdown(ctx context.Context) error {
	return nil
}

// ============== OTLP Exporter ==============

// OTLPExporter OTLP 协议导出器
type OTLPExporter struct {
	// endpoint 导出端点
	endpoint string

	// headers 请求头
	headers map[string]string

	// client HTTP 客户端
	client *http.Client

	// Batch 批量配置
	batchSize    int
	batchTimeout time.Duration
	buffer       []*SpanData
	bufferMu     sync.Mutex

	// Shutdown
	done      chan struct{}
	closeOnce sync.Once
}

// OTLPExporterOption OTLP 导出器选项
type OTLPExporterOption func(*OTLPExporter)

// NewOTLPExporter 创建 OTLP 导出器
func NewOTLPExporter(endpoint string, opts ...OTLPExporterOption) *OTLPExporter {
	e := &OTLPExporter{
		endpoint:     endpoint,
		headers:      make(map[string]string),
		client:       &http.Client{Timeout: 30 * time.Second},
		batchSize:    512,
		batchTimeout: 5 * time.Second,
		buffer:       make([]*SpanData, 0),
		done:         make(chan struct{}),
	}

	for _, opt := range opts {
		opt(e)
	}

	// 启动批量导出
	go e.batchLoop()

	return e
}

// WithOTLPHeaders 设置请求头
func WithOTLPHeaders(headers map[string]string) OTLPExporterOption {
	return func(e *OTLPExporter) {
		for k, v := range headers {
			e.headers[k] = v
		}
	}
}

// WithOTLPBatchSize 设置批量大小
func WithOTLPBatchSize(size int) OTLPExporterOption {
	return func(e *OTLPExporter) {
		e.batchSize = size
	}
}

// ExportSpans 导出 Span
func (e *OTLPExporter) ExportSpans(ctx context.Context, spans []*SpanData) error {
	e.bufferMu.Lock()
	e.buffer = append(e.buffer, spans...)
	shouldFlush := len(e.buffer) >= e.batchSize
	e.bufferMu.Unlock()

	if shouldFlush {
		return e.flush(ctx)
	}

	return nil
}

// flush 刷新缓冲区
func (e *OTLPExporter) flush(ctx context.Context) error {
	e.bufferMu.Lock()
	if len(e.buffer) == 0 {
		e.bufferMu.Unlock()
		return nil
	}

	spans := e.buffer
	e.buffer = make([]*SpanData, 0)
	e.bufferMu.Unlock()

	return e.send(ctx, spans)
}

// send 发送数据
func (e *OTLPExporter) send(ctx context.Context, spans []*SpanData) error {
	// 转换为 OTLP 格式
	payload := e.toOTLPPayload(spans)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.endpoint+"/v1/traces", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("export failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// toOTLPPayload 转换为 OTLP 格式
func (e *OTLPExporter) toOTLPPayload(spans []*SpanData) map[string]any {
	resourceSpans := make([]map[string]any, 0)

	// 按服务分组
	serviceSpans := make(map[string][]*SpanData)
	for _, span := range spans {
		serviceName := "unknown"
		if name, ok := span.Attributes["service.name"].(string); ok {
			serviceName = name
		}
		serviceSpans[serviceName] = append(serviceSpans[serviceName], span)
	}

	for serviceName, svcSpans := range serviceSpans {
		scopeSpans := make([]map[string]any, 0)

		otlpSpans := make([]map[string]any, len(svcSpans))
		for i, span := range svcSpans {
			otlpSpans[i] = e.spanToOTLP(span)
		}

		scopeSpans = append(scopeSpans, map[string]any{
			"scope": map[string]any{
				"name":    "toolkit",
				"version": "1.0.0",
			},
			"spans": otlpSpans,
		})

		resourceSpans = append(resourceSpans, map[string]any{
			"resource": map[string]any{
				"attributes": []map[string]any{
					{"key": "service.name", "value": map[string]any{"stringValue": serviceName}},
				},
			},
			"scopeSpans": scopeSpans,
		})
	}

	return map[string]any{
		"resourceSpans": resourceSpans,
	}
}

// spanToOTLP 转换单个 Span
func (e *OTLPExporter) spanToOTLP(span *SpanData) map[string]any {
	attributes := make([]map[string]any, 0)
	for k, v := range span.Attributes {
		attributes = append(attributes, map[string]any{
			"key":   k,
			"value": e.valueToOTLP(v),
		})
	}

	events := make([]map[string]any, len(span.Events))
	for i, event := range span.Events {
		eventAttrs := make([]map[string]any, 0)
		for k, v := range event.Attributes {
			eventAttrs = append(eventAttrs, map[string]any{
				"key":   k,
				"value": e.valueToOTLP(v),
			})
		}
		events[i] = map[string]any{
			"timeUnixNano": event.Timestamp.UnixNano(),
			"name":         event.Name,
			"attributes":   eventAttrs,
		}
	}

	return map[string]any{
		"traceId":           span.TraceID,
		"spanId":            span.SpanID,
		"parentSpanId":      span.ParentSpanID,
		"name":              span.Name,
		"kind":              int(span.Kind) + 1, // OTLP kind starts from 1
		"startTimeUnixNano": span.StartTime.UnixNano(),
		"endTimeUnixNano":   span.EndTime.UnixNano(),
		"attributes":        attributes,
		"events":            events,
		"status": map[string]any{
			"code":    int(span.Status),
			"message": span.StatusMsg,
		},
	}
}

// valueToOTLP 转换值为 OTLP 格式
func (e *OTLPExporter) valueToOTLP(v any) map[string]any {
	switch val := v.(type) {
	case string:
		return map[string]any{"stringValue": val}
	case int:
		return map[string]any{"intValue": val}
	case int64:
		return map[string]any{"intValue": val}
	case float64:
		return map[string]any{"doubleValue": val}
	case bool:
		return map[string]any{"boolValue": val}
	default:
		return map[string]any{"stringValue": fmt.Sprintf("%v", val)}
	}
}

// batchLoop 批量导出循环
func (e *OTLPExporter) batchLoop() {
	ticker := time.NewTicker(e.batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-e.done:
			return
		case <-ticker.C:
			e.flush(context.Background())
		}
	}
}

// Shutdown 关闭导出器，使用 sync.Once 防止重复关闭 channel 导致 panic
func (e *OTLPExporter) Shutdown(ctx context.Context) error {
	e.closeOnce.Do(func() {
		close(e.done)
	})
	return e.flush(ctx)
}

// ============== Jaeger Exporter ==============

// JaegerExporter Jaeger 导出器
type JaegerExporter struct {
	endpoint string
	client   *http.Client
	username string
	password string
}

// NewJaegerExporter 创建 Jaeger 导出器
func NewJaegerExporter(endpoint string) *JaegerExporter {
	return &JaegerExporter{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// WithAuth 设置认证
func (e *JaegerExporter) WithAuth(username, password string) *JaegerExporter {
	e.username = username
	e.password = password
	return e
}

// ExportSpans 导出 Span
func (e *JaegerExporter) ExportSpans(ctx context.Context, spans []*SpanData) error {
	if len(spans) == 0 {
		return nil
	}

	payload := e.toJaegerPayload(spans)

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.endpoint+"/api/traces", bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if e.username != "" {
		req.SetBasicAuth(e.username, e.password)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("jaeger export failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// toJaegerPayload 转换为 Jaeger 格式
func (e *JaegerExporter) toJaegerPayload(spans []*SpanData) map[string]any {
	jaegerSpans := make([]map[string]any, len(spans))

	for i, span := range spans {
		tags := make([]map[string]any, 0)
		for k, v := range span.Attributes {
			tags = append(tags, map[string]any{
				"key":   k,
				"type":  e.getJaegerType(v),
				"value": v,
			})
		}

		logs := make([]map[string]any, len(span.Events))
		for j, event := range span.Events {
			fields := make([]map[string]any, 0)
			for k, v := range event.Attributes {
				fields = append(fields, map[string]any{
					"key":   k,
					"type":  e.getJaegerType(v),
					"value": v,
				})
			}
			logs[j] = map[string]any{
				"timestamp": event.Timestamp.UnixMicro(),
				"fields":    fields,
			}
		}

		jaegerSpans[i] = map[string]any{
			"traceID":       span.TraceID,
			"spanID":        span.SpanID,
			"parentSpanID":  span.ParentSpanID,
			"operationName": span.Name,
			"startTime":     span.StartTime.UnixMicro(),
			"duration":      span.EndTime.Sub(span.StartTime).Microseconds(),
			"tags":          tags,
			"logs":          logs,
		}
	}

	return map[string]any{
		"data": []map[string]any{
			{
				"traceID":   spans[0].TraceID,
				"spans":     jaegerSpans,
				"processes": map[string]any{},
			},
		},
	}
}

// getJaegerType 获取 Jaeger 类型
func (e *JaegerExporter) getJaegerType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case int, int32, int64:
		return "int64"
	case float32, float64:
		return "float64"
	case bool:
		return "bool"
	default:
		return "string"
	}
}

// Shutdown 关闭导出器
func (e *JaegerExporter) Shutdown(ctx context.Context) error {
	return nil
}

// ============== Zipkin Exporter ==============

// ZipkinExporter Zipkin 导出器
type ZipkinExporter struct {
	endpoint    string
	client      *http.Client
	serviceName string
}

// NewZipkinExporter 创建 Zipkin 导出器
func NewZipkinExporter(endpoint, serviceName string) *ZipkinExporter {
	return &ZipkinExporter{
		endpoint:    endpoint,
		client:      &http.Client{Timeout: 30 * time.Second},
		serviceName: serviceName,
	}
}

// ExportSpans 导出 Span
func (e *ZipkinExporter) ExportSpans(ctx context.Context, spans []*SpanData) error {
	zipkinSpans := make([]map[string]any, len(spans))

	for i, span := range spans {
		tags := make(map[string]string)
		for k, v := range span.Attributes {
			tags[k] = fmt.Sprintf("%v", v)
		}

		annotations := make([]map[string]any, len(span.Events))
		for j, event := range span.Events {
			annotations[j] = map[string]any{
				"timestamp": event.Timestamp.UnixMicro(),
				"value":     event.Name,
			}
		}

		zipkinSpans[i] = map[string]any{
			"traceId":     span.TraceID,
			"id":          span.SpanID,
			"parentId":    span.ParentSpanID,
			"name":        span.Name,
			"timestamp":   span.StartTime.UnixMicro(),
			"duration":    span.EndTime.Sub(span.StartTime).Microseconds(),
			"tags":        tags,
			"annotations": annotations,
			"localEndpoint": map[string]any{
				"serviceName": e.serviceName,
			},
		}
	}

	data, err := json.Marshal(zipkinSpans)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.endpoint+"/api/v2/spans", bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("zipkin export failed: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// Shutdown 关闭导出器
func (e *ZipkinExporter) Shutdown(ctx context.Context) error {
	return nil
}

// ============== Multi Exporter ==============

// MultiExporter 多导出器
type MultiExporter struct {
	exporters []Exporter
}

// NewMultiExporter 创建多导出器
func NewMultiExporter(exporters ...Exporter) *MultiExporter {
	return &MultiExporter{
		exporters: exporters,
	}
}

// ExportSpans 导出 Span
func (e *MultiExporter) ExportSpans(ctx context.Context, spans []*SpanData) error {
	var lastErr error
	for _, exporter := range e.exporters {
		if err := exporter.ExportSpans(ctx, spans); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Shutdown 关闭导出器
func (e *MultiExporter) Shutdown(ctx context.Context) error {
	var lastErr error
	for _, exporter := range e.exporters {
		if err := exporter.Shutdown(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
