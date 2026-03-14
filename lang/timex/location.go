package timex

import (
	"sync"
	"time"
)

// 预定义时区（使用懒加载）
var (
	shanghaiOnce sync.Once
	shanghaiLoc  *time.Location

	beijingOnce sync.Once
	beijingLoc  *time.Location

	tokyoOnce sync.Once
	tokyoLoc  *time.Location

	newYorkOnce sync.Once
	newYorkLoc  *time.Location

	londonOnce sync.Once
	londonLoc  *time.Location

	utcOnce sync.Once
	utcLoc  *time.Location
)

// Shanghai 返回上海时区 (Asia/Shanghai, UTC+8)
//
// 返回:
//   - *time.Location: 上海时区
//
// 示例:
//
//	loc := timex.Shanghai()
//	t := time.Now().In(loc)
func Shanghai() *time.Location {
	shanghaiOnce.Do(func() {
		var err error
		shanghaiLoc, err = time.LoadLocation("Asia/Shanghai")
		if err != nil {
			// 如果加载失败，使用固定偏移
			shanghaiLoc = time.FixedZone("CST", 8*60*60)
		}
	})
	return shanghaiLoc
}

// Beijing 返回北京时区（与上海相同, UTC+8）
//
// 返回:
//   - *time.Location: 北京时区
func Beijing() *time.Location {
	beijingOnce.Do(func() {
		beijingLoc = Shanghai()
	})
	return beijingLoc
}

// Tokyo 返回东京时区 (Asia/Tokyo, UTC+9)
//
// 返回:
//   - *time.Location: 东京时区
func Tokyo() *time.Location {
	tokyoOnce.Do(func() {
		var err error
		tokyoLoc, err = time.LoadLocation("Asia/Tokyo")
		if err != nil {
			tokyoLoc = time.FixedZone("JST", 9*60*60)
		}
	})
	return tokyoLoc
}

// NewYork 返回纽约时区 (America/New_York, UTC-5/UTC-4)
//
// 返回:
//   - *time.Location: 纽约时区（考虑夏令时）
func NewYork() *time.Location {
	newYorkOnce.Do(func() {
		var err error
		newYorkLoc, err = time.LoadLocation("America/New_York")
		if err != nil {
			newYorkLoc = time.FixedZone("EST", -5*60*60)
		}
	})
	return newYorkLoc
}

// London 返回伦敦时区 (Europe/London, UTC/UTC+1)
//
// 返回:
//   - *time.Location: 伦敦时区（考虑夏令时）
func London() *time.Location {
	londonOnce.Do(func() {
		var err error
		londonLoc, err = time.LoadLocation("Europe/London")
		if err != nil {
			londonLoc = time.FixedZone("GMT", 0)
		}
	})
	return londonLoc
}

// UTC 返回 UTC 时区
//
// 返回:
//   - *time.Location: UTC 时区
func UTC() *time.Location {
	utcOnce.Do(func() {
		utcLoc = time.UTC
	})
	return utcLoc
}

// InShanghai 将时间转换为上海时区
//
// 参数:
//   - t: 要转换的时间
//
// 返回:
//   - time.Time: 上海时区的时间
//
// 示例:
//
//	shanghaiTime := timex.InShanghai(time.Now().UTC())
func InShanghai(t time.Time) time.Time {
	return t.In(Shanghai())
}

// InBeijing 将时间转换为北京时区
//
// 参数:
//   - t: 要转换的时间
//
// 返回:
//   - time.Time: 北京时区的时间
func InBeijing(t time.Time) time.Time {
	return t.In(Beijing())
}

// InTokyo 将时间转换为东京时区
//
// 参数:
//   - t: 要转换的时间
//
// 返回:
//   - time.Time: 东京时区的时间
func InTokyo(t time.Time) time.Time {
	return t.In(Tokyo())
}

// InNewYork 将时间转换为纽约时区
//
// 参数:
//   - t: 要转换的时间
//
// 返回:
//   - time.Time: 纽约时区的时间
func InNewYork(t time.Time) time.Time {
	return t.In(NewYork())
}

// InLondon 将时间转换为伦敦时区
//
// 参数:
//   - t: 要转换的时间
//
// 返回:
//   - time.Time: 伦敦时区的时间
func InLondon(t time.Time) time.Time {
	return t.In(London())
}

// InUTC 将时间转换为 UTC 时区
//
// 参数:
//   - t: 要转换的时间
//
// 返回:
//   - time.Time: UTC 时区的时间
func InUTC(t time.Time) time.Time {
	return t.In(UTC())
}

// NowShanghai 返回上海时区的当前时间
//
// 返回:
//   - time.Time: 上海时区的当前时间
//
// 示例:
//
//	now := timex.NowShanghai()
//	fmt.Println(now.Format("2006-01-02 15:04:05"))
func NowShanghai() time.Time {
	return Now().In(Shanghai())
}

// NowBeijing 返回北京时区的当前时间
//
// 返回:
//   - time.Time: 北京时区的当前时间
func NowBeijing() time.Time {
	return Now().In(Beijing())
}

// NowTokyo 返回东京时区的当前时间
//
// 返回:
//   - time.Time: 东京时区的当前时间
func NowTokyo() time.Time {
	return Now().In(Tokyo())
}

// NowNewYork 返回纽约时区的当前时间
//
// 返回:
//   - time.Time: 纽约时区的当前时间
func NowNewYork() time.Time {
	return Now().In(NewYork())
}

// NowLondon 返回伦敦时区的当前时间
//
// 返回:
//   - time.Time: 伦敦时区的当前时间
func NowLondon() time.Time {
	return Now().In(London())
}

// NowUTC 返回 UTC 时区的当前时间
//
// 返回:
//   - time.Time: UTC 时区的当前时间
func NowUTC() time.Time {
	return Now().In(UTC())
}

// ParseInShanghai 在上海时区解析时间字符串
//
// 参数:
//   - layout: 时间格式
//   - value: 时间字符串
//
// 返回:
//   - time.Time: 解析后的时间（上海时区）
//   - error: 解析错误
//
// 示例:
//
//	t, err := timex.ParseInShanghai("2006-01-02 15:04:05", "2024-01-29 10:00:00")
func ParseInShanghai(layout, value string) (time.Time, error) {
	return time.ParseInLocation(layout, value, Shanghai())
}

// ParseInBeijing 在北京时区解析时间字符串
//
// 参数:
//   - layout: 时间格式
//   - value: 时间字符串
//
// 返回:
//   - time.Time: 解析后的时间
//   - error: 解析错误
func ParseInBeijing(layout, value string) (time.Time, error) {
	return time.ParseInLocation(layout, value, Beijing())
}

// FixedZone 创建固定偏移的时区
//
// 参数:
//   - name: 时区名称
//   - offsetHours: 相对 UTC 的小时偏移（可以为负数）
//
// 返回:
//   - *time.Location: 固定偏移时区
//
// 示例:
//
//	cst := timex.FixedZone("CST", 8)  // UTC+8
//	est := timex.FixedZone("EST", -5) // UTC-5
func FixedZone(name string, offsetHours int) *time.Location {
	return time.FixedZone(name, offsetHours*60*60)
}
