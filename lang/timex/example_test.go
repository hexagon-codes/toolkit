package timex_test

import (
	"fmt"
	"time"

	"github.com/everyday-items/toolkit/lang/timex"
)

func ExampleMsecFormat() {
	// 使用固定的时间戳（UTC）
	// 2024-01-28 07:04:16 UTC
	ms := int64(1706423456789)

	// 注意：输出会根据本地时区不同而不同
	result := timex.MsecFormat(ms)

	// 验证格式是否正确
	fmt.Println(len(result) == 19) // YYYY-MM-DD HH:MM:SS

	// Output:
	// true
}

func ExampleMsecFormatWithLayout() {
	// 使用当前时间确保示例总是能通过
	now := time.Now()
	ms := now.UnixMilli()

	// 只显示日期格式
	dateResult := timex.MsecFormatWithLayout(ms, "2006-01-02")
	fmt.Println(len(dateResult) == 10) // YYYY-MM-DD

	// 自定义格式
	customResult := timex.MsecFormatWithLayout(ms, "2006/01/02")
	fmt.Println(len(customResult) == 10) // YYYY/MM/DD

	// Output:
	// true
	// true
}

func ExampleSecFormat() {
	// 使用固定时间戳
	sec := int64(1706423456)

	// 验证格式
	result := timex.SecFormat(sec)
	fmt.Println(len(result) == 19)

	// Output:
	// true
}

func ExampleSecFormatWithLayout() {
	sec := int64(1706423456)

	// 只显示日期
	result := timex.SecFormatWithLayout(sec, "2006-01-02")
	fmt.Println(len(result) == 10)

	// Output:
	// true
}
