package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func setupMiniRedis(t *testing.T) (*miniredis.Miniredis, *Client) {
	t.Helper()

	// 启动 miniredis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	// 创建配置
	cfg := DefaultConfig(mr.Addr())
	cfg.DialTimeout = 1 * time.Second

	// 创建客户端
	client, err := New(cfg)
	if err != nil {
		mr.Close()
		t.Fatalf("failed to create redis client: %v", err)
	}

	return mr, client
}

func TestNew(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	if client == nil {
		t.Fatal("expected client to be created")
	}

	if client.config == nil {
		t.Fatal("expected config to be set")
	}
}

func TestNewWithNilConfig(t *testing.T) {
	client, err := New(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}

	if client != nil {
		t.Error("expected nil client for nil config")
	}
}

func TestNewWithInvalidMode(t *testing.T) {
	cfg := DefaultConfig("localhost:6379")
	cfg.Mode = "invalid"

	client, err := New(cfg)
	if err == nil {
		t.Error("expected error for invalid mode")
	}

	if client != nil {
		client.Close()
		t.Error("expected nil client for invalid mode")
	}
}

func TestHealth(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// 正常情况
	err := client.Health(ctx)
	if err != nil {
		t.Errorf("expected health check to pass, got error: %v", err)
	}

	// 关闭 Redis
	mr.Close()

	// 健康检查应该失败
	err = client.Health(ctx)
	if err == nil {
		t.Error("expected health check to fail after closing redis")
	}
}

func TestHealthWithNilClient(t *testing.T) {
	var client *Client
	ctx := context.Background()

	err := client.Health(ctx)
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestGetWithDefault(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// 设置值
	key := "test-key"
	value := "test-value"
	err := client.Set(ctx, key, value, 0).Err()
	if err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	// 获取存在的值
	result := client.GetWithDefault(ctx, key, "default")
	if result != value {
		t.Errorf("expected %s, got %s", value, result)
	}

	// 获取不存在的值
	defaultValue := "default-value"
	result = client.GetWithDefault(ctx, "non-existent", defaultValue)
	if result != defaultValue {
		t.Errorf("expected %s, got %s", defaultValue, result)
	}
}

func TestSetWithExpire(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	key := "expire-key"
	value := "expire-value"
	expiration := 5 * time.Second // miniredis 的最小 TTL 是 1 秒

	// 设置带过期时间的值
	err := client.SetWithExpire(ctx, key, value, expiration)
	if err != nil {
		t.Fatalf("failed to set key with expire: %v", err)
	}

	// 验证值存在
	result, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}

	if result != value {
		t.Errorf("expected %s, got %s", value, result)
	}

	// 验证 TTL
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}

	if ttl <= 0 || ttl > expiration {
		t.Errorf("expected TTL between 0 and %v, got %v", expiration, ttl)
	}
}

func TestSetNX(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	key := "nx-key"
	value := "nx-value"

	// 第一次设置应该成功
	ok, err := client.SetNX(ctx, key, value, 0)
	if err != nil {
		t.Fatalf("failed to setnx: %v", err)
	}

	if !ok {
		t.Error("expected setnx to succeed for non-existent key")
	}

	// 第二次设置应该失败
	ok, err = client.SetNX(ctx, key, "new-value", 0)
	if err != nil {
		t.Fatalf("failed to setnx: %v", err)
	}

	if ok {
		t.Error("expected setnx to fail for existing key")
	}

	// 验证值未被覆盖
	result, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}

	if result != value {
		t.Errorf("expected %s, got %s", value, result)
	}
}

func TestSetNXEx(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	key := "nxex-key"
	value := "nxex-value"
	expiration := 5 * time.Second // miniredis 的最小 TTL 是 1 秒

	// 设置成功
	ok, err := client.SetNXEx(ctx, key, value, expiration)
	if err != nil {
		t.Fatalf("failed to setnxex: %v", err)
	}

	if !ok {
		t.Error("expected setnxex to succeed")
	}

	// 验证 TTL
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}

	if ttl <= 0 || ttl > expiration {
		t.Errorf("expected TTL between 0 and %v, got %v", expiration, ttl)
	}
}

