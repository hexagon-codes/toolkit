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
