package redis_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	cache "github.com/hexagon-codes/toolkit/cache/redis"
)

func TestPackageDocumentationUsesCurrentCacheAPI(t *testing.T) {
	source, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	documentation := string(source)

	for _, forbidden := range []string{
		"redis.New(redisClient)",
		"c.Get(ctx,",
	} {
		if strings.Contains(documentation, forbidden) {
			t.Errorf("package documentation references removed API %q", forbidden)
		}
	}
	for _, required := range []string{
		"redis.NewStableCache(redisClient)",
		"c.GetOrLoad(ctx,",
		"c.Set(ctx,",
		"c.Del(ctx,",
	} {
		if !strings.Contains(documentation, required) {
			t.Errorf("package documentation must demonstrate current API %q", required)
		}
	}
}

func ExampleStableCache() {
	var redisClient goredis.UniversalClient
	c := cache.NewStableCache(redisClient)
	ctx := context.Background()

	var dest string
	_ = c.GetOrLoad(ctx, "key", 5*time.Minute, &dest, func(context.Context) (any, error) {
		return "value", nil
	})
	_ = c.Set(ctx, "key", "value", 5*time.Minute)
	_ = c.Del(ctx, "key")
}