func TestMGetValues(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// 设置多个值
	keys := []string{"mget-key1", "mget-key2", "mget-key3"}
	values := []string{"value1", "value2", "value3"}

	for i, key := range keys {
		err := client.Set(ctx, key, values[i], 0).Err()
		if err != nil {
			t.Fatalf("failed to set key %s: %v", key, err)
		}
	}

	// 批量获取
	results, err := client.MGetValues(ctx, keys...)
	if err != nil {
		t.Fatalf("failed to mget: %v", err)
	}

	if len(results) != len(keys) {
		t.Errorf("expected %d results, got %d", len(keys), len(results))
	}

	for i, result := range results {
		if result == nil {
			t.Errorf("expected result[%d] to be non-nil", i)
			continue
		}

		if result.(string) != values[i] {
			t.Errorf("expected result[%d] %s, got %s", i, values[i], result)
		}
	}
}

func TestMSetValues(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// 批量设置
	err := client.MSetValues(ctx, "mset-key1", "value1", "mset-key2", "value2")
	if err != nil {
		t.Fatalf("failed to mset: %v", err)
	}

	// 验证值
	val1, err := client.Get(ctx, "mset-key1").Result()
	if err != nil {
		t.Fatalf("failed to get mset-key1: %v", err)
	}

	if val1 != "value1" {
		t.Errorf("expected value1, got %s", val1)
	}

	val2, err := client.Get(ctx, "mset-key2").Result()
	if err != nil {
		t.Fatalf("failed to get mset-key2: %v", err)
	}

	if val2 != "value2" {
		t.Errorf("expected value2, got %s", val2)
	}
}

func TestIncrByWithExpire(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	key := "incr-key"
	expiration := 100 * time.Millisecond

	// 第一次自增
	val, err := client.IncrByWithExpire(ctx, key, 5, expiration)
	if err != nil {
		t.Fatalf("failed to incr: %v", err)
	}

	if val != 5 {
		t.Errorf("expected 5, got %d", val)
	}

	// 第二次自增
	val, err = client.IncrByWithExpire(ctx, key, 3, expiration)
	if err != nil {
		t.Fatalf("failed to incr: %v", err)
	}

	if val != 8 {
		t.Errorf("expected 8, got %d", val)
	}

	// 验证 TTL
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}

	if ttl <= 0 {
		t.Errorf("expected positive TTL, got %v", ttl)
	}
}

func TestExistsCount(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// 设置一些键
	keys := []string{"exists-key1", "exists-key2"}
	for _, key := range keys {
		err := client.Set(ctx, key, "value", 0).Err()
		if err != nil {
			t.Fatalf("failed to set key %s: %v", key, err)
		}
	}

	// 检查存在
	count, err := client.ExistsCount(ctx, keys...)
	if err != nil {
		t.Fatalf("failed to check exists: %v", err)
	}

	if count != int64(len(keys)) {
		t.Errorf("expected count %d, got %d", len(keys), count)
	}

	// 检查不存在的键
	count, err = client.ExistsCount(ctx, "non-existent")
	if err != nil {
		t.Fatalf("failed to check exists: %v", err)
	}

	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

func TestDeleteKeys(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// 设置键
	key := "delete-key"
	err := client.Set(ctx, key, "value", 0).Err()
	if err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	// 删除键
	err = client.DeleteKeys(ctx, key)
	if err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}

	// 验证键不存在
	_, err = client.Get(ctx, key).Result()
	if err == nil {
		t.Error("expected key to be deleted")
	}
}

func TestSetExpireAt(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	key := "expireat-key"
	value := "expireat-value"

	// 设置键
	err := client.Set(ctx, key, value, 0).Err()
	if err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	// 设置过期时间戳
	expireAt := time.Now().Add(1 * time.Hour)
	err = client.SetExpireAt(ctx, key, expireAt)
	if err != nil {
		t.Fatalf("failed to set expireat: %v", err)
	}

	// 验证 TTL
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}

	if ttl <= 0 || ttl > 1*time.Hour {
		t.Errorf("expected TTL around 1 hour, got %v", ttl)
	}
}

