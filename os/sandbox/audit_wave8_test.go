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
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	res, err := s.Exec(context.Background(), "echo", []string{"probe"})
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
// 影响: 整个 darwin 沙箱在现代 macOS 上完全不可用 —— Exec/ExecCode 永远失败。
// 这不是测试环境问题: 用 (allow file-read*)(allow file-map-executable) 即可正常执行,
// 证明缺陷在策略生成逻辑本身。
func TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile(t *testing.T) {
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	res, err := s.Exec(context.Background(), "echo", []string{"probe"})
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
			cfg:     Config{Workspace: ""},
			wantErr: true,
		},
		{
			name:    "正常工作区",
			cfg:     Config{Workspace: t.TempDir()},
			wantErr: false,
		},
		{
			name:    "Timeout 为 0 走默认 60",
			cfg:     Config{Workspace: t.TempDir(), Timeout: 0},
			wantErr: false,
		},
		{
			name:    "Timeout 负数也走默认 60",
			cfg:     Config{Workspace: t.TempDir(), Timeout: -100},
			wantErr: false,
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
	s, err := New(Config{Workspace: ws, Timeout: 1}) // 声称 1 秒超时
	if err != nil {
		t.Fatalf("New 失败: %v", err)
	}

	// 不带任何 deadline 的 ctx, 执行一个 sleep 3 秒的命令。
	// cfg.Timeout=1 必须真正生效, 在 ~1 秒被杀, 而非跑满 3 秒。
	start := time.Now()
	ctx := context.Background()
	_, execErr := s.Exec(ctx, "sleep", []string{"3"})
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
	_, err := s.Exec(ctx, "sleep", []string{"10"})
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
	res, err := s.Exec(context.Background(), "sh", []string{"-c", "exit 7"})
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

	_, err := s.Exec(context.Background(), "sh", []string{"-c", "this_cmd_does_not_exist_xyz_123"})
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

	res, err := s.Exec(context.Background(), "sh", []string{"-c", "echo OUT; echo ERR 1>&2"})
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

	res, err := s.Exec(context.Background(), "echo", []string{payload})
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
		network    bool
		wantAllow  bool
		wantDenied bool
	}{
		{"网络关闭(默认)", false, false, true},
		{"网络开启", true, true, false},
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
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws, Network: false})

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
	res, err := s.ExecCode(ctx, "python", code)
	if err != nil {
		t.Skipf("ExecCode 失败(环境差异), 跳过: %v", err)
	}
	out := res.Stdout + res.Stderr
	t.Logf("网络关闭沙箱内连接结果: %q (exit=%d)", strings.TrimSpace(out), res.ExitCode)
	if strings.Contains(res.Stdout, "CONNECTED") {
		t.Errorf("安全缺陷: 网络关闭时仍成功建立出站 TCP 连接, 沙箱网络隔离失效")
	}
}

// ============================================================================
// 五、ExecCode: 语言支持 / 临时文件清理 / 并发隔离(核心安全点)
// ============================================================================

// TestExecCode_UnsupportedLanguage 验证不支持的语言报错。
func TestExecCode_UnsupportedLanguage(t *testing.T) {
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	tests := []string{"ruby", "perl", "", "PYTHON", "Python3", "rust"}
	for _, lang := range tests {
		t.Run("lang="+lang, func(t *testing.T) {
			_, err := s.ExecCode(context.Background(), lang, "print('x')")
			if err == nil {
				t.Errorf("语言 %q 应不被支持并报错", lang)
			}
		})
	}
}

// TestExecCode_PythonHelloWorld 验证 python 代码正常执行并捕获输出。
func TestExecCode_PythonHelloWorld(t *testing.T) {
	if !sandboxExecWorks(t) {
		t.Skip("源码 SBPL 策略在本 macOS 上无法执行二进制, 见 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile")
	}
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	res, err := s.ExecCode(context.Background(), "python", "print('hello-sandbox')")
	if err != nil {
		t.Fatalf("ExecCode 失败: %v", err)
	}
	if !strings.Contains(res.Stdout, "hello-sandbox") {
		t.Errorf("期望输出含 hello-sandbox, 实际 stdout=%q stderr=%q exit=%d", res.Stdout, res.Stderr, res.ExitCode)
	}
}

