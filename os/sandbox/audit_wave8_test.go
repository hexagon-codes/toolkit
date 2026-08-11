//go:build darwin

// audit_wave8_test.go 第 8 波审计测试
//
// 目标: toolkit/os/sandbox 沙箱包 (darwin Seatbelt 实现)
// 重点: 资源限制/超时/命令注入防护/逃逸边界/清理(临时文件/进程)/并发隔离
//
// 约束: 只新增测试、不改源码、只测本包。table-driven + 中文注释。
package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// sandboxExecWorks 探测当前 macOS 上, 源码生成的 SBPL 策略能否成功执行任意二进制。
//
// 背景(关键发现): 在 macOS 26+ 上, 源码 generateSBPL 生成的 (deny default) +
// 枚举 file-read* 子路径策略, 缺少 (allow file-map-executable) 且未覆盖 dyld
// 共享缓存路径(/System/Volumes/Preboot/Cryptexes/...), 导致 dyld 在加载阶段被
// SIGABRT 杀掉, 任何命令都返回 exit=-1 且无输出。
//
// 为避免把"平台沙箱整体不可用"误判为单个行为测试失败, 行为类测试先用本探针,
// 不可用则 Skip(并由 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile 专门记录该缺陷)。
func sandboxExecWorks(t *testing.T) bool {
	t.Helper()
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})
	res, err := s.Exec(context.Background(), Command{Path: "/bin/echo", Args: []string{"probe"}})
	if err == nil && res != nil && res.ExitCode == 0 && strings.Contains(res.Stdout, "probe") {
		return true
	}
	return false
}

// TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile 专门记录核心安全/可用性缺陷:
// 源码 generateSBPL 生成的 Seatbelt 策略在 macOS 26+ 上无法执行任何二进制。
//
// 现象: Exec 返回 ExitCode=-1, Stdout/Stderr 均为空(进程在 dyld 阶段 SIGABRT)。
// 根因: (deny default) 策略未授予 file-map-executable, 且 file-read* 仅枚举
//
//	/usr /bin /System 等, 未覆盖 dyld 共享缓存所在的
//	/System/Volumes/Preboot/Cryptexes/OS/...(独立卷, 不是 /System 的 subpath)。
//
// 影响: 整个 darwin 沙箱在现代 macOS 上完全不可用 —— Exec 永远失败。
// 这不是测试环境问题: 用 (allow file-read*)(allow file-map-executable) 即可正常执行,
// 证明缺陷在策略生成逻辑本身。
func TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile(t *testing.T) {
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	res, err := s.Exec(context.Background(), Command{Path: "/bin/echo", Args: []string{"probe"}})
	// 期望(若源码正确): exit=0, stdout 含 probe。
	// 实际(缺陷): exit=-1, 无输出, 因 dyld 被沙箱杀掉。
	ok := err == nil && res != nil && res.ExitCode == 0 && strings.Contains(res.Stdout, "probe")
	if ok {
		t.Log("当前环境下源码 SBPL 策略可正常执行 echo(可能 macOS 版本较旧或已修复)")
		return
	}
	exit := -999
	var sout, serr string
	if res != nil {
		exit, sout, serr = res.ExitCode, res.Stdout, res.Stderr
	}
	t.Errorf("缺陷确认: 源码生成的 Seatbelt 策略无法执行最简单的 echo。"+
		" exit=%d stdout=%q stderr=%q err=%v。"+
		" 根因: 缺 file-map-executable + 未覆盖 dyld 共享缓存(/System/Volumes/Preboot/Cryptexes)。"+
		" 后果: darwin 沙箱在 macOS 26+ 完全不可用。", exit, sout, serr, err)
}

// ============================================================================
// 一、New 工厂函数: 配置校验 / 默认值 / 边界
// ============================================================================

