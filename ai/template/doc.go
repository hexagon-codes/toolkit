// Package template 提供 AI Prompt 模板引擎
//
// 本包用于构建和管理可复用的 Prompt 模板，支持：
//   - 变量替换
//   - 条件渲染
//   - 消息组装
//   - 模板继承
//
// 基本用法：
//
//	tmpl := template.New("greeting").
//	    System("You are a helpful assistant.").
//	    User("Hello, my name is {{.Name}}!")
//
//	messages, err := tmpl.Render(map[string]any{
//	    "Name": "Alice",
//	})
//
// 带条件：
//
//	tmpl := template.New("assistant").
//	    System("You are {{if .Expert}}an expert{{else}}a helpful{{end}} assistant.")
//
// 从文件加载：
//
//	tmpl, err := template.LoadFile("prompts/greeting.tmpl")
package template
