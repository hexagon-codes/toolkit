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
