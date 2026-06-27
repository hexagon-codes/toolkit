//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// BUG-20260626：用户经数据连接器授权了某本地目录，但 code_exec 的 macOS seatbelt 沙箱
// deny-default 只放行 Workspace + 系统目录，授权目录读不到（os.path.exists → False）。
// 修复：Config.ReadablePaths 让 darwin profile 为每个授权目录追加只读放行。
//
// 这三例：①profile 串含只读放行且不含写放行 ②真机 sandbox-exec 能读授权目录
// ③对照组——不授权时确实读不到（证明放行确由 ReadablePaths 起作用，而非沙箱本就不隔离）。

func TestReadablePaths_DarwinProfileEmitsReadOnlyRule(t *testing.T) {
	ws := t.TempDir()
	ext := t.TempDir() // 工作区之外的授权目录
	s := newDarwinSandbox(Config{Workspace: ws, ReadablePaths: []string{ext}, Timeout: 10})
	sbpl := s.generateSBPL()

	wantRead := fmt.Sprintf("(allow file-read* (subpath \"%s\"))", ext)
	if !strings.Contains(sbpl, wantRead) {
		t.Fatalf("SBPL 未放行授权目录只读\n want: %s\n got:\n%s", wantRead, sbpl)
	}
	// 只读：不得为授权目录授予写权限
	writeRule := fmt.Sprintf("(allow file-write* (subpath \"%s\"))", ext)
	if strings.Contains(sbpl, writeRule) {
		t.Fatalf("授权目录不应获写权限: %s", writeRule)
	}
}

func TestReadablePaths_DarwinExecCanReadAuthorizedDir(t *testing.T) {
	requireSandboxTools(t)
	ws := t.TempDir()
	ext := externalAuthorizedDir(t) // 必须落在系统放行清单之外（同真实 /Users/<u>/work 场景）
	fp := filepath.Join(ext, "hello.txt")
	if err := os.WriteFile(fp, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	sb, err := New(Config{Workspace: ws, ReadablePaths: []string{ext}, Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	code := fmt.Sprintf("import os\nprint(os.path.exists(%q), os.path.isfile(%q))", ext, fp)
	res, err := sb.ExecCode(context.Background(), "python", code)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "True True" {
		t.Fatalf("授权目录应可读, 期望 'True True' 实得 %q (stderr=%q exit=%d)", got, res.Stderr, res.ExitCode)
	}
}

func TestReadablePaths_DarwinDeniedWithoutGrant(t *testing.T) {
	requireSandboxTools(t)
	ws := t.TempDir()
	ext := externalAuthorizedDir(t) // 未授权，且落在系统放行清单之外
	fp := filepath.Join(ext, "hello.txt")
	if err := os.WriteFile(fp, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	sb, err := New(Config{Workspace: ws, Timeout: 20}) // 不给 ReadablePaths
	if err != nil {
		t.Fatal(err)
	}
	code := fmt.Sprintf("import os\nprint(os.path.exists(%q))", ext)
	res, err := sb.ExecCode(context.Background(), "python", code)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "False" {
		t.Fatalf("未授权目录应读不到(沙箱隔离), 期望 'False' 实得 %q (stderr=%q)", got, res.Stderr)
	}
}

// externalAuthorizedDir 在「系统放行清单之外」造一个真实目录（家目录下临时子目录），
// 模拟用户经数据连接器授权的 /Users/<u>/work——它不在 darwin profile 的系统 subpath 里，
// 故默认 deny。用 /var/folders(=t.TempDir) 会落进 /private/var 放行区，无法体现隔离。
func externalAuthorizedDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("无法取家目录: %v", err)
	}
	dir, err := os.MkdirTemp(home, ".hexclaw-sbtest-")
	if err != nil {
		t.Skipf("家目录不可写，跳过: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// BUG-20260626 挑刺：ReadablePaths 来自连接器自由文本，若含 `"`/`\`/换行 等会破坏 SBPL
//
//	`(subpath "...")` 字面量——整张 profile 失效，sandbox-exec 解析失败 → 所有 code_exec 全挂；
//	含 `"` 还可能注入额外规则(如 (allow network*))。非绝对路径写进 subpath 也无意义。
//	正解：对每个授权路径校验（绝对 + 不含危险字符），非法者跳过，不污染 profile、不连累正常路径。
func TestReadablePaths_DarwinRejectsProfileBreakingPaths(t *testing.T) {
	ws := t.TempDir()
	good := t.TempDir()
	bad := []string{
		`/Users/x/a"b`,  // 引号：终止字符串字面量 → 注入/损坏
		"/Users/x/a\nb", // 换行
		`/x") (allow network*) (allow file-read* (subpath "/`, // 显式注入企图
		"relative/not/abs", // 非绝对
		"",                 // 空
	}
	s := newDarwinSandbox(Config{Workspace: ws, ReadablePaths: append(bad, good), Timeout: 10})
	sbpl := s.generateSBPL()

	// 正常路径仍放行
	if !strings.Contains(sbpl, fmt.Sprintf("(allow file-read* (subpath \"%s\"))", good)) {
		t.Fatalf("合法授权目录应仍被放行\n%s", sbpl)
	}
	// 不得出现注入的网络放行
	if strings.Contains(sbpl, "(allow network*)") && !s.cfg.Network {
		t.Fatalf("非法路径注入了 (allow network*)！\n%s", sbpl)
	}
	// 每个 ReadablePaths 衍生的 file-read 规则其 subpath 字面量必须闭合且不含裸引号
	for _, line := range strings.Split(sbpl, "\n") {
		if !strings.Contains(line, "(allow file-read* (subpath ") {
			continue
		}
		// 形如 (allow file-read* (subpath "<P>"))；提取 <P>，不得含 " 或换行
		inner := line
		if i := strings.Index(inner, "(subpath \""); i >= 0 {
			inner = inner[i+len("(subpath \""):]
		}
		j := strings.Index(inner, "\"")
		if j < 0 {
			t.Fatalf("subpath 字面量未闭合(profile 损坏): %q", line)
		}
		p := inner[:j]
		if strings.ContainsAny(p, "\"\n\r") {
			t.Fatalf("subpath 路径含危险字符(会损坏/注入 SBPL): %q", p)
		}
	}
}

func TestReadablePaths_DarwinBadPathDoesNotBreakExec(t *testing.T) {
	requireSandboxTools(t)
	ws := t.TempDir()
	s, err := New(Config{Workspace: ws, ReadablePaths: []string{`/Users/x/a"b`}, Timeout: 15})
	if err != nil {
		t.Fatal(err)
	}
	// 即便配了一个会损坏 profile 的非法路径，普通 code_exec 也必须照常跑（沙箱不被一个脏路径搞瘫）。
	res, err := s.ExecCode(context.Background(), "python", "print(40+2)")
	if err != nil {
		t.Fatalf("一个非法授权路径不应搞瘫 code_exec: %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "42" {
		t.Fatalf("期望 '42' 实得 %q (stderr=%q exit=%d)", got, res.Stderr, res.ExitCode)
	}
}

func requireSandboxTools(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 不可用，跳过真机沙箱集成")
	}
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec 不可用，跳过真机沙箱集成")
	}
}
