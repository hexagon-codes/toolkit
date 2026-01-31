package multi

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// mockLayer 模拟缓存层
type mockLayer struct {
	data        map[string]any
	callCount   int
	shouldErr   bool
	errToReturn error
}

func newMockLayer() *mockLayer {
	return &mockLayer{
		data: make(map[string]any),
	}
}

func (m *mockLayer) GetOrLoad(ctx context.Context, key string, ttl time.Duration, dest any, loader func(ctx context.Context) (any, error)) error {
	m.callCount++

	if m.shouldErr {
		return m.errToReturn
	}

	// 检查缓存
	if val, ok := m.data[key]; ok {
		// 模拟数据复制
		if ptr, ok := dest.(*string); ok {
			if str, ok := val.(string); ok {
				*ptr = str
			}
		} else if ptr, ok := dest.(*int); ok {
			if i, ok := val.(int); ok {
				*ptr = i
			}
		}
		return nil
	}

	// 缓存未命中，调用 loader
	val, err := loader(ctx)
	if err != nil {
		return err
	}

	// 写入缓存
	m.data[key] = val

	// 模拟数据复制
	if ptr, ok := dest.(*string); ok {
		if str, ok := val.(string); ok {
			*ptr = str
		}
	} else if ptr, ok := dest.(*int); ok {
		if i, ok := val.(int); ok {
			*ptr = i
		}
	}

	return nil
}

func (m *mockLayer) Del(ctx context.Context, keys ...string) error {
	if m.shouldErr {
		return m.errToReturn
	}

	for _, key := range keys {
		delete(m.data, key)
	}
	return nil
}

func TestNewCache(t *testing.T) {
	layer1 := newMockLayer()
	layer2 := newMockLayer()

	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 10 * time.Minute, Name: "local"},
		{Layer: layer2, TTL: 60 * time.Minute, Name: "redis"},
	})

	if cache == nil {
		t.Fatal("cache is nil")
	}

	if cache.LayerCount() != 2 {
		t.Errorf("expected 2 layers, got %d", cache.LayerCount())
	}
}

func TestCache_GetOrLoad_InvalidParams(t *testing.T) {
	cache := NewCache([]LayerConfig{
		{Layer: newMockLayer(), TTL: time.Minute, Name: "test"},
	})

	ctx := context.Background()

	// 空 key
	var dest string
	err := cache.GetOrLoad(ctx, "", &dest, func(ctx context.Context) (any, error) {
		return "value", nil
	})
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("expected ErrInvalidKey, got: %v", err)
	}

	// nil loader
	err = cache.GetOrLoad(ctx, "key", &dest, nil)
	if !errors.Is(err, ErrInvalidLoader) {
		t.Errorf("expected ErrInvalidLoader, got: %v", err)
	}

	// nil dest
	err = cache.GetOrLoad(ctx, "key", nil, func(ctx context.Context) (any, error) {
		return "value", nil
	})
	if !errors.Is(err, ErrInvalidDest) {
		t.Errorf("expected ErrInvalidDest, got: %v", err)
	}
}

func TestCache_GetOrLoad_NoLayers(t *testing.T) {
	cache := NewCache([]LayerConfig{})

	ctx := context.Background()
	var dest string
	err := cache.GetOrLoad(ctx, "key", &dest, func(ctx context.Context) (any, error) {
		return "value", nil
	})

	if !errors.Is(err, ErrNoLayers) {
		t.Errorf("expected ErrNoLayers, got: %v", err)
	}
}

func TestCache_GetOrLoad_SingleLayer(t *testing.T) {
	layer := newMockLayer()
	cache := NewCache([]LayerConfig{
		{Layer: layer, TTL: time.Minute, Name: "test"},
	})

	ctx := context.Background()
	loadCount := 0

	// 第一次加载
	var dest string
	err := cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loadCount++
		return "value1", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	if dest != "value1" {
		t.Errorf("expected 'value1', got '%s'", dest)
	}

	if loadCount != 1 {
		t.Errorf("expected loadCount=1, got %d", loadCount)
	}

	// 第二次加载（应该命中缓存）
	var dest2 string
	err = cache.GetOrLoad(ctx, "key1", &dest2, func(ctx context.Context) (any, error) {
		loadCount++
		return "value2", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	if dest2 != "value1" {
		t.Errorf("expected cached value 'value1', got '%s'", dest2)
	}

	if loadCount != 1 {
		t.Errorf("expected loadCount=1 (cache hit), got %d", loadCount)
	}
}

func TestCache_Del(t *testing.T) {
	layer := newMockLayer()
	cache := NewCache([]LayerConfig{
		{Layer: layer, TTL: time.Minute, Name: "test"},
	})

	ctx := context.Background()

	// 写入缓存
	var dest string
	cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		return "value1", nil
	})

	// 删除
	err := cache.Del(ctx, "key1")
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}

	// 验证删除
	loadCount := 0
	cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loadCount++
		return "new_value", nil
	})

	if loadCount != 1 {
		t.Error("expected loader to be called after Del")
	}
}

