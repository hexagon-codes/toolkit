package sse

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// claudeMessageStop 模拟 Anthropic Claude 的流结束判定：event 类型为 message_stop。
func claudeMessageStop(e *Event) bool {
	return e.Event == "message_stop"
}

// geminiFinishReason 模拟 Google Gemini 的流结束判定：data 中携带非空 finishReason。
func geminiFinishReason(e *Event) bool {
	var chunk struct {
		FinishReason string `json:"finishReason"`
	}
	if err := e.JSON(&chunk); err != nil {
		return false
	}
	return chunk.FinishReason != ""
}

// TestWithDoneFunc_ProviderAgnostic 表驱动覆盖 provider 无关的 done 谓词：
// 同一套 Each/ReadUntilDone 机制下，不同上游通过注入各自的 done 判定函数收敛。
func TestWithDoneFunc_ProviderAgnostic(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		doneFn     func(*Event) bool
		wantEvents int    // Each 回调到的事件总数（含命中 done 的那个事件）
		wantLast   string // 最后一个回调事件的 Data（用于确认 done 事件未被吞掉）
	}{
		{
			name:       "OpenAI-[DONE]哨兵",
			input:      "data: {\"i\":1}\n\ndata: {\"i\":2}\n\ndata: [DONE]\n\ndata: {\"i\":3}\n\n",
			doneFn:     IsOpenAIDone,
			wantEvents: 3, // 1、2、[DONE]；[DONE] 之后的 {"i":3} 不再迭代
			wantLast:   "[DONE]",
		},
		{
			name:       "Claude-message_stop事件",
			input:      "event: message_start\ndata: {}\n\nevent: content_block_delta\ndata: {\"t\":\"hi\"}\n\nevent: message_stop\ndata: {}\n\nevent: trailing\ndata: {}\n\n",
			doneFn:     claudeMessageStop,
			wantEvents: 3, // message_start、content_block_delta、message_stop
			wantLast:   "{}",
		},
		{
			name:       "Gemini-finishReason非空",
			input:      "data: {\"finishReason\":\"\",\"text\":\"a\"}\n\ndata: {\"finishReason\":\"STOP\",\"text\":\"b\"}\n\ndata: {\"text\":\"c\"}\n\n",
			doneFn:     geminiFinishReason,
			wantEvents: 2, // 第二个事件 finishReason=STOP 即结束，第三个不再读
			wantLast:   `{"finishReason":"STOP","text":"b"}`,
		},
		{
			name:       "无done命中-读到EOF",
			input:      "data: a\n\ndata: b\n\n",
			doneFn:     IsOpenAIDone,
			wantEvents: 2,
			wantLast:   "b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReaderWithOptions(strings.NewReader(tt.input), WithDoneFunc(tt.doneFn))

			var got []*Event
			err := r.Each(func(ev *Event) error {
				got = append(got, ev)
				return nil
			})
			if err != nil {
				t.Fatalf("Each 意外错误: %v", err)
			}
			if len(got) != tt.wantEvents {
				t.Fatalf("期望回调 %d 个事件，实际 %d: %+v", tt.wantEvents, len(got), got)
			}
			if got[len(got)-1].Data != tt.wantLast {
				t.Errorf("最后一个事件 Data 期望 %q，实际 %q", tt.wantLast, got[len(got)-1].Data)
			}
		})
	}
}

// TestReadUntilDone 表驱动逐事件验证 ReadUntilDone 的 (event, done, err) 三元返回。
func TestReadUntilDone(t *testing.T) {
	type step struct {
		wantData string
		wantDone bool
		wantErr  error // nil 表示成功；非 nil 用 errors.Is 断言
	}
	tests := []struct {
		name   string
		input  string
		doneFn func(*Event) bool
		steps  []step
	}{
		{
			name:   "OpenAI流-命中DONE后EOF",
			input:  "data: x\n\ndata: [DONE]\n\n",
			doneFn: IsOpenAIDone,
			steps: []step{
				{wantData: "x", wantDone: false},
				{wantData: "[DONE]", wantDone: true},
				{wantErr: io.EOF},
			},
		},
		{
			name:   "未配置谓词-done恒为false",
			input:  "data: [DONE]\n\n",
			doneFn: nil, // 不注入谓词
			steps: []step{
				{wantData: "[DONE]", wantDone: false},
				{wantErr: io.EOF},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []ReaderOption
			if tt.doneFn != nil {
				opts = append(opts, WithDoneFunc(tt.doneFn))
			}
			r := NewReaderWithOptions(strings.NewReader(tt.input), opts...)

			for i, s := range tt.steps {
				ev, done, err := r.ReadUntilDone()
				if s.wantErr != nil {
					if !errors.Is(err, s.wantErr) {
						t.Fatalf("步骤 %d 期望错误 %v，实际 %v", i, s.wantErr, err)
					}
					if done {
						t.Errorf("步骤 %d 出错时 done 应为 false", i)
					}
					continue
				}
				if err != nil {
					t.Fatalf("步骤 %d 意外错误: %v", i, err)
				}
				if ev.Data != s.wantData {
					t.Errorf("步骤 %d 期望 Data=%q，实际 %q", i, s.wantData, ev.Data)
				}
				if done != s.wantDone {
					t.Errorf("步骤 %d 期望 done=%v，实际 %v", i, s.wantDone, done)
				}
			}
		})
	}
}

