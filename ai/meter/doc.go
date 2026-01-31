// Package meter 提供 AI API 用量统计和成本追踪
//
// 本包用于追踪 AI API 的使用情况，包括：
//   - Token 消耗统计
//   - 请求次数统计
//   - 成本估算
//   - 按时间窗口聚合
//
// 基本用法：
//
//	m := meter.New()
//	m.Record("gpt-4", 100, 50)  // 100 输入 token, 50 输出 token
//
//	stats := m.Stats()
//	fmt.Printf("Total tokens: %d\n", stats.TotalTokens)
//	fmt.Printf("Estimated cost: $%.4f\n", stats.EstimatedCost)
//
// 按模型统计：
//
//	modelStats := m.StatsByModel("gpt-4")
//
// 导出报告：
//
//	report := m.Report()
//	fmt.Println(report)
package meter