func TestCache_Del_NoKeys(t *testing.T) {
	cache := NewCache([]LayerConfig{
		{Layer: newMockLayer(), TTL: time.Minute, Name: "test"},
	})

	err := cache.Del(context.Background())
	if err != nil {
		t.Errorf("Del with no keys should not error: %v", err)
	}
}

func TestCache_OnError(t *testing.T) {
	layer := newMockLayer()
	layer.shouldErr = true
	layer.errToReturn = errors.New("layer error")

	var errorCount atomic.Int32
	cache := NewCache([]LayerConfig{
		{Layer: layer, TTL: time.Minute, Name: "test"},
	}, WithOnError(func(ctx context.Context, layerName, op, key string, err error) {
		errorCount.Add(1)
	}))

	ctx := context.Background()

	// 触发错误（但loader会成功）
	var dest string
	cache.GetOrLoad(ctx, "key", &dest, func(ctx context.Context) (any, error) {
		return "value", nil
	})

	if errorCount.Load() == 0 {
		t.Error("expected OnError to be called")
	}
}

func TestBuilder_Basic(t *testing.T) {
	layer1 := newMockLayer()
	layer2 := newMockLayer()

	cache := NewBuilder().
		WithLocal(layer1, 10*time.Minute).
		WithRedis(layer2, 60*time.Minute).
		Build()

	if cache == nil {
		t.Fatal("cache is nil")
	}

	if cache.LayerCount() != 2 {
		t.Errorf("expected 2 layers, got %d", cache.LayerCount())
	}
}

func TestBuilder_WithOptionsMethod(t *testing.T) {
	errorCount := 0

	cache := NewBuilder().
		WithLayer(newMockLayer(), time.Minute, "test").
		WithOnError(func(ctx context.Context, layer, op, key string, err error) {
			errorCount++
		}).
		WithSkipBackfill(true).
		Build()

	if cache == nil {
		t.Fatal("cache is nil")
	}

	if !cache.opts.SkipBackfill {
		t.Error("expected SkipBackfill=true")
	}
}

func TestErrors(t *testing.T) {
	if ErrNotFound == nil {
		t.Error("ErrNotFound should not be nil")
	}
	if ErrInvalidDest == nil {
		t.Error("ErrInvalidDest should not be nil")
	}
	if ErrInvalidKey == nil {
		t.Error("ErrInvalidKey should not be nil")
	}
	if ErrInvalidLoader == nil {
		t.Error("ErrInvalidLoader should not be nil")
	}
	if ErrNoLayers == nil {
		t.Error("ErrNoLayers should not be nil")
	}
}

func TestOptions(t *testing.T) {
	customErr := errors.New("custom not found")

	cache := NewCache([]LayerConfig{
		{Layer: newMockLayer(), TTL: time.Minute, Name: "test"},
	}, WithIsNotFound(func(err error) bool {
		return errors.Is(err, customErr)
	}))

	// 测试自定义 IsNotFound
	var dest string
	err := cache.GetOrLoad(context.Background(), "key", &dest, func(ctx context.Context) (any, error) {
		return nil, customErr
	})

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound for custom error, got: %v", err)
	}
}

func TestCache_LayerCount(t *testing.T) {
	cache := NewCache([]LayerConfig{
		{Layer: newMockLayer(), TTL: time.Minute, Name: "layer1"},
		{Layer: newMockLayer(), TTL: time.Minute, Name: "layer2"},
		{Layer: newMockLayer(), TTL: time.Minute, Name: "layer3"},
	})

	if cache.LayerCount() != 3 {
		t.Errorf("expected 3 layers, got %d", cache.LayerCount())
	}
}

