// Package idgen 提供 ID 生成工具
//
// 包括雪花算法 ID 生成器和 UUID 工具。
//
// 雪花算法用法:
//
//	gen := idgen.NewSnowflake(1)  // 节点 ID
//	id := gen.Generate()
//
// UUID 用法:
//
//	uuid := idgen.NewUUID()        // v4 UUID
//	uuid := idgen.NewUUIDString()  // 字符串形式
//
// --- English ---
//
// Package idgen provides ID generation utilities.
//
// Includes Snowflake ID generator and UUID utilities.
//
// Snowflake usage:
//
//	gen := idgen.NewSnowflake(1)  // node ID
//	id := gen.Generate()
//
// UUID usage:
//
//	uuid := idgen.NewUUID()        // v4 UUID
//	uuid := idgen.NewUUIDString()  // string representation
package idgen