func TestGetTTL(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	key := "ttl-key"
	expiration := 1 * time.Hour

	// 设置带过期时间的键
	err := client.Set(ctx, key, "value", expiration).Err()
	if err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	// 获取 TTL
	ttl, err := client.GetTTL(ctx, key)
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}

	if ttl <= 0 || ttl > expiration {
		t.Errorf("expected TTL around %v, got %v", expiration, ttl)
	}

	// 不存在的键
	ttl, err = client.GetTTL(ctx, "non-existent")
	if err != nil {
		t.Fatalf("failed to get TTL for non-existent key: %v", err)
	}

	if ttl >= 0 {
		t.Errorf("expected negative TTL for non-existent key, got %v", ttl)
	}
}

func TestClose(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()

	err := client.Close()
	if err != nil {
		t.Errorf("expected close to succeed, got error: %v", err)
	}

	// 关闭后操作应该失败
	ctx := context.Background()
	err = client.Set(ctx, "key", "value", 0).Err()
	if err == nil {
		t.Error("expected error after closing client")
	}
}

func TestCloseNilClient(t *testing.T) {
	var client *Client
	err := client.Close()
	if err != nil {
		t.Errorf("expected no error for nil client, got: %v", err)
	}
}

func TestStats(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	stats := client.Stats()
	if stats == nil {
		t.Error("expected stats to be non-nil")
	}
}

func TestStatsNilClient(t *testing.T) {
	var client *Client
	stats := client.Stats()
	if stats != nil {
		t.Error("expected nil stats for nil client")
	}
}

func TestNewClusterMode(t *testing.T) {
	// 集群模式需要多个节点，这里只测试配置转换
	cfg := DefaultClusterConfig([]string{"localhost:7000", "localhost:7001"})

	// 由于没有真实的集群环境，我们只验证配置
	if cfg.Mode != ModeCluster {
		t.Errorf("expected mode %s, got %s", ModeCluster, cfg.Mode)
	}

	if len(cfg.Addrs) != 2 {
		t.Errorf("expected 2 addrs, got %d", len(cfg.Addrs))
	}
}

func TestNewSentinelMode(t *testing.T) {
	// 哨兵模式需要特殊设置，这里只测试配置
	cfg := DefaultConfig("localhost:6379")
	cfg.Mode = ModeSentinel
	cfg.MasterName = "mymaster"
	cfg.SentinelAddrs = []string{"localhost:26379"}

	if cfg.Mode != ModeSentinel {
		t.Errorf("expected mode %s, got %s", ModeSentinel, cfg.Mode)
	}

	if cfg.MasterName != "mymaster" {
		t.Errorf("expected MasterName mymaster, got %s", cfg.MasterName)
	}
}

func TestPipeline(t *testing.T) {
	mr, client := setupMiniRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	// 使用 pipeline 批量操作
	pipe := client.Pipeline()
	pipe.Set(ctx, "pipe-key1", "value1", 0)
	pipe.Set(ctx, "pipe-key2", "value2", 0)
	pipe.Incr(ctx, "pipe-counter")

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		t.Fatalf("failed to exec pipeline: %v", err)
	}

	if len(cmds) != 3 {
		t.Errorf("expected 3 commands, got %d", len(cmds))
	}

	// 验证结果
	val1, err := client.Get(ctx, "pipe-key1").Result()
	if err != nil {
		t.Fatalf("failed to get pipe-key1: %v", err)
	}

	if val1 != "value1" {
		t.Errorf("expected value1, got %s", val1)
	}
}

func TestGetGlobal(t *testing.T) {
	// 由于 Init 使用 sync.Once，我们无法重置全局状态
	// 这里只测试 GetGlobal 不会 panic
	client := GetGlobal()
	_ = client // 可能为 nil，但不应该 panic
}