func TestCache_MultiLayer_LocalMiss_RedisHit(t *testing.T) {
	localLayer := newMockLayer()
	redisLayer := newMockLayer()

	// Pre-populate redis
	redisLayer.data["key1"] = "redis_value"

	cache := NewCache([]LayerConfig{
		{Layer: localLayer, TTL: 10 * time.Minute, Name: "local"},
		{Layer: redisLayer, TTL: 60 * time.Minute, Name: "redis"},
	})

	ctx := context.Background()
	var dest string
	loaderCalled := false

	err := cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loaderCalled = true
		return "db_value", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	// Should have got value from redis, loader should not be called
	if dest != "redis_value" {
		t.Errorf("expected 'redis_value', got '%s'", dest)
	}

	if loaderCalled {
		t.Error("loader should not be called when redis has data")
	}
}

func TestCache_Del_MultipleKeys(t *testing.T) {
	layer := newMockLayer()
	layer.data["key1"] = "value1"
	layer.data["key2"] = "value2"
	layer.data["key3"] = "value3"

	cache := NewCache([]LayerConfig{
		{Layer: layer, TTL: time.Minute, Name: "test"},
	})

	ctx := context.Background()
	err := cache.Del(ctx, "key1", "key2")
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}

	// key1 and key2 should be deleted
	if _, exists := layer.data["key1"]; exists {
		t.Error("key1 should be deleted")
	}
	if _, exists := layer.data["key2"]; exists {
		t.Error("key2 should be deleted")
	}
	// key3 should still exist
	if _, exists := layer.data["key3"]; !exists {
		t.Error("key3 should still exist")
	}
}

func TestCache_Del_Error(t *testing.T) {
	layer := newMockLayer()
	layer.shouldErr = true
	layer.errToReturn = errors.New("del error")

	errorCount := 0
	cache := NewCache([]LayerConfig{
		{Layer: layer, TTL: time.Minute, Name: "test"},
	}, WithOnError(func(ctx context.Context, layerName, op, key string, err error) {
		errorCount++
	}))

	ctx := context.Background()
	err := cache.Del(ctx, "key1")

	if err == nil {
		t.Error("Del should return error")
	}
	if errorCount != 1 {
		t.Errorf("expected 1 error callback, got %d", errorCount)
	}
}

func TestCache_NotFound(t *testing.T) {
	layer := newMockLayer()

	cache := NewCache([]LayerConfig{
		{Layer: layer, TTL: time.Minute, Name: "test"},
	})

	ctx := context.Background()
	var dest string

	err := cache.GetOrLoad(ctx, "missing", &dest, func(ctx context.Context) (any, error) {
		return nil, ErrNotFound
	})

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestCache_isNotFound_NilError(t *testing.T) {
	cache := NewCache([]LayerConfig{
		{Layer: newMockLayer(), TTL: time.Minute, Name: "test"},
	})

	if cache.isNotFound(nil) {
		t.Error("isNotFound(nil) should return false")
	}
}

func TestCache_LoaderError(t *testing.T) {
	layer := newMockLayer()

	cache := NewCache([]LayerConfig{
		{Layer: layer, TTL: time.Minute, Name: "test"},
	})

	ctx := context.Background()
	var dest string
	customErr := errors.New("custom loader error")

	err := cache.GetOrLoad(ctx, "key", &dest, func(ctx context.Context) (any, error) {
		return nil, customErr
	})

	if !errors.Is(err, customErr) {
		t.Errorf("expected customErr, got: %v", err)
	}
}

func TestCache_AllLayersFail_LoaderSuccess(t *testing.T) {
	layer1 := newMockLayer()
	layer1.shouldErr = true
	layer1.errToReturn = errors.New("layer1 error")

	layer2 := newMockLayer()
	layer2.shouldErr = true
	layer2.errToReturn = errors.New("layer2 error")

	var errorCount int32
	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: time.Minute, Name: "layer1"},
		{Layer: layer2, TTL: time.Minute, Name: "layer2"},
	}, WithOnError(func(ctx context.Context, layerName, op, key string, err error) {
		atomic.AddInt32(&errorCount, 1)
	}))

	ctx := context.Background()
	var dest string

	err := cache.GetOrLoad(ctx, "key", &dest, func(ctx context.Context) (any, error) {
		return "loader_value", nil
	})

	// Should succeed because loader succeeds
	if err != nil {
		t.Fatalf("GetOrLoad should succeed: %v", err)
	}

	// Wait for background backfill goroutines to complete
	time.Sleep(100 * time.Millisecond)

	// Error callback should be called for both layers during Get (2) and backfill (2)
	if count := atomic.LoadInt32(&errorCount); count != 4 {
		t.Errorf("expected 4 error callbacks, got %d", count)
	}
}

