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
