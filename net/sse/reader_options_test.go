package sse

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// readAllEvents 读取 Reader 中的全部事件，返回事件切片与终止错误。
// 终止错误为 io.EOF 时视为正常结束（返回 nil 错误），便于断言"成功读完"的场景。
func readAllEvents(t *testing.T, r *Reader) ([]*Event, error) {
	t.Helper()
	var events []*Event
	for {
		ev, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return events, nil
			}
			return events, err
		}
		events = append(events, ev)
	}
}

// TestNewReaderWithOptions_NoOptions 验证不传任何选项时，
// NewReaderWithOptions 的行为与默认 NewReader 完全一致（向后兼容）。
func TestNewReaderWithOptions_NoOptions(t *testing.T) {
	input := "event: message\ndata: hello\n\ndata:world\n\n"

	r := NewReaderWithOptions(strings.NewReader(input))
	events, err := readAllEvents(t, r)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("期望读取 2 个事件，实际 %d", len(events))
	}
	// 第一个事件：event + data（带空格前缀，剥离一个空格）
	if events[0].Event != "message" || events[0].Data != "hello" {
		t.Errorf("事件0 解析错误: %+v", events[0])
	}
	// 第二个事件：宽松模式下 "data:world"（无空格）仍被识别
	if events[1].Data != "world" {
		t.Errorf("事件1 期望 data=world，实际 %q", events[1].Data)
	}
}

// TestWithStrictDataPrefix 表驱动覆盖严格 data 前缀模式：
// 仅 "data: "（含单空格）被识别，其余 data 形式被忽略。
func TestWithStrictDataPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantData string // 期望首个事件的 Data；空字符串配合 wantEvent 使用
		wantNone bool   // true 表示该输入不应产生任何非空事件（全部 data 行被忽略）
	}{
		{
			name:     "规范前缀-带单空格",
			input:    "data: hello\n\n",
			wantData: "hello",
		},
		{
			name:     "规范前缀-值内含多余空格逐字保留",
			input:    "data:  hello\n\n", // "data: " 之后是 " hello"
			wantData: " hello",
		},
		{
			name:     "无空格-被忽略",
			input:    "data:hello\n\n",
			wantNone: true,
		},
		{
			name:     "仅前缀无空格-被忽略",
			input:    "data:\n\n",
			wantNone: true,
		},
		{
			name:     "字段名非data-被忽略",
			input:    "datax: hello\n\n",
			wantNone: true,
		},
		{
			name:     "多行data严格拼接",
			input:    "data: line1\ndata: line2\n\n",
			wantData: "line1\nline2",
		},
		{
			name:     "混入无空格data行被跳过仅保留规范行",
			input:    "data: keep\ndata:drop\n\n",
			wantData: "keep",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReaderWithOptions(strings.NewReader(tt.input), WithStrictDataPrefix())
			events, err := readAllEvents(t, r)
			if err != nil {
				t.Fatalf("意外错误: %v", err)
			}

			if tt.wantNone {
				if len(events) != 0 {
					t.Fatalf("期望无事件（data 行均被忽略），实际 %d 个: %+v", len(events), events)
				}
				return
			}

			if len(events) == 0 {
				t.Fatalf("期望至少 1 个事件，实际 0 个")
			}
			if events[0].Data != tt.wantData {
				t.Errorf("Data 不符：期望 %q，实际 %q", tt.wantData, events[0].Data)
			}
		})
	}
}

// TestWithStrictDataPrefix_OtherFieldsUnaffected 验证严格模式只收紧 data 前缀，
// 不影响 event/id/retry/注释 等其它字段的解析。
func TestWithStrictDataPrefix_OtherFieldsUnaffected(t *testing.T) {
	input := ": comment line\nid: 42\nevent: update\nretry: 1500\ndata: payload\n\n"

	r := NewReaderWithOptions(strings.NewReader(input), WithStrictDataPrefix())
	ev, err := r.Read()
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if ev.ID != "42" {
		t.Errorf("期望 ID=42，实际 %q", ev.ID)
	}
	if ev.Event != "update" {
		t.Errorf("期望 Event=update，实际 %q", ev.Event)
	}
	if ev.Retry != 1500 {
		t.Errorf("期望 Retry=1500，实际 %d", ev.Retry)
	}
	if ev.Data != "payload" {
		t.Errorf("期望 Data=payload，实际 %q", ev.Data)
	}
}