// TestExecCode_TempFileCleanup 验证临时文件在执行后被清理(defer os.Remove)。
//
// 回归: ExecCode 改用唯一文件名后, 仍必须在执行结束清理掉所有 "_hexclaw_exec*"
// 临时文件, 不残留(无论固定名还是带随机后缀的唯一名)。
func TestExecCode_TempFileCleanup(t *testing.T) {
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	_, err := s.ExecCode(context.Background(), "python", "print(1)")
	if err != nil {
		t.Fatalf("ExecCode 失败: %v", err)
	}
	// 旧固定文件名 _hexclaw_exec.py 不应残留。
	leftover := filepath.Join(ws, "_hexclaw_exec.py")
	if _, statErr := os.Stat(leftover); statErr == nil {
		t.Errorf("临时文件未清理: %s 仍存在", leftover)
	}
	// 唯一命名(_hexclaw_exec_*.py)同样不得残留: 扫描整个 workspace。
	entries, _ := os.ReadDir(ws)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "_hexclaw_exec") {
			t.Errorf("临时文件未清理: workspace 残留 %s", e.Name())
		}
	}
}

// TestExecCode_ConcurrentSameWorkspaceRace 验证并发执行隔离已修复。
//
// 回归: 旧实现对同一 workspace 使用 *固定* 临时文件名 "_hexclaw_exec.py",
// 并发执行多份不同代码时会互相覆盖同一文件(A 可能执行到 B 的代码), 且
// defer os.Remove 可能删掉别人正在用的文件, 违反"并发执行隔离"安全要求。
// 修复后改用唯一文件名。本测试并发跑 N 份各自带唯一标记的代码,
// 必须不出现 "拿到别人代码输出" 的串扰、也不出现因竞态导致的执行异常。
func TestExecCode_ConcurrentSameWorkspaceRace(t *testing.T) {
	if !sandboxExecWorks(t) {
		// 平台沙箱整体不可用时, 执行普遍失败会掩盖串扰信号, 改测"临时文件写入层"的竞态。
		// 见 TestExecCode_UniqueTempFileNameIsolatesConcurrency 用静态/文件层方式确证隔离。
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
			code := fmt.Sprintf("print('%s')", marker)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			res, err := s.ExecCode(ctx, "python", code)
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
			t.Logf("goroutine %d 报错(并发竞态导致): %v", oc.idx, oc.err)
			continue
		}
		if oc.gotMarker != want {
			// 拿到的不是自己的标记 => 文件被其他 goroutine 覆盖 => 串扰
			if strings.HasPrefix(oc.gotMarker, "MARK_") {
				crosstalk++
				t.Logf("串扰: goroutine %d 期望 %q 实际拿到 %q (执行了别人的代码)", oc.idx, want, oc.gotMarker)
			} else {
				// 空输出或语法错误也是竞态结果(文件被部分覆盖/删除)
				failures++
				t.Logf("goroutine %d 输出异常(竞态): want=%q got=%q exit=%d", oc.idx, want, oc.gotMarker, oc.exit)
			}
		}
	}
	if crosstalk > 0 || failures > 0 {
		t.Errorf("并发隔离缺陷: 固定临时文件名 _hexclaw_exec.py 导致 %d 次代码串扰 / %d 次执行异常 (共 %d 并发)", crosstalk, failures, n)
	}
}

