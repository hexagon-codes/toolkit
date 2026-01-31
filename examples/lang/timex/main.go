package main

import (
	"fmt"
	"time"

	"github.com/everyday-items/toolkit/lang/timex"
)

func main() {
	fmt.Println("=== lang/timex 时间格式化示例 ===")

	// 1. 毫秒时间戳格式化
	fmt.Println("1. 毫秒时间戳格式化")
	demonstrateMsecFormat()

	// 2. 秒级时间戳格式化
	fmt.Println("\n2. 秒级时间戳格式化")
	demonstrateSecFormat()

	// 3. 自定义格式化
	fmt.Println("\n3. 自定义格式化")
	demonstrateCustomFormat()

	// 4. 实际应用场景
	fmt.Println("\n4. 实际应用场景")
	demonstratePracticalUseCases()

	// 5. 时间戳转换对比
	fmt.Println("\n5. 时间戳转换对比")
	demonstrateComparison()

	// 6. 边界情况
	fmt.Println("\n6. 边界情况处理")
	demonstrateEdgeCases()
}

// demonstrateMsecFormat 演示毫秒时间戳格式化
func demonstrateMsecFormat() {
	// 当前时间（毫秒）
	fmt.Println("\n  [当前时间]")
	nowMs := time.Now().UnixMilli()
	formatted := timex.MsecFormat(nowMs)
	fmt.Printf("  毫秒时间戳: %d\n", nowMs)
	fmt.Printf("  ✓ 格式化结果: %s\n", formatted)

	// 指定时间点（毫秒）
	fmt.Println("\n  [指定时间点]")
	specificTime := time.Date(2024, 1, 29, 15, 30, 45, 0, time.Local)
	specificMs := specificTime.UnixMilli()
	formatted2 := timex.MsecFormat(specificMs)
	fmt.Printf("  时间点: %v\n", specificTime)
	fmt.Printf("  毫秒时间戳: %d\n", specificMs)
	fmt.Printf("  ✓ 格式化结果: %s\n", formatted2)

	// 历史时间
	fmt.Println("\n  [历史时间]")
	historicalMs := int64(1609459200000) // 2021-01-01 00:00:00
	formatted3 := timex.MsecFormat(historicalMs)
	fmt.Printf("  毫秒时间戳: %d\n", historicalMs)
	fmt.Printf("  ✓ 格式化结果: %s\n", formatted3)

	// 毫秒精度展示
	fmt.Println("\n  [毫秒精度]")
	baseTime := time.Now()
	for i := 0; i < 3; i++ {
		ms := baseTime.Add(time.Duration(i) * 100 * time.Millisecond).UnixMilli()
		fmt.Printf("  %d ms -> %s\n", ms, timex.MsecFormat(ms))
	}
}

// demonstrateSecFormat 演示秒级时间戳格式化
func demonstrateSecFormat() {
	// 当前时间（秒）
	fmt.Println("\n  [当前时间]")
	nowSec := time.Now().Unix()
	formatted := timex.SecFormat(nowSec)
	fmt.Printf("  秒级时间戳: %d\n", nowSec)
	fmt.Printf("  ✓ 格式化结果: %s\n", formatted)

	// Unix 纪元时间
	fmt.Println("\n  [Unix 纪元时间]")
	epoch := int64(0)
	formatted2 := timex.SecFormat(epoch)
	fmt.Printf("  秒级时间戳: %d\n", epoch)
	fmt.Printf("  ✓ 格式化结果: %s (1970-01-01 08:00:00 UTC+8)\n", formatted2)

	// 常见时间点
	fmt.Println("\n  [常见时间点]")
	timestamps := map[string]int64{
		"2000年": 946684800,
		"2010年": 1262304000,
		"2020年": 1577836800,
		"2024年": 1704067200,
	}

	for label, ts := range timestamps {
		fmt.Printf("  %s: %s\n", label, timex.SecFormat(ts))
	}
}

