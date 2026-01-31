package timex

import (
	"time"
)

// MsecFormat 将毫秒时间戳转换为 Y-m-d H:i:s 格式
//
// 参数:
//   - msectime: 毫秒级 Unix 时间戳
//
// 返回:
//   - "2006-01-02 15:04:05" 格式的时间字符串
//
// 示例:
//
//	// 当前时间（毫秒）
//	ms := time.Now().UnixMilli()
//	formatted := timex.MsecFormat(ms)
//	// 输出: "2024-01-29 15:04:05"
func MsecFormat(msectime int64) string {
	// 将毫秒转换为秒
	sec := msectime / 1000
	// 转换纳秒余数
	nsec := (msectime % 1000) * 1e6
	return time.Unix(sec, nsec).Format("2006-01-02 15:04:05")
}

// MsecFormatWithLayout 将毫秒时间戳转换为自定义格式
//
// 参数:
//   - msectime: 毫秒级 Unix 时间戳
//   - layout: 自定义时间格式 (Go time 格式)
//
// 返回:
//   - 格式化的时间字符串
//
// 示例:
//
//	ms := time.Now().UnixMilli()
//	formatted := timex.MsecFormatWithLayout(ms, "2006/01/02")
//	// 输出: "2024/01/29"
func MsecFormatWithLayout(msectime int64, layout string) string {
	sec := msectime / 1000
	nsec := (msectime % 1000) * 1e6
	return time.Unix(sec, nsec).Format(layout)
}

// SecFormat 将秒级时间戳转换为 Y-m-d H:i:s 格式
//
// 参数:
//   - sectime: 秒级 Unix 时间戳
//
// 返回:
//   - "2006-01-02 15:04:05" 格式的时间字符串
func SecFormat(sectime int64) string {
	return time.Unix(sectime, 0).Format("2006-01-02 15:04:05")
}

// SecFormatWithLayout 将秒级时间戳转换为自定义格式
//
// 参数:
//   - sectime: 秒级 Unix 时间戳
//   - layout: 自定义时间格式
//
// 返回:
//   - 格式化的时间字符串
func SecFormatWithLayout(sectime int64, layout string) string {
	return time.Unix(sectime, 0).Format(layout)
}