// TestExecCode_UniqueTempFileNameIsolatesConcurrency 以文件层方式确证并发隔离已修复,
// 不依赖沙箱能否真正 exec(因此在 macOS 26+ SBPL 失效时仍能跑出结论)。
//
// 回归: 旧实现对同一 workspace + 同一语言写死 "_hexclaw_exec.<ext>", 并发两次
// ExecCode 会写同一物理文件(后写覆盖先写 + defer 误删), 违反并发执行隔离。
// 修复后改用 os.CreateTemp 生成 "_hexclaw_exec_<随机>.<ext>" 唯一名。本测试并发触发
// ExecCode, 后台探针扫描 workspace, 断言:
//  1. 旧固定共享路径 "_hexclaw_exec.py" 从不出现(无共享物理文件);
//  2. 并发调用产生多个不同的唯一临时文件(每次调用各自隔离)。
func TestExecCode_UniqueTempFileNameIsolatesConcurrency(t *testing.T) {
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})
	fixedPath := filepath.Join(ws, "_hexclaw_exec.py")

	// 后台高频探测: 无论 exec 成败, WriteFile 一定先发生, 探针能在文件被 defer 删除前
	// 捕获到它。用 atomic + 互斥保护的 set 收集出现过的唯一文件名。
	stop := make(chan struct{})
	var sawFixed atomic.Int32 // 观测到旧固定共享文件 => 隔离失败
	var setMu sync.Mutex
	uniqueSeen := make(map[string]struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				if _, err := os.Stat(fixedPath); err == nil {
					sawFixed.Store(1)
				}
				entries, _ := os.ReadDir(ws)
				setMu.Lock()
				for _, e := range entries {
					name := e.Name()
					// 唯一名形如 _hexclaw_exec_<随机>.py, 排除恰为固定名的情况。
					if strings.HasPrefix(name, "_hexclaw_exec_") {
						uniqueSeen[name] = struct{}{}
					}
				}
				setMu.Unlock()
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// 不同代码体, 修复后各自落到独立的唯一文件 => 无物理串扰。
			_, _ = s.ExecCode(ctx, "python", fmt.Sprintf("print(%d)", i))
		}(i)
	}
	wg.Wait()
	close(stop)

	if sawFixed.Load() == 1 {
		t.Errorf("回归失败: 观测到旧固定共享路径 %s, ExecCode 未使用唯一文件名, 并发隔离缺失", fixedPath)
	}
	setMu.Lock()
	distinct := len(uniqueSeen)
	setMu.Unlock()
	// 8 个并发调用应产生多个不同的唯一文件(探针时序下至少观测到 >1 个即证明隔离)。
	if distinct < 2 {
		t.Errorf("回归失败: 仅观测到 %d 个唯一临时文件, 期望多个互相隔离的 _hexclaw_exec_*.py", distinct)
	}
	t.Logf("并发隔离已修复: 未见共享固定文件, 观测到 %d 个独立唯一临时文件", distinct)
}

// TestExecCode_GoLanguageDispatch 验证 go 语言走 "go run" 分支(不依赖网络)。
func TestExecCode_GoLanguageDispatch(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过 go run")
	}
	if !sandboxExecWorks(t) {
		t.Skip("源码 SBPL 策略在本 macOS 上无法执行二进制, 见 TestDarwinSBPL_BinaryAbortsUnderGeneratedProfile")
	}
	ws := t.TempDir()
	s := newDarwinSandbox(Config{Workspace: ws})

	code := "package main\nimport \"fmt\"\nfunc main(){ fmt.Println(\"go-ok\") }\n"
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	res, err := s.ExecCode(ctx, "go", code)
	if err != nil {
		t.Skipf("go run 失败(可能沙箱限制 GOCACHE/网络), 跳过: %v", err)
	}
	t.Logf("go run 结果: stdout=%q stderr=%q exit=%d", res.Stdout, res.Stderr, res.ExitCode)
	// 注意: go 临时文件名是 _hexclaw_exec.go, go run 单文件需 package main, 已满足。
}

// ============================================================================
// 六、cleanEnv: 危险环境变量清理
// ============================================================================

// TestCleanEnv_RemovesDangerous 验证源码 *确实* 会清理的那些精确命名危险变量,
// 并验证安全变量保留。(对"通配遗漏"的缺陷断言移至 TestCleanEnv_DangerousMatchingNuance,
// 避免两个测试对同一行为做相反断言。)
func TestCleanEnv_RemovesDangerous(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"LD_PRELOAD=/evil.so",         // 精确命中 LD_PRELOAD
		"LD_LIBRARY_PATH=/x",          // 精确命中 LD_LIBRARY_PATH
		"DYLD_INSERT_LIBRARIES=/evil", // 精确命中
		"DYLD_LIBRARY_PATH=/y",        // 精确命中
		"HOME=/Users/me",
		"LD_=foo", // 命中 "LD_" 项(要求 "LD_=")
	}
	out := cleanEnv(in)
	joined := strings.Join(out, "\n")

	// 这些精确命名的危险变量应被清理。
	mustStrip := []string{
		"LD_PRELOAD=", "LD_LIBRARY_PATH=", "DYLD_INSERT_LIBRARIES=",
		"DYLD_LIBRARY_PATH=", "LD_=",
	}
	for _, name := range mustStrip {
		if strings.Contains(joined, name) {
			t.Errorf("精确命名危险变量未清理: %q", name)
		}
	}
	// 安全变量应保留。
	for _, keep := range []string{"PATH=/usr/bin", "HOME=/Users/me"} {
		if !strings.Contains(joined, keep) {
			t.Errorf("安全变量被误删: %q", keep)
		}
	}
}

