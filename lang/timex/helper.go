package timex

import (
	"time"
)

// Now 返回当前时间（方便测试时 mock）
var Now = time.Now

// IsToday 判断是否为今天
func IsToday(t time.Time) bool {
	now := Now()
	return t.Year() == now.Year() && t.YearDay() == now.YearDay()
}

// IsYesterday 判断是否为昨天
func IsYesterday(t time.Time) bool {
	yesterday := Now().AddDate(0, 0, -1)
	return t.Year() == yesterday.Year() && t.YearDay() == yesterday.YearDay()
}

// IsTomorrow 判断是否为明天
func IsTomorrow(t time.Time) bool {
	tomorrow := Now().AddDate(0, 0, 1)
	return t.Year() == tomorrow.Year() && t.YearDay() == tomorrow.YearDay()
}

// IsThisWeek 判断是否为本周
func IsThisWeek(t time.Time) bool {
	now := Now()
	y1, w1 := t.ISOWeek()
	y2, w2 := now.ISOWeek()
	return y1 == y2 && w1 == w2
}

// IsThisMonth 判断是否为本月
func IsThisMonth(t time.Time) bool {
	now := Now()
	return t.Year() == now.Year() && t.Month() == now.Month()
}

// IsThisYear 判断是否为今年
func IsThisYear(t time.Time) bool {
	return t.Year() == Now().Year()
}

// IsWeekend 判断是否为周末
func IsWeekend(t time.Time) bool {
	weekday := t.Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}

// IsLeapYear 判断是否为闰年
func IsLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// StartOfDay 获取当天开始时间 (00:00:00)
func StartOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// EndOfDay 获取当天结束时间 (23:59:59.999999999)
func EndOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

// StartOfWeek 获取本周开始时间（周一）
func StartOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // 周日视为 7
	}
	return StartOfDay(t.AddDate(0, 0, 1-weekday))
}

// EndOfWeek 获取本周结束时间（周日）
func EndOfWeek(t time.Time) time.Time {
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return EndOfDay(t.AddDate(0, 0, 7-weekday))
}

// StartOfMonth 获取本月开始时间
func StartOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// EndOfMonth 获取本月结束时间
func EndOfMonth(t time.Time) time.Time {
	return StartOfMonth(t).AddDate(0, 1, 0).Add(-time.Nanosecond)
}

// StartOfYear 获取本年开始时间
func StartOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}

// EndOfYear 获取本年结束时间
func EndOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), 12, 31, 23, 59, 59, 999999999, t.Location())
}

// Between 判断时间是否在范围内（包含边界）
func Between(t, start, end time.Time) bool {
	return (t.Equal(start) || t.After(start)) && (t.Equal(end) || t.Before(end))
}

// DaysBetween 计算两个时间之间的天数差（绝对值）
func DaysBetween(t1, t2 time.Time) int {
	// 归一化到当天 00:00:00
	t1 = StartOfDay(t1)
	t2 = StartOfDay(t2)

	duration := t2.Sub(t1)
	days := int(duration.Hours() / 24)
	if days < 0 {
		days = -days
	}
	return days
}

// HoursBetween 计算两个时间之间的小时数差（绝对值）
func HoursBetween(t1, t2 time.Time) int {
	duration := t2.Sub(t1)
	hours := int(duration.Hours())
	if hours < 0 {
		hours = -hours
	}
	return hours
}

// MinutesBetween 计算两个时间之间的分钟数差（绝对值）
func MinutesBetween(t1, t2 time.Time) int {
	duration := t2.Sub(t1)
	minutes := int(duration.Minutes())
	if minutes < 0 {
		minutes = -minutes
	}
	return minutes
}

// AddDays 添加天数
func AddDays(t time.Time, days int) time.Time {
	return t.AddDate(0, 0, days)
}

// AddWeeks 添加周数
func AddWeeks(t time.Time, weeks int) time.Time {
	return t.AddDate(0, 0, weeks*7)
}

// AddMonths 添加月数
func AddMonths(t time.Time, months int) time.Time {
	return t.AddDate(0, months, 0)
}

// AddYears 添加年数
func AddYears(t time.Time, years int) time.Time {
	return t.AddDate(years, 0, 0)
}

// DaysInMonth 获取指定月份的天数
func DaysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// DaysInYear 获取指定年份的天数
func DaysInYear(year int) int {
	if IsLeapYear(year) {
		return 366
	}
	return 365
}

// Age 计算年龄
func Age(birthday time.Time) int {
	now := Now()
	years := now.Year() - birthday.Year()

	// 检查是否已过生日
	if now.Month() < birthday.Month() ||
		(now.Month() == birthday.Month() && now.Day() < birthday.Day()) {
		years--
	}

	if years < 0 {
		return 0
	}
	return years
}

// Unix 时间戳转换
func Unix(sec int64) time.Time {
	return time.Unix(sec, 0)
}

// UnixMilli 毫秒时间戳转换
func UnixMilli(msec int64) time.Time {
	return time.UnixMilli(msec)
}

// ToUnix 转换为秒级时间戳
func ToUnix(t time.Time) int64 {
	return t.Unix()
}

// ToUnixMilli 转换为毫秒级时间戳
func ToUnixMilli(t time.Time) int64 {
	return t.UnixMilli()
}