// TestEach_HandlerError 验证 handler 返回错误时 Each 立即中止并原样返回该错误。
func TestEach_HandlerError(t *testing.T) {
	input := "data: a\n\ndata: b\n\ndata: c\n\n"
	r := NewReaderWithOptions(strings.NewReader(input), WithDoneFunc(IsOpenAIDone))

	sentinel := errors.New("handler stop")
	var seen int
	err := r.Each(func(ev *Event) error {
		seen++
		if ev.Data == "b" {
			return sentinel
		}
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("期望 handler 错误透传，实际 %v", err)
	}
	if seen != 2 {
		t.Errorf("期望在第 2 个事件处中止（共回调 2 次），实际 %d", seen)
	}
}

// TestEach_NoDoneFunc_ReadsToEOF 验证未配置 done 谓词时 Each 退化为读到 EOF。
func TestEach_NoDoneFunc_ReadsToEOF(t *testing.T) {
	input := "data: a\n\ndata: b\n\ndata: c\n\n"
	r := NewReaderWithOptions(strings.NewReader(input)) // 无 done 谓词

	var got []string
	if err := r.Each(func(ev *Event) error {
		got = append(got, ev.Data)
		return nil
	}); err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if strings.Join(got, ",") != "a,b,c" {
		t.Errorf("期望读完全部事件 a,b,c，实际 %v", got)
	}
}

// TestWithDoneFunc_NilEquivalent 验证 WithDoneFunc(nil) 等价于未配置谓词。
func TestWithDoneFunc_NilEquivalent(t *testing.T) {
	input := "data: [DONE]\n\ndata: after\n\n"
	r := NewReaderWithOptions(strings.NewReader(input), WithDoneFunc(nil))

	var got []string
	if err := r.Each(func(ev *Event) error {
		got = append(got, ev.Data)
		return nil
	}); err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	// nil 谓词不应触发结束，[DONE] 仅作为普通事件，after 也应被读到。
	if strings.Join(got, ",") != "[DONE],after" {
		t.Errorf("期望 nil 谓词不结束流，实际 %v", got)
	}
}

// TestWithDoneFunc_CombinedWithOtherOptions 验证 done 谓词可与既有选项组合，互不干扰。
func TestWithDoneFunc_CombinedWithOtherOptions(t *testing.T) {
	// 严格 data 前缀 + 总字节上限 + done 谓词三选项组合。
	// "data:drop"（无空格）在严格模式下被忽略；命中 [DONE] 结束。
	input := "data: keep\ndata:drop\n\ndata: [DONE]\n\n"
	r := NewReaderWithOptions(
		strings.NewReader(input),
		WithStrictDataPrefix(),
		WithMaxTotalBytes(1024),
		WithDoneFunc(IsOpenAIDone),
	)

	var got []*Event
	if err := r.Each(func(ev *Event) error {
		got = append(got, ev)
		return nil
	}); err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 个事件（keep 事件 + [DONE]），实际 %d: %+v", len(got), got)
	}
	if got[0].Data != "keep" {
		t.Errorf("首事件期望 Data=keep（严格模式忽略 drop），实际 %q", got[0].Data)
	}
	if !IsOpenAIDone(got[1]) {
		t.Errorf("末事件应为 [DONE]，实际 %q", got[1].Data)
	}
}

// TestErrMaxBytesExceeded_Message 验证错误文案含描述性措辞且错误身份不变。
func TestErrMaxBytesExceeded_Message(t *testing.T) {
	// 文案应包含描述性短语，便于下游对错误信息做断言或日志归类。
	if !strings.Contains(ErrMaxBytesExceeded.Error(), "exceeded maximum total bytes") {
		t.Errorf("ErrMaxBytesExceeded 文案应含 'exceeded maximum total bytes'，实际 %q", ErrMaxBytesExceeded.Error())
	}

	// 错误身份不变：实际触发上限时仍可用 errors.Is 判定。
	r := NewReaderWithOptions(strings.NewReader("data: hello\n\n"), WithMaxTotalBytes(1))
	_, err := r.Read()
	if !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("期望 errors.Is(err, ErrMaxBytesExceeded)，实际 %v", err)
	}
}