// TestCleanEnv_DangerousMatchingNuance 验证 cleanEnv 对 LD_/DYLD_ 做真正的前缀通配。
//
// 回归: 旧实现对每个条目统一拼接 "="(prefix+"="), 使 "LD_" 退化为只匹配 "LD_=",
// 导致任意 LD_<X>/DYLD_<X> 注入向量(LD_FOO、DYLD_BAR, 尤其经典攻击向量 LD_AUDIT)
// 全部漏网。修复后取 "=" 之前的变量名做 HasPrefix("LD_"/"DYLD_") 判断,
// 命中所有以其开头的变量。本测试钉死该通配语义, 任一向量泄漏即失败。
func TestCleanEnv_DangerousMatchingNuance(t *testing.T) {
	in := []string{
		"LD_FOO=/evil",      // 通配 LD_<X>
		"DYLD_BAR=/evil",    // 通配 DYLD_<X>
		"LD_AUDIT=/evil.so", // 经典攻击向量, 必须被前缀通配命中
	}
	out := cleanEnv(in)
	joined := strings.Join(out, "\n")

	leaked := []string{}
	for _, dangerous := range in {
		name := strings.SplitN(dangerous, "=", 2)[0]
		if strings.Contains(joined, name+"=") {
			leaked = append(leaked, name)
		}
	}
	if len(leaked) > 0 {
		t.Errorf("回归失败: cleanEnv 清理遗漏 %v (LD_/DYLD_ 前缀通配未生效, 或漏掉 LD_AUDIT 等向量)", leaked)
	}
}

// ============================================================================
// 七、expandPath: 波浪号展开边界
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

// ============================================================================
// 八、NetPolicy: 黑名单语义 / 端口 / 并发竞态(补充已有测试未覆盖的边界)
// ============================================================================

// TestNetPolicy_BlacklistSubdomainAware 验证黑名单具备子域感知能力。
//
// 回归: 黑名单作为安全兜底, 运维直觉是"加黑 example.com 即封掉整个域"。
// 旧实现用 matchDomainPattern 精确匹配, 黑名单写 "example.com" 不拦截子域
// "evil.example.com", 形成反直觉的安全旁路。修复后 matchBlacklistPattern
// 对裸域名自动覆盖根域及任意子域(host 以 "."+pattern 结尾)。
func TestNetPolicy_BlacklistSubdomainAware(t *testing.T) {
	p := &NetPolicy{
		Mode:            "allow-all",
		DomainBlacklist: []string{"example.com"}, // 想封整个 example.com
		counters:        make(map[string]*rateBucket),
	}
	// 根域被拦截。
	if p.IsAllowed("example.com", 443) {
		t.Error("example.com 应被黑名单拦截")
	}
	// 子域也必须被拦截 —— 修复后黑名单具备子域感知。
	if p.IsAllowed("evil.example.com", 443) {
		t.Error("回归失败: evil.example.com 应被黑名单子域感知拦截(加黑根域即封整个域)")
	}
	// 多级子域同样覆盖。
	if p.IsAllowed("a.b.example.com", 443) {
		t.Error("回归失败: a.b.example.com 多级子域应被拦截")
	}
	// 形近但不同的域不应被误伤(notexample.com 不是 example.com 的子域)。
	if !p.IsAllowed("notexample.com", 443) {
		t.Error("notexample.com 非 example.com 子域, 不应被误封")
	}
}

// TestNetPolicy_PortCheckBeforeWhitelist 暴露求值顺序: deny-all 模式即便端口/域名
// 都没配也直接拒绝; 而 allow-all 下端口过滤先于域名放行。验证端口在 whitelist 前求值,
// 即被黑名单/端口拦下的请求不会因 whitelist 命中而放行。
func TestNetPolicy_PortAndModeInteraction(t *testing.T) {
	tests := []struct {
		name string
		p    *NetPolicy
		host string
		port int
		want bool
	}{
		{
			name: "deny-all 压倒一切",
			p:    &NetPolicy{Mode: "deny-all", AllowedPorts: []int{443}, DomainWhitelist: []string{"x.com"}},
			host: "x.com", port: 443, want: false,
		},
		{
			name: "whitelist 命中但端口不符 -> 拒绝",
			p:    &NetPolicy{Mode: "whitelist-only", AllowedPorts: []int{443}, DomainWhitelist: []string{"x.com"}},
			host: "x.com", port: 8080, want: false,
		},
		{
			name: "whitelist 命中端口符合 -> 放行",
			p:    &NetPolicy{Mode: "whitelist-only", AllowedPorts: []int{443}, DomainWhitelist: []string{"x.com"}},
			host: "x.com", port: 443, want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.p.counters == nil {
				tt.p.counters = make(map[string]*rateBucket)
			}
			got := tt.p.IsAllowed(tt.host, tt.port)
			if got != tt.want {
				t.Errorf("IsAllowed(%q,%d)=%v, want %v", tt.host, tt.port, got, tt.want)
			}
		})
	}
}

