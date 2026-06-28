package stringx

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTruncateBytes_RuneSafe 锁定按字节封顶的 rune 安全截断（RU-1 / hex-test AP-141）。
// 对照实验：裸 byte-slice 在多字节边界劈裂产生非法 UTF-8；TruncateBytes 回退到 rune 边界后永远是 valid UTF-8。
func TestTruncateBytes_RuneSafe(t *testing.T) {
	// "你好世界" = 4 个 CJK 字符，每个 3 字节，共 12 字节。
	cjk := "你好世界"

	t.Run("对照：裸 byte-slice 在 rune 中间切 → 非法 UTF-8（这正是被修的 bug 形态）", func(t *testing.T) {
		// 截到第 7 字节落在第 3 个字符（"世"）中间。
		naive := cjk[:7] + "..."
		if utf8.ValidString(naive) {
			t.Fatalf("前提失效：裸 byte-slice 本应产生非法 UTF-8，得到 %q", naive)
		}
	})

	t.Run("TruncateBytes 同样字节预算 → valid UTF-8 且不含替换符", func(t *testing.T) {
		out := TruncateBytes(cjk, 7, "...")
		if !utf8.ValidString(out) {
			t.Fatalf("TruncateBytes 产生非法 UTF-8: %q", out)
		}
		if strings.ContainsRune(out, utf8.RuneError) {
			t.Fatalf("TruncateBytes 含替换符 U+FFFD: %q", out)
		}
		// 回退到边界：内容部分是 "你好"(6 字节) + 后缀（"世" 第 7 字节起被回退掉）。
		if out != "你好..." {
			t.Fatalf("期望回退到 rune 边界 \"你好...\"，得到 %q", out)
		}
	})

	t.Run("未超预算 → 原样返回（不加后缀）", func(t *testing.T) {
		if out := TruncateBytes(cjk, 100, "..."); out != cjk {
			t.Fatalf("未超预算应原样返回，得到 %q", out)
		}
		// 边界相等：len==maxBytes 不截断。
		if out := TruncateBytes(cjk, len(cjk), "X"); out != cjk {
			t.Fatalf("len==maxBytes 不应截断，得到 %q", out)
		}
	})

	t.Run("切点恰落在 rune 边界 → 不回退", func(t *testing.T) {
		// 第 6 字节正好是 "你好" 之后的边界。
		out := TruncateBytes(cjk, 6, "…")
		if out != "你好…" {
			t.Fatalf("切点在边界应不回退，期望 \"你好…\"，得到 %q", out)
		}
	})

	t.Run("emoji（4 字节 astral）边界安全", func(t *testing.T) {
		s := "ab😀cd" // a b [4字节emoji] c d
		// 截到第 3 字节落在 emoji 中间 → 回退到 "ab"。
		out := TruncateBytes(s, 3, "_")
		if !utf8.ValidString(out) {
			t.Fatalf("emoji 边界产生非法 UTF-8: %q", out)
		}
		if out != "ab_" {
			t.Fatalf("期望回退到 \"ab_\"，得到 %q", out)
		}
	})

	t.Run("纯 ASCII 行为与裸切一致（无多字节可劈裂）", func(t *testing.T) {
		if out := TruncateBytes("hello world", 5, "..."); out != "hello..." {
			t.Fatalf("ASCII 截断异常: %q", out)
		}
	})

	t.Run("maxBytes<=0 → 空串", func(t *testing.T) {
		if out := TruncateBytes(cjk, 0, "..."); out != "" {
			t.Fatalf("maxBytes<=0 应返回空串，得到 %q", out)
		}
	})
}
