package timex

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// FormatDuration 将 Duration 格式化为人类可读的字符串
//
// 返回格式: "1d2h3m4s" 或 "2h30m5s" 等
// 只显示非零部分
//
// 参数:
//   - d: 要格式化的 Duration
//
// 返回:
//   - string: 格式化的字符串
//
// 示例:
//
//	timex.FormatDuration(time.Hour * 2 + time.Minute * 30)  // "2h30m"
//	timex.FormatDuration(time.Second * 90)                   // "1m30s"
//	timex.FormatDuration(time.Millisecond * 500)             // "500ms"
func FormatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	// 保存原始值，用于微秒/纳秒级别的回退格式化
	original := d

	var result strings.Builder
	negative := d < 0
	if negative {
		d = -d
		result.WriteByte('-')
	}

	days := d / (24 * time.Hour)
	d = d % (24 * time.Hour)
	hours := d / time.Hour
	d = d % time.Hour
	minutes := d / time.Minute
	d = d % time.Minute
	seconds := d / time.Second
	d = d % time.Second
	millis := d / time.Millisecond

	if days > 0 {
		fmt.Fprintf(&result, "%dd", days)
	}
	if hours > 0 {
		fmt.Fprintf(&result, "%dh", hours)
	}
	if minutes > 0 {
		fmt.Fprintf(&result, "%dm", minutes)
	}
	if seconds > 0 {
		fmt.Fprintf(&result, "%ds", seconds)
	}
	if millis > 0 {
		fmt.Fprintf(&result, "%dms", millis)
	}

	// 如果只有微秒或纳秒，使用原始 Duration 的 String() 方法（保留负号）
	if result.Len() == 0 || (negative && result.Len() == 1) {
		return original.String()
	}

	return result.String()
}

// FormatDurationShort 将 Duration 格式化为简短形式
//
// 只显示最重要的两个单位
//
// 参数:
//   - d: 要格式化的 Duration
//
// 返回:
//   - string: 简短格式的字符串
//
// 示例:
//
//	timex.FormatDurationShort(time.Hour * 26 + time.Minute * 30)  // "1d2h"
func FormatDurationShort(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	negative := d < 0
	if negative {
		d = -d
	}

	days := d / (24 * time.Hour)
	d = d % (24 * time.Hour)
	hours := d / time.Hour
	d = d % time.Hour
	minutes := d / time.Minute
	d = d % time.Minute
	seconds := d / time.Second

	var result string
	if days > 0 {
		if hours > 0 {
			result = fmt.Sprintf("%dd%dh", days, hours)
		} else {
			result = fmt.Sprintf("%dd", days)
		}
	} else if hours > 0 {
		if minutes > 0 {
			result = fmt.Sprintf("%dh%dm", hours, minutes)
		} else {
			result = fmt.Sprintf("%dh", hours)
		}
	} else if minutes > 0 {
		if seconds > 0 {
			result = fmt.Sprintf("%dm%ds", minutes, seconds)
		} else {
			result = fmt.Sprintf("%dm", minutes)
		}
	} else {
		result = fmt.Sprintf("%ds", seconds)
	}

	if negative {
		return "-" + result
	}
	return result
}

// durationPattern 匹配 duration 字符串的正则表达式
var durationPattern = regexp.MustCompile(`^(-?)(?:(\d+)d)?(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?(?:(\d+)ms)?$`)

// ParseDuration 解析 duration 字符串，支持天数
//
// 支持的格式: "1d", "2h", "3m", "4s", "5ms", "1d2h3m4s" 等
// 也支持标准 Go duration 格式
//
// 参数:
//   - s: 要解析的字符串
//
// 返回:
//   - time.Duration: 解析后的 Duration
//   - error: 解析错误
//
// 示例:
//
//	timex.ParseDuration("1d")      // 24h0m0s
//	timex.ParseDuration("1d2h")    // 26h0m0s
//	timex.ParseDuration("1h30m")   // 1h30m0s
//	timex.ParseDuration("90s")     // 1m30s
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}

	// 如果包含 'd' 则使用自定义解析
	if strings.Contains(s, "d") {
		matches := durationPattern.FindStringSubmatch(s)
		if matches == nil {
			return 0, fmt.Errorf("invalid duration format: %s", s)
		}

		var d time.Duration
		negative := matches[1] == "-"

		if matches[2] != "" {
			days, _ := strconv.ParseInt(matches[2], 10, 64)
			d += time.Duration(days) * 24 * time.Hour
		}
		if matches[3] != "" {
			hours, _ := strconv.ParseInt(matches[3], 10, 64)
			d += time.Duration(hours) * time.Hour
		}
		if matches[4] != "" {
			minutes, _ := strconv.ParseInt(matches[4], 10, 64)
			d += time.Duration(minutes) * time.Minute
		}
		if matches[5] != "" {
			seconds, _ := strconv.ParseInt(matches[5], 10, 64)
			d += time.Duration(seconds) * time.Second
		}
		if matches[6] != "" {
			millis, _ := strconv.ParseInt(matches[6], 10, 64)
			d += time.Duration(millis) * time.Millisecond
		}

		if negative {
			d = -d
		}
		return d, nil
	}

	// 使用标准库解析
	return time.ParseDuration(s)
}

// MustParseDuration 解析 duration 字符串，解析失败时 panic
//
// 参数:
//   - s: 要解析的字符串
//
// 返回:
//   - time.Duration: 解析后的 Duration
func MustParseDuration(s string) time.Duration {
	d, err := ParseDuration(s)
	if err != nil {
		panic(err)
	}
	return d
}

// DurationRound 将 Duration 四舍五入到指定精度
//
// 参数:
//   - d: 要处理的 Duration
//   - precision: 精度
//
// 返回:
//   - time.Duration: 四舍五入后的 Duration
//
// 示例:
//
//	timex.DurationRound(time.Second * 90, time.Minute)  // 2m0s
func DurationRound(d, precision time.Duration) time.Duration {
	if precision <= 0 {
		return d
	}
	return d.Round(precision)
}

// DurationTruncate 将 Duration 截断到指定精度
//
// 参数:
//   - d: 要处理的 Duration
//   - precision: 精度
//
// 返回:
//   - time.Duration: 截断后的 Duration
//
// 示例:
//
//	timex.DurationTruncate(time.Second * 90, time.Minute)  // 1m0s
func DurationTruncate(d, precision time.Duration) time.Duration {
	if precision <= 0 {
		return d
	}
	return d.Truncate(precision)
}