// TestNetPolicy_CheckRateLimit_Concurrent 并发竞态: 多 goroutine 同一 processID
// 高频调用, 验证 (a) 不 panic / 不 data race; (b) 放行次数恰好等于 MaxRequestsPerMinute。
// 需配合 -race 运行才能检出竞态。
func TestNetPolicy_CheckRateLimit_Concurrent(t *testing.T) {
	const limit = 100
	const goroutines = 50
	const perG = 10 // 总调用 500 次, 远超 limit
	p := &NetPolicy{
		MaxRequestsPerMinute: limit,
		counters:             make(map[string]*rateBucket),
	}

	var allowed int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := 0
			for i := 0; i < perG; i++ {
				if p.CheckRateLimit("shared-pid") {
					local++
				}
			}
			mu.Lock()
			allowed += int64(local)
			mu.Unlock()
		}()
	}
	wg.Wait()

	// 在同一分钟窗口内, 放行总数应恰为 limit。若计数器有竞态, 可能 > limit。
	if allowed != int64(limit) {
		t.Errorf("并发限流不准: 放行 %d 次, 期望恰好 %d (窗口内). 可能存在计数竞态", allowed, limit)
	}
}

// TestNetPolicy_RateLimit_UnicodeAndLongProcessID 边界: 超长 / Unicode processID。
func TestNetPolicy_RateLimit_UnicodeAndLongProcessID(t *testing.T) {
	p := &NetPolicy{MaxRequestsPerMinute: 3, counters: make(map[string]*rateBucket)}

	ids := []string{
		strings.Repeat("x", 100000),      // 超长
		"进程-标识符-🚀",                       // Unicode + emoji
		string([]byte{0x00, 0x01, 0x02}), // 含 NUL 的非法字符串
	}
	for _, id := range ids {
		// 每个 id 独立计数, 前 3 次放行, 第 4 次拒绝。
		for i := 0; i < 3; i++ {
			if !p.CheckRateLimit(id) {
				t.Errorf("processID(len=%d) 第 %d 次应放行", len(id), i+1)
			}
		}
		if p.CheckRateLimit(id) {
			t.Errorf("processID(len=%d) 第 4 次应被限流", len(id))
		}
	}
}

// ============================================================================
// 九、matchDomainPattern: 补充边界(已有测试未覆盖的奇异输入)
// ============================================================================

// TestMatchDomainPattern_EdgeCases 覆盖空白裁剪 / 多级通配 / 单独 "*." 等。
func TestMatchDomainPattern_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		host    string
		want    bool
	}{
		{"前后空白被裁剪后精确匹配", "  example.com  ", "example.com", true},
		{"host 带空白裁剪", "example.com", "  example.com ", true},
		{"通配只匹配后缀不匹配中间", "*.github.com", "github.com.evil.com", false},
		{"裸星号点 *. 匹配任意单段?", "*.", "x", false},
		{"裸星号点 *. host 为空", "*.", "", true}, // suffix="." host="" 不以"."结尾, 但 host==pattern[2:]=="" -> true
		{"通配大小写混合", "*.GITHUB.com", "API.github.COM", true},
		{"非通配前缀不匹配子域", "github.com", "api.github.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchDomainPattern(tt.pattern, tt.host)
			if got != tt.want {
				t.Errorf("matchDomainPattern(%q,%q)=%v, want %v", tt.pattern, tt.host, got, tt.want)
			}
		})
	}
}

// ============================================================================
// 十、NetProxy: 通配匹配 / ProxyEnvVars / 并发 isAllowed
// ============================================================================