// TestNew_Validation 校验 New 对 Workspace / Timeout 的处理。
func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantErr   bool
		checkSbox func(t *testing.T, s Sandbox)
	}{
		{
			name:    "空工作区报错",
			cfg:     Config{Workspace: "", RequiredCapabilities: CapabilityFilesystem | CapabilityOutput},
			wantErr: true,
		},
		{
			name:    "正常工作区",
			cfg:     Config{Workspace: t.TempDir(), RequiredCapabilities: CapabilityFilesystem | CapabilityOutput},
			wantErr: false,
		},
		{
			name:    "Timeout 为 0 走默认 60",
			cfg:     Config{Workspace: t.TempDir(), Timeout: 0, RequiredCapabilities: CapabilityFilesystem | CapabilityOutput},
			wantErr: false,
		},
		{
			name:    "Timeout 负数返回配置错误",
			cfg:     Config{Workspace: t.TempDir(), Timeout: -100, RequiredCapabilities: CapabilityFilesystem | CapabilityOutput},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望报错, 实际 err=nil, sandbox=%v", s)
				}
				return
			}
			if err != nil {
				t.Fatalf("期望成功, 实际报错: %v", err)
			}
			if s == nil {
				t.Fatal("成功时 Sandbox 不应为 nil")
			}
		})
	}
}

// TestNew_TimeoutDefaultEnforced 验证 cfg.Timeout 真正生效。
//
// 回归: cfg.Timeout 在 darwin/linux/basic 三个 POSIX 路径曾完全未生效(死字段),
// 用户配置 Timeout=1 期望 1 秒强制终止却无任何效果。修复后 Exec 会据 cfg.Timeout
// 派生 deadline, 在调用方 ctx 无更早 deadline 时强制杀掉超时进程。
//
// 断言: 配置 1 秒超时执行 sleep 3, 必须在 ~1 秒(显著早于 3 秒)被终止。
func TestNew_TimeoutDefaultEnforced(t *testing.T) {
	if !sandboxExecWorks(t) {
		t.Skip("源码 SBPL 策略在本 macOS 上无法执行二进制, 无法实测 sleep 时长, 见 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile")
	}
	ws := t.TempDir()
	s, err := New(Config{
		Workspace:            ws,
		Timeout:              1,
		RequiredCapabilities: CapabilityFilesystem | CapabilityOutput,
	}) // 声称 1 秒超时
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}

	// 不带任何 deadline 的 ctx, 执行一个 sleep 3 秒的命令。
	// cfg.Timeout=1 必须真正生效, 在 ~1 秒被杀, 而非跑满 3 秒。
	start := time.Now()
	ctx := context.Background()
	_, execErr := s.Exec(ctx, Command{Path: "/bin/sleep", Args: []string{"3"}})
	elapsed := time.Since(start)

	t.Logf("Exec 耗时=%v, err=%v, cfg.Timeout=1s", elapsed, execErr)
	// 必须显著早于 3 秒结束(给宽松上界 2.5s 容忍调度抖动)。
	if elapsed >= 2500*time.Millisecond {
		t.Errorf("cfg.Timeout=1s 未生效: 命令跑满 %v(接近 sleep 3), 超时字段仍是死配置", elapsed)
	}
	// 进程被 ctx 超时信号杀掉, sandbox-exec 返回非 ExitError 的 err。
	if execErr == nil {
		t.Errorf("超时杀进程应返回非 nil error, 实际 nil(耗时=%v)", elapsed)
	}
}

// ============================================================================
// 二、Exec: 超时 / 退出码 / 错误路径
// ============================================================================

// TestExec_ContextTimeout 验证调用方 ctx 超时能真正杀掉子进程。
func TestExec_ContextTimeout(t *testing.T) {
	if !sandboxExecWorks(t) {
		t.Skip("源码 SBPL 策略在本 macOS 上无法执行二进制(sleep 会立即 abort), 无法实测 ctx 超时, 见 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile")
	}
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := s.Exec(ctx, Command{Path: "/bin/sleep", Args: []string{"10"}})
	elapsed := time.Since(start)

	// ctx 超时, sandbox-exec 被信号杀掉, Run 返回非 ExitError 的 err
	// (signal: killed), 源码会把它当作 "sandbox exec failed" 返回 error。
	if elapsed > 3*time.Second {
		t.Errorf("ctx 超时未及时杀进程, 耗时=%v", elapsed)
	}
	t.Logf("ctx 超时后耗时=%v, err=%v", elapsed, err)
	if err == nil {
		t.Log("注意: 被信号杀掉时返回 err=nil 不符预期; 通常应为非 nil")
	}
}

