// Package elasticsearch 提供 Elasticsearch 客户端单例管理
//
// 封装官方 Elasticsearch Go 客户端，提供单例模式、健康检查和优雅关闭功能。
//
// 基本用法:
//
//	// 应用启动时初始化
//	err := elasticsearch.Init(&elasticsearch.Config{
//	    Addresses: []string{"http://localhost:9200"},
//	    Username:  "elastic",
//	    Password:  "password",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer elasticsearch.Close()
//
//	// 使用客户端
//	es := elasticsearch.ES()
//	res, _ := es.Index("users", strings.NewReader(`{"name":"test"}`))
//
//	// 或使用封装的客户端
//	client := elasticsearch.GetClient()
//	info, _ := client.Info(ctx)
//
// Elastic Cloud:
//
//	err := elasticsearch.Init(&elasticsearch.Config{
//	    CloudID:  "my-deployment:xxxx",
//	    APIKey:   "your-api-key",
//	})
//
// 健康检查:
//
//	if err := elasticsearch.GetClient().Ping(ctx); err != nil {
//	    // 处理不健康状态
//	}
//
// --- English ---
//
// Package elasticsearch provides Elasticsearch client singleton management.
//
// It wraps the official Elasticsearch Go client with singleton pattern,
// health checks, and graceful shutdown.
//
// Basic usage:
//
//	// Initialize at application startup
//	err := elasticsearch.Init(&elasticsearch.Config{
//	    Addresses: []string{"http://localhost:9200"},
//	    Username:  "elastic",
//	    Password:  "password",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer elasticsearch.Close()
//
//	// Use the client
//	es := elasticsearch.ES()
//	res, _ := es.Index("users", strings.NewReader(`{"name":"test"}`))
//
//	// Or use the wrapped client
//	client := elasticsearch.GetClient()
//	info, _ := client.Info(ctx)
//
// Elastic Cloud:
//
//	err := elasticsearch.Init(&elasticsearch.Config{
//	    CloudID:  "my-deployment:xxxx",
//	    APIKey:   "your-api-key",
//	})
//
// Health check:
//
//	if err := elasticsearch.GetClient().Ping(ctx); err != nil {
//	    // handle unhealthy
//	}
package elasticsearch