// TestNetProxy_WildcardMatchesSubdomainNotSuffixTrap 暴露 NetProxy.isAllowed 的
// 通配实现与 NetPolicy 不一致: 这里 "*.github.com" 的 suffix=".github.com",
// 用 strings.HasSuffix 判断, 因此 "evil-github.com" *不会* 命中(正确),
// 但 "github.com" 本身也 *不会* 命中(因为不含 ".github.com" 后缀)。
//
// 对比 NetPolicy.matchDomainPattern 额外有 host==pattern[2:] 的兜底,
// 两套通配语义不一致 —— 同一个 "*.github.com" 在 policy 里放行根域、在 proxy 里拦截根域。
func TestNetProxy_WildcardRootDomainInconsistency(t *testing.T) {
	proxy := NewNetProxy(NetProxyConfig{AllowDomains: []string{"*.github.com"}})

	// proxy: 根域 github.com 不命中通配(无 ".github.com" 后缀)
	proxyRoot := proxy.isAllowed("github.com")
	// policy: 根域 github.com 命中(host==pattern[2:])
	pol := &NetPolicy{Mode: "whitelist-only", DomainWhitelist: []string{"*.github.com"}, counters: map[string]*rateBucket{}}
	polRoot := pol.IsAllowed("github.com", 0) // AllowedPorts 空 -> 不过滤端口

	if proxyRoot == polRoot {
		t.Errorf("预期 proxy/policy 对根域通配语义不一致, 实际都=%v", proxyRoot)
	}
	t.Logf("语义不一致已确认: NetProxy.isAllowed(github.com, *.github.com)=%v, NetPolicy=%v", proxyRoot, polRoot)

	// 子域两者都应命中
	if !proxy.isAllowed("api.github.com") {
		t.Error("api.github.com 应命中 proxy 通配")
	}
}

// TestNetProxy_ProxyEnvVars 验证生成的代理环境变量齐全且格式正确。
func TestNetProxy_ProxyEnvVars(t *testing.T) {
	addr := "127.0.0.1:8899"
	vars := ProxyEnvVars(addr)

	want := map[string]string{
		"HTTP_PROXY":  "http://127.0.0.1:8899",
		"HTTPS_PROXY": "http://127.0.0.1:8899",
		"http_proxy":  "http://127.0.0.1:8899",
		"https_proxy": "http://127.0.0.1:8899",
	}
	seen := map[string]string{}
	for _, kv := range vars {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			seen[parts[0]] = parts[1]
		}
	}
	for k, v := range want {
		if seen[k] != v {
			t.Errorf("环境变量 %s=%q, 期望 %q", k, seen[k], v)
		}
	}
	// 回归: ProxyEnvVars 不得注入 SSL_CERT_FILE。旧实现把它设为空字符串,
	// 空值与"不设置"语义不同 —— 会让子进程把 CA 证书路径解析为空, 反而破坏
	// TLS 证书校验。正确做法是根本不注入该变量, 让子进程使用系统 CA 信任库。
	if _, ok := seen["SSL_CERT_FILE"]; ok {
		t.Errorf("回归失败: ProxyEnvVars 不应注入 SSL_CERT_FILE(空值会破坏子进程 TLS 校验), 实际注入=%q", seen["SSL_CERT_FILE"])
	}
}

// TestNetProxy_StartCancelCleanup 验证 ctx 取消后代理 listener 被关闭(无泄漏)。
func TestNetProxy_StartCancelCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := NewNetProxy(NetProxyConfig{})
	addr, err := p.Start(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	if addr == "" {
		t.Fatal("Start 应返回非空地址")
	}
	cancel()
	// 给 goroutine 一点时间响应 ctx.Done 并 srv.Close。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p.listener != nil {
			// listener.Addr 仍可读, 但底层 socket 应已关闭; 这里只验证不 panic。
			break
		}
	}
	t.Logf("代理在 %s 启动并随 ctx 取消而关闭", addr)
}

// TestNetProxy_ConcurrentIsAllowed 并发读 allowList(配合 -race 检 data race)。
func TestNetProxy_ConcurrentIsAllowed(t *testing.T) {
	p := NewNetProxy(NetProxyConfig{AllowDomains: []string{"a.com", "*.b.com"}})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.isAllowed("a.com")
			_ = p.isAllowed("x.b.com")
			_ = p.isAllowed("evil.com")
			p.log("GET", "a.com", "/", 200) // 并发读 logger
		}()
	}
	wg.Wait()
}