func TestCache_SkipBackfill(t *testing.T) {
	layer := newMockLayer()

	cache := NewCache([]LayerConfig{
		{Layer: layer, TTL: time.Minute, Name: "test"},
	}, WithSkipBackfill(true))

	if !cache.opts.SkipBackfill {
		t.Error("SkipBackfill should be true")
	}
}

func TestBuilder_WithIsNotFound(t *testing.T) {
	customNotFound := errors.New("custom not found")

	cache := NewBuilder().
		WithLayer(newMockLayer(), time.Minute, "test").
		WithIsNotFound(func(err error) bool {
			return errors.Is(err, customNotFound)
		}).
		Build()

	if cache.opts.IsNotFound == nil {
		t.Fatal("IsNotFound should not be nil")
	}

	// Test with custom not found error
	ctx := context.Background()
	var dest string
	err := cache.GetOrLoad(ctx, "key", &dest, func(ctx context.Context) (any, error) {
		return nil, customNotFound
	})

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestBuilder_WithOptions(t *testing.T) {
	cache := NewBuilder().
		WithLayer(newMockLayer(), time.Minute, "test").
		WithOptions(WithSkipBackfill(true)).
		Build()

	if !cache.opts.SkipBackfill {
		t.Error("SkipBackfill should be true")
	}
}

func TestApplyOptions_NilFn(t *testing.T) {
	// Should not panic with nil option
	opts := applyOptions(nil, WithSkipBackfill(true), nil)

	if !opts.SkipBackfill {
		t.Error("SkipBackfill should be true")
	}
}

func TestApplyOptions_NilIsNotFound(t *testing.T) {
	opts := applyOptions(WithIsNotFound(nil))

	// Should default to checking ErrNotFound
	if opts.IsNotFound == nil {
		t.Error("IsNotFound should have default function")
	}

	// Test default behavior
	if !opts.IsNotFound(ErrNotFound) {
		t.Error("default IsNotFound should return true for ErrNotFound")
	}
}

func TestCopyValue(t *testing.T) {
	// Test copyValue function
	var dest string
	err := copyValue("source", &dest)
	if err != nil {
		t.Errorf("copyValue should not error: %v", err)
	}
}

func TestCache_IntValues(t *testing.T) {
	layer := newMockLayer()

	cache := NewCache([]LayerConfig{
		{Layer: layer, TTL: time.Minute, Name: "test"},
	})

	ctx := context.Background()
	var dest int

	err := cache.GetOrLoad(ctx, "int_key", &dest, func(ctx context.Context) (any, error) {
		return 42, nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	if dest != 42 {
		t.Errorf("expected 42, got %d", dest)
	}

	// Second call should hit cache
	var dest2 int
	err = cache.GetOrLoad(ctx, "int_key", &dest2, func(ctx context.Context) (any, error) {
		return 100, nil // Different value, but should get cached value
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	if dest2 != 42 {
		t.Errorf("expected cached value 42, got %d", dest2)
	}
}

// ============================================================================
// 3-Layer Tests (for loadFromNextLayers and backfillRange coverage)
// ============================================================================

func TestCache_ThreeLayer_MiddleLayerHit(t *testing.T) {
	layer1 := newMockLayer() // local - empty
	layer2 := newMockLayer() // redis - has data
	layer3 := newMockLayer() // db cache - empty

	// Pre-populate middle layer
	layer2.data["key1"] = "redis_value"

	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 5 * time.Minute, Name: "local"},
		{Layer: layer2, TTL: 30 * time.Minute, Name: "redis"},
		{Layer: layer3, TTL: 60 * time.Minute, Name: "db_cache"},
	})

	ctx := context.Background()
	var dest string
	loaderCalled := false

	err := cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loaderCalled = true
		return "db_value", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	// Should have got value from redis (middle layer)
	if dest != "redis_value" {
		t.Errorf("expected 'redis_value', got '%s'", dest)
	}

	if loaderCalled {
		t.Error("loader should not be called when middle layer has data")
	}

	// Wait for backfill to complete
	time.Sleep(100 * time.Millisecond)

	// Now local should have the value backfilled
	if _, exists := layer1.data["key1"]; !exists {
		t.Error("expected local layer to have backfilled data")
	}
}

func TestCache_ThreeLayer_LastLayerHit(t *testing.T) {
	layer1 := newMockLayer() // local - empty
	layer2 := newMockLayer() // redis - empty
	layer3 := newMockLayer() // db cache - has data

	// Pre-populate last layer
	layer3.data["key1"] = "dbcache_value"

	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 5 * time.Minute, Name: "local"},
		{Layer: layer2, TTL: 30 * time.Minute, Name: "redis"},
		{Layer: layer3, TTL: 60 * time.Minute, Name: "db_cache"},
	})

	ctx := context.Background()
	var dest string
	loaderCalled := false

	err := cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loaderCalled = true
		return "db_value", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	// Should have got value from last layer
	if dest != "dbcache_value" {
		t.Errorf("expected 'dbcache_value', got '%s'", dest)
	}

	if loaderCalled {
		t.Error("loader should not be called when last layer has data")
	}

	// Wait for backfill to complete
	time.Sleep(100 * time.Millisecond)
}