// TestStrictVsLooseDivergence 直接对比同一输入在宽松/严格两种模式下的差异，
// 钉死两种模式的行为契约。
func TestStrictVsLooseDivergence(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		looseData string // 宽松模式期望首事件 Data
		looseSome bool   // 宽松模式是否应产生非空事件
		strictNone bool  // 严格模式是否应无事件
		strictData string // strictNone=false 时严格模式期望 Data
	}{
		{
			name:      "无空格data：宽松识别/严格忽略",
			input:     "data:{\"ok\":true}\n\n",
			looseData: `{"ok":true}`,
			looseSome: true,
			strictNone: true,
		},
		{
			name:       "带空格data：两模式一致",
			input:      "data: {\"ok\":true}\n\n",
			looseData:  `{"ok":true}`,
			looseSome:  true,
			strictNone: false,
			strictData: `{"ok":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 宽松
			loose := NewReader(strings.NewReader(tt.input))
			le, err := readAllEvents(t, loose)
			if err != nil {
				t.Fatalf("宽松模式意外错误: %v", err)
			}
			if tt.looseSome {
				if len(le) == 0 || le[0].Data != tt.looseData {
					t.Fatalf("宽松模式期望 Data=%q，实际 %+v", tt.looseData, le)
				}
			}

			// 严格
			strict := NewReaderWithOptions(strings.NewReader(tt.input), WithStrictDataPrefix())
			se, err := readAllEvents(t, strict)
			if err != nil {
				t.Fatalf("严格模式意外错误: %v", err)
			}
			if tt.strictNone {
				if len(se) != 0 {
					t.Fatalf("严格模式期望无事件，实际 %+v", se)
				}
			} else {
				if len(se) == 0 || se[0].Data != tt.strictData {
					t.Fatalf("严格模式期望 Data=%q，实际 %+v", tt.strictData, se)
				}
			}
		})
	}
}

// TestWithMaxTotalBytes 表驱动覆盖总字节上限：未超限正常读取、超限返回错误。
func TestWithMaxTotalBytes(t *testing.T) {
	// 单事件 "data: hello\n\n" 共 13 字节："data: hello\n"(12) + "\n"(1)。
	const oneEvent = "data: hello\n\n"

	tests := []struct {
		name        string
		input       string
		max         int64
		wantErr     error // 期望最终错误（nil 表示正常 EOF 结束）
		wantAtLeast int   // 至少应成功读取的事件数
	}{
		{
			name:        "上限为0不限制-正常读完",
			input:       strings.Repeat(oneEvent, 100),
			max:         0,
			wantErr:     nil,
			wantAtLeast: 100,
		},
		{
			name:        "上限充足-正常读完",
			input:       oneEvent,
			max:         1024,
			wantErr:     nil,
			wantAtLeast: 1,
		},
		{
			name:        "上限恰好等于单事件字节-不超限",
			input:       oneEvent, // 13 字节
			max:         int64(len(oneEvent)),
			wantErr:     nil,
			wantAtLeast: 1,
		},
		{
			name:        "上限略小于单事件-读取首事件即超限",
			input:       oneEvent,
			max:         int64(len(oneEvent)) - 1,
			wantErr:     ErrMaxBytesExceeded,
			wantAtLeast: 0,
		},
		{
			name:        "多事件累计超限-中途报错",
			input:       strings.Repeat(oneEvent, 10),
			max:         int64(len(oneEvent))*3 + 1, // 第 4 个事件读取过程中累计超限
			wantErr:     ErrMaxBytesExceeded,
			wantAtLeast: 3,
		},
		{
			name:        "上限为1-首行读取即超限",
			input:       oneEvent,
			max:         1,
			wantErr:     ErrMaxBytesExceeded,
			wantAtLeast: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReaderWithOptions(strings.NewReader(tt.input), WithMaxTotalBytes(tt.max))
			events, err := readAllEvents(t, r)

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("期望正常结束，实际错误: %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("期望错误 %v，实际 %v", tt.wantErr, err)
				}
			}
			if len(events) < tt.wantAtLeast {
				t.Fatalf("期望至少读取 %d 个事件，实际 %d", tt.wantAtLeast, len(events))
			}
		})
	}
}

// TestWithMaxTotalBytes_Negative 验证负数上限被归一为 0（不限制）。
func TestWithMaxTotalBytes_Negative(t *testing.T) {
	input := strings.Repeat("data: x\n\n", 50)
	r := NewReaderWithOptions(strings.NewReader(input), WithMaxTotalBytes(-1))
	events, err := readAllEvents(t, r)
	if err != nil {
		t.Fatalf("负数上限应等价于不限制，意外错误: %v", err)
	}
	if len(events) != 50 {
		t.Fatalf("期望 50 个事件，实际 %d", len(events))
	}
}

// TestWithMaxTotalBytes_Cumulative 验证字节计数在多次 Read 调用间累计，
// 即上限约束的是整个流的总字节，而非单次 Read。
func TestWithMaxTotalBytes_Cumulative(t *testing.T) {
	const oneEvent = "data: y\n\n" // 9 字节
	input := strings.Repeat(oneEvent, 5)

	// 上限设为 2.x 个事件的字节，确保第 3 个事件读取时累计超限。
	r := NewReaderWithOptions(strings.NewReader(input), WithMaxTotalBytes(int64(len(oneEvent))*2+1))

	// 前两次 Read 应成功
	for i := 0; i < 2; i++ {
		if _, err := r.Read(); err != nil {
			t.Fatalf("第 %d 次 Read 应成功，实际错误: %v", i+1, err)
		}
	}
	// 第三次 Read 累计超限
	if _, err := r.Read(); !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("第 3 次 Read 期望 ErrMaxBytesExceeded，实际 %v", err)
	}
}

// TestReaderOptions_Combined 验证两类选项可组合使用且互不干扰。
func TestReaderOptions_Combined(t *testing.T) {
	// 含一个规范 data 行、一个无空格 data 行（严格模式应忽略）。
	input := "data: ok\ndata:ignored\n\n"

	r := NewReaderWithOptions(
		strings.NewReader(input),
		WithStrictDataPrefix(),
		WithMaxTotalBytes(1024),
	)
	ev, err := r.Read()
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if ev.Data != "ok" {
		t.Errorf("期望 Data=ok（仅保留规范 data 行），实际 %q", ev.Data)
	}
}

// TestWithMaxTotalBytes_EOFNoTrailingNewline 验证无结尾换行的最后一行
// 也被计入字节统计，且在上限内可正常返回。
func TestWithMaxTotalBytes_EOFNoTrailingNewline(t *testing.T) {
	input := "data: tail" // 无结尾空行，EOF 时返回该事件
	r := NewReaderWithOptions(strings.NewReader(input), WithMaxTotalBytes(1024))
	ev, err := r.Read()
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if ev.Data != "tail" {
		t.Errorf("期望 Data=tail，实际 %q", ev.Data)
	}
}

// TestWithMaxTotalBytes_ExceedOnPartialLine 验证即使在 EOF 且无换行的场景下，
// 已读取的部分字节超限也会返回 ErrMaxBytesExceeded。
func TestWithMaxTotalBytes_ExceedOnPartialLine(t *testing.T) {
	input := "data: a very long single line without trailing newline"
	r := NewReaderWithOptions(strings.NewReader(input), WithMaxTotalBytes(5))
	if _, err := r.Read(); !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("期望 ErrMaxBytesExceeded，实际 %v", err)
	}
}