// demonstrateCustomFormat 演示自定义格式化
func demonstrateCustomFormat() {
	now := time.Now()
	nowMs := now.UnixMilli()
	nowSec := now.Unix()

	// 毫秒时间戳自定义格式
	fmt.Println("\n  [毫秒时间戳] 自定义格式")
	formats := []string{
		"2006-01-02",                     // 日期
		"15:04:05",                       // 时间
		"2006/01/02 15:04:05",            // 斜杠分隔
		"2006年01月02日",                    // 中文
		"02-Jan-2006",                    // 英文月份
		"Monday, 02-Jan-06 15:04:05 MST", // RFC850
		time.RFC3339,                     // RFC3339
	}

	for _, format := range formats {
		result := timex.MsecFormatWithLayout(nowMs, format)
		fmt.Printf("  格式: %-35s => %s\n", format, result)
	}

	// 秒级时间戳自定义格式
	fmt.Println("\n  [秒级时间戳] 自定义格式")
	for _, format := range formats[:4] {
		result := timex.SecFormatWithLayout(nowSec, format)
		fmt.Printf("  格式: %-25s => %s\n", format, result)
	}
}

// demonstratePracticalUseCases 实际应用场景
func demonstratePracticalUseCases() {
	// 场景1: API 响应时间格式化
	fmt.Println("\n  [场景1] API 响应时间格式化")
	type APIResponse struct {
		Code      int    `json:"code"`
		Message   string `json:"message"`
		Timestamp int64  `json:"timestamp"` // 毫秒时间戳
	}

	resp := APIResponse{
		Code:      200,
		Message:   "Success",
		Timestamp: time.Now().UnixMilli(),
	}

	fmt.Printf("  API 响应: code=%d, timestamp=%d\n", resp.Code, resp.Timestamp)
	fmt.Printf("  ✓ 格式化时间: %s\n", timex.MsecFormat(resp.Timestamp))

	// 场景2: 日志时间格式化
	fmt.Println("\n  [场景2] 日志时间格式化")
	logTime := time.Now().UnixMilli()
	logLevel := "INFO"
	logMessage := "Application started"
	logLine := fmt.Sprintf("[%s] [%s] %s",
		timex.MsecFormat(logTime),
		logLevel,
		logMessage,
	)
	fmt.Printf("  ✓ 日志输出: %s\n", logLine)

	// 场景3: 数据库时间字段
	fmt.Println("\n  [场景3] 数据库时间字段展示")
	type User struct {
		ID        int64
		Username  string
		CreatedAt int64 // 数据库存储的毫秒时间戳
		UpdatedAt int64
	}

	user := User{
		ID:        1001,
		Username:  "alice",
		CreatedAt: time.Now().Add(-24 * time.Hour).UnixMilli(),
		UpdatedAt: time.Now().UnixMilli(),
	}

	fmt.Printf("  用户: %s\n", user.Username)
	fmt.Printf("  ✓ 创建时间: %s\n", timex.MsecFormat(user.CreatedAt))
	fmt.Printf("  ✓ 更新时间: %s\n", timex.MsecFormat(user.UpdatedAt))

	// 场景4: 订单时间展示
	fmt.Println("\n  [场景4] 订单时间展示")
	orderTime := time.Now().Add(-2 * time.Hour).Unix()
	orderDate := timex.SecFormatWithLayout(orderTime, "2006年01月02日")
	orderFullTime := timex.SecFormat(orderTime)

	fmt.Printf("  订单日期: %s\n", orderDate)
	fmt.Printf("  ✓ 下单时间: %s\n", orderFullTime)

	// 场景5: 时间范围查询
	fmt.Println("\n  [场景5] 时间范围查询")
	startOfDay := time.Now().Truncate(24 * time.Hour).Unix()
	endOfDay := time.Now().Unix()

	fmt.Printf("  查询范围:\n")
	fmt.Printf("  ✓ 开始: %s (%d)\n", timex.SecFormat(startOfDay), startOfDay)
	fmt.Printf("  ✓ 结束: %s (%d)\n", timex.SecFormat(endOfDay), endOfDay)

	// 场景6: 导出文件名
	fmt.Println("\n  [场景6] 导出文件名生成")
	exportTime := time.Now().Unix()
	fileName := fmt.Sprintf("report_%s.csv",
		timex.SecFormatWithLayout(exportTime, "20060102_150405"))
	fmt.Printf("  ✓ 文件名: %s\n", fileName)
}