func TestCache_ThreeLayer_AllMiss_LoaderSuccess(t *testing.T) {
	layer1 := newMockLayer()
	layer2 := newMockLayer()
	layer3 := newMockLayer()

	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 5 * time.Minute, Name: "local"},
		{Layer: layer2, TTL: 30 * time.Minute, Name: "redis"},
		{Layer: layer3, TTL: 60 * time.Minute, Name: "db_cache"},
	})

	ctx := context.Background()
	var dest string
	loaderCallCount := 0

	err := cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loaderCallCount++
		return "loaded_value", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	if dest != "loaded_value" {
		t.Errorf("expected 'loaded_value', got '%s'", dest)
	}

	if loaderCallCount != 1 {
		t.Errorf("expected loader called once, got %d", loaderCallCount)
	}

	// Wait for backfill to complete
	time.Sleep(100 * time.Millisecond)

	// All layers should have data now
	if _, exists := layer1.data["key1"]; !exists {
		t.Error("expected layer1 to have data")
	}
	if _, exists := layer2.data["key1"]; !exists {
		t.Error("expected layer2 to have data")
	}
	if _, exists := layer3.data["key1"]; !exists {
		t.Error("expected layer3 to have data")
	}
}

func TestCache_ThreeLayer_FirstLayerError_MiddleLayerHit(t *testing.T) {
	layer1 := newMockLayer()
	layer1.shouldErr = true
	layer1.errToReturn = errors.New("layer1 error")

	layer2 := newMockLayer()
	layer2.data["key1"] = "redis_value"

	layer3 := newMockLayer()

	var errorCount int32
	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 5 * time.Minute, Name: "local"},
		{Layer: layer2, TTL: 30 * time.Minute, Name: "redis"},
		{Layer: layer3, TTL: 60 * time.Minute, Name: "db_cache"},
	}, WithOnError(func(ctx context.Context, layerName, op, key string, err error) {
		atomic.AddInt32(&errorCount, 1)
	}))

	ctx := context.Background()
	var dest string
	loaderCalled := false

	err := cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loaderCalled = true
		return "db_value", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	if dest != "redis_value" {
		t.Errorf("expected 'redis_value', got '%s'", dest)
	}

	if loaderCalled {
		t.Error("loader should not be called")
	}

	// Wait for backfill
	time.Sleep(100 * time.Millisecond)

	// Error should have been called for layer1
	if atomic.LoadInt32(&errorCount) == 0 {
		t.Error("expected error callback for layer1")
	}
}

func TestCache_ThreeLayer_NotFound_PropagatesCorrectly(t *testing.T) {
	layer1 := newMockLayer()
	layer2 := newMockLayer()
	layer3 := newMockLayer()

	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 5 * time.Minute, Name: "local"},
		{Layer: layer2, TTL: 30 * time.Minute, Name: "redis"},
		{Layer: layer3, TTL: 60 * time.Minute, Name: "db_cache"},
	})

	ctx := context.Background()
	var dest string

	err := cache.GetOrLoad(ctx, "missing_key", &dest, func(ctx context.Context) (any, error) {
		return nil, ErrNotFound
	})

	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestCache_ThreeLayer_WithSkipBackfill(t *testing.T) {
	// SkipBackfill prevents explicit backfillAll calls after loader success
	// The first layer still caches data through normal GetOrLoad flow
	layer1 := newMockLayer()
	layer2 := newMockLayer()
	layer3 := newMockLayer()

	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 5 * time.Minute, Name: "local"},
		{Layer: layer2, TTL: 30 * time.Minute, Name: "redis"},
		{Layer: layer3, TTL: 60 * time.Minute, Name: "db_cache"},
	}, WithSkipBackfill(true))

	if !cache.opts.SkipBackfill {
		t.Error("SkipBackfill should be true")
	}

	ctx := context.Background()
	var dest string
	loaderCalled := false

	// All layers miss, loader succeeds
	err := cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loaderCalled = true
		return "loaded_value", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	if dest != "loaded_value" {
		t.Errorf("expected 'loaded_value', got '%s'", dest)
	}

	if !loaderCalled {
		t.Error("loader should be called when all layers miss")
	}

	// Wait to ensure any backfill would have happened
	time.Sleep(100 * time.Millisecond)

	// Layer1 will have data because GetOrLoad naturally caches
	// This is expected - SkipBackfill only affects explicit backfill calls
	if _, exists := layer1.data["key1"]; !exists {
		t.Error("layer1 should have data from normal cache flow")
	}
}

