// Package mongodb 提供 MongoDB 客户端单例管理
//
// 封装官方 MongoDB Go 驱动，提供单例模式、连接池、健康检查和优雅关闭功能。
//
// 基本用法:
//
//	// 应用启动时初始化
//	err := mongodb.Init(ctx, &mongodb.Config{
//	    URI:      "mongodb://localhost:27017",
//	    Database: "myapp",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mongodb.Close()
//
//	// 在应用中使用
//	coll := mongodb.Collection("users")
//	coll.InsertOne(ctx, bson.M{"name": "test"})
//
//	// 或直接获取客户端
//	client := mongodb.GetClient()
//	client.Coll("orders").Find(ctx, bson.M{})
//
// 健康检查:
//
//	if err := mongodb.GetClient().Ping(ctx); err != nil {
//	    // 处理不健康状态
//	}
//
// --- English ---
//
// Package mongodb provides MongoDB client singleton management.
//
// It wraps the official MongoDB Go driver with singleton pattern,
// connection pooling, health checks, and graceful shutdown.
//
// Basic usage:
//
//	// Initialize at application startup
//	err := mongodb.Init(ctx, &mongodb.Config{
//	    URI:      "mongodb://localhost:27017",
//	    Database: "myapp",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer mongodb.Close()
//
//	// Use in your application
//	coll := mongodb.Collection("users")
//	coll.InsertOne(ctx, bson.M{"name": "test"})
//
//	// Or get the client directly
//	client := mongodb.GetClient()
//	client.Coll("orders").Find(ctx, bson.M{})
//
// Health check:
//
//	if err := mongodb.GetClient().Ping(ctx); err != nil {
//	    // handle unhealthy
//	}
package mongodb