// demonstrateComparison 时间戳转换对比
func demonstrateComparison() {
	// 标准库方式
	fmt.Println("\n  [标准库] time.Unix().Format()")
	nowMs := time.Now().UnixMilli()
	standardWay := time.UnixMilli(nowMs).Format("2006-01-02 15:04:05")
	fmt.Printf("  代码: time.UnixMilli(ms).Format(\"2006-01-02 15:04:05\")\n")
	fmt.Printf("  结果: %s\n", standardWay)

	// timex 方式
	fmt.Println("\n  [timex] MsecFormat()")
	timexWay := timex.MsecFormat(nowMs)
	fmt.Printf("  代码: timex.MsecFormat(ms)\n")
	fmt.Printf("  结果: %s\n", timexWay)

	fmt.Println("\n  ✓ timex 优势:")
	fmt.Println("    - 代码更简洁")
	fmt.Println("    - 不需要记忆 Go 时间格式")
	fmt.Println("    - 统一的 API")
	fmt.Println("    - 零依赖")

	// 性能对比
	fmt.Println("\n  [性能对比] 100万次转换")
	iterations := 1000000

	// 标准库
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_ = time.UnixMilli(nowMs).Format("2006-01-02 15:04:05")
	}
	standardDuration := time.Since(start)
	fmt.Printf("  标准库: %v\n", standardDuration)

	// timex
	start = time.Now()
	for i := 0; i < iterations; i++ {
		_ = timex.MsecFormat(nowMs)
	}
	timexDuration := time.Since(start)
	fmt.Printf("  timex:  %v\n", timexDuration)

	fmt.Printf("  ✓ 性能相当（都是基于标准库）\n")
}

// demonstrateEdgeCases 边界情况
func demonstrateEdgeCases() {
	// 零值
	fmt.Println("\n  [零值] 时间戳为 0")
	zeroMs := int64(0)
	zeroSec := int64(0)
	fmt.Printf("  MsecFormat(0)  = %s\n", timex.MsecFormat(zeroMs))
	fmt.Printf("  SecFormat(0)   = %s\n", timex.SecFormat(zeroSec))

	// 负数时间戳
	fmt.Println("\n  [负数] 1970年之前的时间")
	negativeSec := int64(-86400) // 1969-12-31
	fmt.Printf("  SecFormat(-86400) = %s\n", timex.SecFormat(negativeSec))

	// 未来时间
	fmt.Println("\n  [未来时间] 2100年")
	futureTime := time.Date(2100, 1, 1, 0, 0, 0, 0, time.Local)
	futureSec := futureTime.Unix()
	fmt.Printf("  SecFormat(%d) = %s\n", futureSec, timex.SecFormat(futureSec))

	// 毫秒精度丢失
	fmt.Println("\n  [精度对比] 毫秒 vs 秒")
	now := time.Now()
	nowMs := now.UnixMilli()
	nowSec := now.Unix()

	fmt.Printf("  毫秒时间戳: %d\n", nowMs)
	fmt.Printf("  秒级时间戳: %d\n", nowSec)
	fmt.Printf("  MsecFormat:  %s.%03d\n", timex.MsecFormat(nowMs), nowMs%1000)
	fmt.Printf("  SecFormat:   %s.000 (精度丢失)\n", timex.SecFormat(nowSec))

	// 时区问题
	fmt.Println("\n  [时区] 不同时区的时间")
	utcTime := time.Now().UTC()
	localTime := time.Now()

	fmt.Printf("  UTC:   %s\n", timex.MsecFormat(utcTime.UnixMilli()))
	fmt.Printf("  Local: %s\n", timex.MsecFormat(localTime.UnixMilli()))
	fmt.Printf("  ✓ timex 使用本地时区\n")

	// 大数值
	fmt.Println("\n  [大数值] 超大时间戳")
	largeMs := int64(9999999999999) // 2286年
	fmt.Printf("  时间戳: %d\n", largeMs)
	fmt.Printf("  ✓ 格式化: %s\n", timex.MsecFormat(largeMs))
}