func TestCache_ThreeLayer_MiddleLayerError_LastLayerHit(t *testing.T) {
	// This test exercises loadFromNextLayers and backfillRange
	// When middle layer errors (non-NotFound), the loop continues to next layer
	// When next layer hits, backfillRange is called

	layer1 := newMockLayer() // local - empty
	layer2 := newMockLayer() // redis - errors
	layer2.shouldErr = true
	layer2.errToReturn = errors.New("redis connection error")
	layer3 := newMockLayer() // db cache - has data
	layer3.data["key1"] = "dbcache_value"

	var errorCount int32
	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 5 * time.Minute, Name: "local"},
		{Layer: layer2, TTL: 30 * time.Minute, Name: "redis"},
		{Layer: layer3, TTL: 60 * time.Minute, Name: "db_cache"},
	}, WithOnError(func(ctx context.Context, layerName, op, key string, err error) {
		atomic.AddInt32(&errorCount, 1)
	}))

	ctx := context.Background()
	var dest string
	loaderCalled := false

	err := cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loaderCalled = true
		return "db_value", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	// Should have got value from layer3
	if dest != "dbcache_value" {
		t.Errorf("expected 'dbcache_value', got '%s'", dest)
	}

	if loaderCalled {
		t.Error("loader should not be called when layer3 has data")
	}

	// Error callback should have been called for layer2
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&errorCount) == 0 {
		t.Error("expected error callback for layer2")
	}
}

func TestCache_FourLayer_BackfillRange(t *testing.T) {
	// 4-layer test to exercise backfillRange more thoroughly
	layer1 := newMockLayer() // empty
	layer2 := newMockLayer() // errors
	layer2.shouldErr = true
	layer2.errToReturn = errors.New("layer2 error")
	layer3 := newMockLayer() // empty
	layer4 := newMockLayer() // has data
	layer4.data["key1"] = "layer4_value"

	var errorCount int32
	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 5 * time.Minute, Name: "layer1"},
		{Layer: layer2, TTL: 10 * time.Minute, Name: "layer2"},
		{Layer: layer3, TTL: 30 * time.Minute, Name: "layer3"},
		{Layer: layer4, TTL: 60 * time.Minute, Name: "layer4"},
	}, WithOnError(func(ctx context.Context, layerName, op, key string, err error) {
		atomic.AddInt32(&errorCount, 1)
	}))

	ctx := context.Background()
	var dest string
	loaderCalled := false

	err := cache.GetOrLoad(ctx, "key1", &dest, func(ctx context.Context) (any, error) {
		loaderCalled = true
		return "loaded_value", nil
	})

	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	if dest != "layer4_value" {
		t.Errorf("expected 'layer4_value', got '%s'", dest)
	}

	if loaderCalled {
		t.Error("loader should not be called when layer4 has data")
	}

	time.Sleep(100 * time.Millisecond)
}

func TestCache_Del_ThreeLayers(t *testing.T) {
	layer1 := newMockLayer()
	layer1.data["key1"] = "value1"
	layer2 := newMockLayer()
	layer2.data["key1"] = "value1"
	layer3 := newMockLayer()
	layer3.data["key1"] = "value1"

	cache := NewCache([]LayerConfig{
		{Layer: layer1, TTL: 5 * time.Minute, Name: "local"},
		{Layer: layer2, TTL: 30 * time.Minute, Name: "redis"},
		{Layer: layer3, TTL: 60 * time.Minute, Name: "db_cache"},
	})

	ctx := context.Background()
	err := cache.Del(ctx, "key1")
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}

	// All layers should have key deleted
	if _, exists := layer1.data["key1"]; exists {
		t.Error("layer1 should not have key1")
	}
	if _, exists := layer2.data["key1"]; exists {
		t.Error("layer2 should not have key1")
	}
	if _, exists := layer3.data["key1"]; exists {
		t.Error("layer3 should not have key1")
	}
}