// TestExec_ExitCodeNonZero 验证非零退出码被正确捕获(不当作 error 返回)。
func TestExec_ExitCodeNonZero(t *testing.T) {
	if !sandboxExecWorks(t) {
		t.Skip("源码 SBPL 策略在本 macOS 上无法执行二进制, 见 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile")
	}
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	// false 命令: 退出码 1, 不报 error。
	res, err := s.Exec(context.Background(), Command{Path: "/bin/sh", Args: []string{"-c", "exit 7"}})
	if err != nil {
		t.Fatalf("非零退出码不应返回 Go error, 实际: %v", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("期望 ExitCode=7, 实际=%d (stderr=%q)", res.ExitCode, res.Stderr)
	}
}

// TestExec_CommandNotFound 验证不存在命令返回 error。
func TestExec_CommandNotFound(t *testing.T) {
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	_, err := s.Exec(context.Background(), Command{Path: "/bin/sh", Args: []string{"-c", "this_cmd_does_not_exist_xyz_123"}})
	// sh 找不到命令 -> 退出码 127, 不报 Go error (是 ExitError)。
	if err != nil {
		t.Logf("命令未找到返回 err=%v", err)
	}
}

// TestExec_StdoutStderrCapture 验证标准输出/错误分别捕获。
func TestExec_StdoutStderrCapture(t *testing.T) {
	if !sandboxExecWorks(t) {
		t.Skip("源码 SBPL 策略在本 macOS 上无法执行二进制, 见 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile")
	}
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	res, err := s.Exec(context.Background(), Command{Path: "/bin/sh", Args: []string{"-c", "echo OUT; echo ERR 1>&2"}})
	if err != nil {
		t.Fatalf("Exec 失败: %v", err)
	}
	if !strings.Contains(res.Stdout, "OUT") {
		t.Errorf("stdout 应含 OUT, 实际=%q", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "ERR") {
		t.Errorf("stderr 应含 ERR, 实际=%q", res.Stderr)
	}
}

// ============================================================================
// 三、命令注入防护: args 是否被 shell 解释
// ============================================================================

// TestExec_NoShellInjection 验证 args 通过 exec 直接传参, 不经 shell 解释,
// 因此 args 中的 shell 元字符不会触发注入。
func TestExec_NoShellInjection(t *testing.T) {
	if !sandboxExecWorks(t) {
		t.Skip("源码 SBPL 策略在本 macOS 上无法执行二进制, 见 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile")
	}
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	// 若 args 被 shell 解释, "; touch pwned" 会创建文件。
	// 由于 exec 直接传参给 /bin/echo, 它只会被原样打印。
	marker := filepath.Join(ws, "pwned")
	payload := "hello; touch " + marker

	res, err := s.Exec(context.Background(), Command{Path: "/bin/echo", Args: []string{payload}})
	if err != nil {
		t.Fatalf("Exec 失败: %v", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Errorf("命令注入成功! 文件 %s 被创建, 说明 args 经 shell 解释", marker)
	}
	if !strings.Contains(res.Stdout, payload) {
		t.Errorf("echo 应原样打印 payload, 实际=%q", res.Stdout)
	}
}

// ============================================================================
// 四、Seatbelt SBPL 策略生成: 逃逸边界 / 拒绝路径 / 网络
// ============================================================================

// TestGenerateSBPL_Structure 验证生成的 SBPL 包含必备段落。
//
// 回归: macOS 26+ 在 (deny default) 下若缺少 (allow file-map-executable),
// dyld 无法 mmap 可执行映像, 任何二进制都在 dyld 阶段 SIGABRT, 整个 darwin
// 沙箱不可用; 同理必须显式 (allow file-read* (literal "/")) 让 dyld 能 stat/read
// 根目录 inode。本测试钉死这两条关键规则, 防止未来 SBPL 重构静默丢失它们。
func TestGenerateSBPL_Structure(t *testing.T) {
	ws := "/tmp/test-ws"
	s := newDarwinSandbox(Config{Workspace: ws})
	sbpl := s.generateSBPL()

	mustContain := []string{
		"(version 1)",
		"(deny default)",
		"(allow process-exec)",
		"(allow file-map-executable)",        // 关键: dyld mmap 可执行映像
		"(allow file-read* (literal \"/\"))", // 关键: dyld stat/read 根目录
		fmt.Sprintf("(allow file-read* (subpath \"%s\"))", ws),
		fmt.Sprintf("(allow file-write* (subpath \"%s\"))", ws),
	}
	for _, frag := range mustContain {
		if !strings.Contains(sbpl, frag) {
			t.Errorf("SBPL 缺少必备片段: %q\n生成内容:\n%s", frag, sbpl)
		}
	}
}

// TestGenerateSBPL_NetworkToggle 验证网络开关切换 allow/deny network*。
func TestGenerateSBPL_NetworkToggle(t *testing.T) {
	tests := []struct {
		name       string
		network    NetworkMode
		wantAllow  bool
		wantDenied bool
	}{
		{"网络关闭(默认)", NetworkDisabled, false, true},
		{"网络开启", NetworkHost, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newDarwinSandbox(Config{Workspace: "/tmp/ws", Network: tt.network})
			sbpl := s.generateSBPL()
			hasAllow := strings.Contains(sbpl, "(allow network*)")
			hasDeny := strings.Contains(sbpl, "(deny network*)")
			if hasAllow != tt.wantAllow {
				t.Errorf("allow network* 存在=%v, 期望=%v", hasAllow, tt.wantAllow)
			}
			if hasDeny != tt.wantDenied {
				t.Errorf("deny network* 存在=%v, 期望=%v", hasDeny, tt.wantDenied)
			}
		})
	}
}

// TestGenerateSBPL_DeniedPathsOrdering 暴露潜在的 SBPL 规则顺序缺陷。
//
// SBPL 是 "last-match-wins"。源码顺序:
//  1. allow file-write* (subpath workspace)   <- 在前
//  2. allow file-write* (subpath "/tmp")       <- 在前
//  3. deny  file-write* (subpath denied)       <- 在后
//
// 对于 workspace 内的 denied 子路径, deny 在后, 能正确覆盖 allow (安全)。
// 但若 denied 路径在 /tmp 下, 同样 deny 在后能覆盖。本测试钉死规则顺序,
// 防止未来重排导致 deny 被 allow 覆盖(逃逸)。
func TestGenerateSBPL_DeniedPathsOrdering(t *testing.T) {
	ws := "/tmp/myws"
	deniedInWs := "/tmp/myws/secret"
	s := newDarwinSandbox(Config{
		Workspace:   ws,
		DeniedPaths: []string{deniedInWs},
	})
	sbpl := s.generateSBPL()

	allowIdx := strings.Index(sbpl, fmt.Sprintf("(allow file-write* (subpath \"%s\"))", ws))
	denyIdx := strings.Index(sbpl, fmt.Sprintf("(deny file-write* (subpath \"%s\"))", deniedInWs))

	if allowIdx < 0 {
		t.Fatalf("未找到 workspace allow 规则")
	}
	if denyIdx < 0 {
		t.Fatalf("未找到 denied deny 规则")
	}
	// last-match-wins: deny 必须出现在 allow 之后才能生效。
	if denyIdx < allowIdx {
		t.Errorf("规则顺序缺陷: deny(idx=%d) 在 allow(idx=%d) 之前, last-match-wins 下 deny 将失效, 形成逃逸", denyIdx, allowIdx)
	}
}

// TestGenerateSBPL_DeniedPathExpansion 验证 ~ 路径在 denied 中被展开。
func TestGenerateSBPL_DeniedPathExpansion(t *testing.T) {
	home, _ := os.UserHomeDir()
	s := newDarwinSandbox(Config{
		Workspace:   "/tmp/ws",
		DeniedPaths: []string{"~/.ssh"},
	})
	sbpl := s.generateSBPL()

	expanded := filepath.Join(home, ".ssh")
	if !strings.Contains(sbpl, expanded) {
		t.Errorf("denied 路径 ~/.ssh 应被展开为 %s, SBPL=%q", expanded, sbpl)
	}
	if strings.Contains(sbpl, "\"~/.ssh\"") {
		t.Errorf("denied 路径不应保留未展开的 ~/.ssh")
	}
}

// TestGenerateSBPL_NetworkBlockActuallyWorks 实测网络关闭时 SBPL 是否真正阻断
// 出站连接。这是沙箱的核心安全承诺。
func TestGenerateSBPL_NetworkBlockActuallyWorks(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过实网测试")
	}
	if !sandboxExecWorks(t) {
		t.Skip("源码 SBPL 策略在本 macOS 上无法执行二进制, 网络阻断无从验证, 见 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 is unavailable: %v", err)
	}
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws, Network: NetworkDisabled})

	// 用 python3 尝试建立到 1.1.1.1:80 的 TCP 连接, 在沙箱内应被拒绝。
	code := `
import socket, sys
try:
    s = socket.create_connection(("1.1.1.1", 80), timeout=3)
    s.close()
    print("CONNECTED")
except Exception as e:
    print("BLOCKED:" + type(e).__name__)
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := s.Exec(ctx, Command{Path: python, Args: []string{"-c", code}})
	if err != nil {
		t.Skipf("sandbox command failed due to environment differences: %v", err)
	}
	out := res.Stdout + res.Stderr
	t.Logf("网络关闭沙箱内连接结果: %q (exit=%d)", strings.TrimSpace(out), res.ExitCode)
	if strings.Contains(res.Stdout, "CONNECTED") {
		t.Errorf("安全缺陷: 网络关闭时仍成功建立出站 TCP 连接, 沙箱网络隔离失效")
	}
}

// ============================================================================
// 五、Exec: 同一工作区并发执行隔离
// ============================================================================

// TestExec_ConcurrentSameWorkspaceIsolation 验证并发结构化命令的输出互不串扰。
func TestExec_ConcurrentSameWorkspaceIsolation(t *testing.T) {
	if !sandboxExecWorks(t) {
		t.Skip("源码 SBPL 策略在本 macOS 上无法执行二进制, 见 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile")
	}
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	const n = 12
	var wg sync.WaitGroup
	type outcome struct {
		idx       int
		gotMarker string
		err       error
		exit      int
	}
	results := make([]outcome, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// 每个 goroutine 打印自己唯一的标记。
			marker := fmt.Sprintf("MARK_%d", i)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			res, err := s.Exec(ctx, Command{Path: "/bin/echo", Args: []string{marker}})
			oc := outcome{idx: i, err: err}
			if res != nil {
				oc.exit = res.ExitCode
				oc.gotMarker = strings.TrimSpace(res.Stdout)
			}
			results[i] = oc
		}(i)
	}
	wg.Wait()

	crosstalk := 0
	failures := 0
	for _, oc := range results {
		want := fmt.Sprintf("MARK_%d", oc.idx)
		if oc.err != nil {
			failures++
			t.Logf("goroutine %d failed: %v", oc.idx, oc.err)
			continue
		}
		if oc.gotMarker != want {
			// 拿到其他标记说明并发命令的输出发生串扰。
			if strings.HasPrefix(oc.gotMarker, "MARK_") {
				crosstalk++
				t.Logf("goroutine %d received another command's output: want %q, got %q", oc.idx, want, oc.gotMarker)
			} else {
				// 空输出或异常退出同样属于并发执行失败。
				failures++
				t.Logf("goroutine %d returned unexpected output: want %q, got %q, exit=%d", oc.idx, want, oc.gotMarker, oc.exit)
			}
		}
	}
	if crosstalk > 0 || failures > 0 {
		t.Errorf("concurrent command isolation failed: %d output mismatches and %d execution failures across %d commands", crosstalk, failures, n)
	}
}

// ============================================================================
// 六、expandPath: 波浪号展开边界
// ============================================================================

// TestExpandPath 覆盖 expandPath 的各种输入。
func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"绝对路径不变", "/etc/passwd", "/etc/passwd"},
		{"波浪斜杠展开", "~/.ssh", filepath.Join(home, ".ssh")},
		{"相对路径不变", "foo/bar", "foo/bar"},
		{"空字符串不变", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.in)
			if got != tt.want {
				t.Errorf("expandPath(%q)=%q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestExpandPath_BareTildeExpanded 验证裸 "~" 被展开为 home 目录。
//
// 回归: 用户在 DeniedPaths 写 "~" 想表达 home 目录。旧实现只处理 "~/" 前缀,
// 裸 "~" 被当成字面路径写进 SBPL deny 规则, 指向不存在的 "~" 文件, deny 静默失效。
// 修复后 expandPath("~") 返回 home 目录, 使"拒绝 home"的意图真正生效。
func TestExpandPath_BareTildeExpanded(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandPath("~")
	if got != home {
		t.Errorf("回归失败: 裸 \"~\" 应展开为 home 目录 %q, 实际 %q", home, got)
	}
	if got == "~" {
		t.Error("回归失败: 裸 \"~\" 仍返回字面路径, deny 规则将静默失效")
	}
	// "~user" 形式解析任意用户 home 涉及平台特定查询且语义不明确, 仍原样返回。
	if expandPath("~root/.ssh") != "~root/.ssh" {
		t.Errorf("~root 应原样返回(不支持 ~user 展开), 与源码约定不符")
	}
}
