// Package idgen 提供 ID 生成工具
//
// 包括雪花算法 ID 生成器和 UUID 工具。
//
// 雪花算法用法:
//
//	gen, err := idgen.NewSnowflake(1) // 节点 ID
//	if err != nil {
//	    return err
//	}
//	id, err := gen.GenerateSafe()
//
// UUID 用法:
//
//	uuid, err := idgen.TryUUID()                  // v4 UUID
//	compact, err := idgen.TryUUIDWithoutHyphen() // 无连字符形式
package idgen
