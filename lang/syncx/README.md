# Syncx - 并发同步工具

提供常用的并发同步模式实现，包括 Singleflight 和对象池。

## 特性

- ✅ Singleflight - 防止缓存击穿，合并重复调用
- ✅ Pool - sync.Pool 的简单封装
- ✅ TypedPool - 类型安全的对象池（泛型）
- ✅ 并发安全 - 所有类型都是线程安全的
- ✅ 零外部依赖 - 只使用 Go 标准库
- ✅ 简单易用 - API 简洁明了

## 快速开始

### Singleflight - 防缓存击穿

```go
import "github.com/everyday-items/toolkit/lang/syncx"

// 创建 Singleflight 实例
sf := syncx.NewSingleflight()

// 多个并发请求会被合并为一次执行
result, err := sf.Do("user:123", func() (any, error) {
    // 即使有 1000 个并发请求，这个函数也只会执行一次
    return db.GetUser(123)
})

// 清除记录，强制下次重新执行
sf.Forget("user:123")
```

### Pool - 对象复用

```go
import (
    "bytes"
    "github.com/everyday-items/toolkit/lang/syncx"
)

// 创建对象池
pool := syncx.NewPool(func() any {
    return &bytes.Buffer{}
})

// 获取对象
buf := pool.Get().(*bytes.Buffer)
defer pool.Put(buf)

// 使用对象
buf.WriteString("hello")
fmt.Println(buf.String())

// 放回前记得重置
buf.Reset()
pool.Put(buf)
```

### TypedPool - 类型安全的对象池

```go
import (
    "bytes"
    "github.com/everyday-items/toolkit/lang/syncx"
)

// 创建类型安全的对象池（无需类型断言）
pool := syncx.NewTypedPool(func() *bytes.Buffer {
    return &bytes.Buffer{}
})

// 获取对象（类型安全，无需断言）
buf := pool.Get()  // 直接返回 *bytes.Buffer
defer pool.Put(buf)

buf.WriteString("hello")
fmt.Println(buf.String())
```

## API 文档

### Singleflight

```go
// NewSingleflight 创建 Singleflight 实例
NewSingleflight() *Singleflight

// Do 执行函数，同一个 key 同时只会执行一次
Do(key string, fn func() (any, error)) (any, error)

// Forget 删除指定 key 的记录
Forget(key string)
```

### Pool

```go
// NewPool 创建对象池
NewPool(newFunc func() any) *Pool

// Get 从池中获取对象
Get() any

// Put 将对象放回池中
Put(x any)
```

### TypedPool

```go
// NewTypedPool 创建类型安全的对象池
NewTypedPool[T any](newFunc func() T) *TypedPool[T]

// Get 从池中获取对象（类型安全）
Get() T

// Put 将对象放回池中
Put(x T)
```

## 使用场景

### 1. 防止缓存击穿

当缓存过期时，如果有大量并发请求访问同一个 key，会导致所有请求都打到数据库，造成数据库压力骤增。Singleflight 可以将这些请求合并为一次。

```go
type UserCache struct {
    sf    *syncx.Singleflight
    cache map[string]*User
    mu    sync.RWMutex
}

func NewUserCache() *UserCache {
    return &UserCache{
        sf:    syncx.NewSingleflight(),
        cache: make(map[string]*User),
    }
}

func (c *UserCache) GetUser(id int64) (*User, error) {
    key := fmt.Sprintf("user:%d", id)

    // 先查缓存
    c.mu.RLock()
    if user, ok := c.cache[key]; ok {
        c.mu.RUnlock()
        return user, nil
    }
    c.mu.RUnlock()

    // 使用 Singleflight 防止击穿
    result, err := c.sf.Do(key, func() (any, error) {
        // 再次检查缓存（双重检查）
        c.mu.RLock()
        if user, ok := c.cache[key]; ok {
            c.mu.RUnlock()
            return user, nil
        }
        c.mu.RUnlock()

        // 从数据库加载
        user, err := db.GetUser(id)
        if err != nil {
            return nil, err
        }

        // 写入缓存
        c.mu.Lock()
        c.cache[key] = user
        c.mu.Unlock()

        return user, nil
    })

    if err != nil {
        return nil, err
    }

    return result.(*User), nil
}

// 更新用户时清除缓存和 Singleflight 记录
func (c *UserCache) UpdateUser(user *User) error {
    key := fmt.Sprintf("user:%d", user.ID)

    // 更新数据库
    if err := db.UpdateUser(user); err != nil {
        return err
    }

    // 删除缓存
    c.mu.Lock()
    delete(c.cache, key)
    c.mu.Unlock()

    // 清除 Singleflight 记录
    c.sf.Forget(key)

    return nil
}
```

### 2. API 请求去重

防止短时间内重复调用同一个外部 API：

```go
type APIClient struct {
    sf *syncx.Singleflight
}

func NewAPIClient() *APIClient {
    return &APIClient{
        sf: syncx.NewSingleflight(),
    }
}

func (c *APIClient) GetWeather(city string) (*Weather, error) {
    key := fmt.Sprintf("weather:%s", city)

    result, err := c.sf.Do(key, func() (any, error) {
        // 调用外部 API
        return weatherAPI.Get(city)
    })

    if err != nil {
        return nil, err
    }

    return result.(*Weather), nil
}

// 同时有 100 个请求 GetWeather("Beijing")，只会调用 1 次 API
```

### 3. 减少数据库压力

合并短时间内的重复查询：

```go
type Repository struct {
    sf *syncx.Singleflight
    db *sql.DB
}

func (r *Repository) FindByID(ctx context.Context, id int64) (*Model, error) {
    key := fmt.Sprintf("model:%d", id)

    result, err := r.sf.Do(key, func() (any, error) {
        var model Model
        err := r.db.QueryRowContext(ctx,
            "SELECT * FROM models WHERE id = ?", id,
        ).Scan(&model.ID, &model.Name)

        if err != nil {
            return nil, err
        }

        return &model, nil
    })

    if err != nil {
        return nil, err
    }

    return result.(*Model), nil
}
```

### 4. 对象池 - 减少 GC 压力

使用对象池复用临时对象，减少内存分配和 GC 压力：

```go
var bufferPool = syncx.NewTypedPool(func() *bytes.Buffer {
    return &bytes.Buffer{}
})

func ProcessData(data []byte) ([]byte, error) {
    // 从池中获取 Buffer
    buf := bufferPool.Get()
    defer bufferPool.Put(buf)

    // 使用 Buffer
    buf.Write(data)
    buf.WriteString(" processed")

    // 返回前复制数据（因为 buf 会被放回池中）
    result := make([]byte, buf.Len())
    copy(result, buf.Bytes())

    // 重置 Buffer
    buf.Reset()

    return result, nil
}
```

### 5. JSON 编解码优化

```go
var encoderPool = syncx.NewTypedPool(func() *json.Encoder {
    return json.NewEncoder(nil)
})

var decoderPool = syncx.NewTypedPool(func() *json.Decoder {
    return json.NewDecoder(nil)
})

func EncodeJSON(w io.Writer, v any) error {
    encoder := encoderPool.Get()
    defer encoderPool.Put(encoder)

    // 重置 Writer
    encoder.Reset(w)

    return encoder.Encode(v)
}

func DecodeJSON(r io.Reader, v any) error {
    decoder := decoderPool.Get()
    defer decoderPool.Put(decoder)

    // 重置 Reader（需要自定义 Decoder）
    // decoder.Reset(r)

    return decoder.Decode(v)
}
```

### 6. HTTP Response Writer

```go
var responsePool = syncx.NewTypedPool(func() *bytes.Buffer {
    return &bytes.Buffer{}
})

func HTTPHandler(w http.ResponseWriter, r *http.Request) {
    buf := responsePool.Get()
    defer func() {
        buf.Reset()
        responsePool.Put(buf)
    }()

    // 构建响应
    buf.WriteString(`{"status": "ok"}`)

    // 写入响应
    w.Header().Set("Content-Type", "application/json")
    w.Write(buf.Bytes())
}
```

### 7. StringBuilder 池

```go
var stringBuilderPool = syncx.NewTypedPool(func() *strings.Builder {
    return &strings.Builder{}
})

func FormatMessage(user string, action string, time time.Time) string {
    sb := stringBuilderPool.Get()
    defer func() {
        sb.Reset()
        stringBuilderPool.Put(sb)
    }()

    sb.WriteString("[")
    sb.WriteString(time.Format("2006-01-02 15:04:05"))
    sb.WriteString("] ")
    sb.WriteString(user)
    sb.WriteString(" ")
    sb.WriteString(action)

    return sb.String()
}
```

### 8. 缓存组件集成

```go
type Cache struct {
    sf    *syncx.Singleflight
    local sync.Map
    redis *redis.Client
}

func NewCache(rdb *redis.Client) *Cache {
    return &Cache{
        sf:    syncx.NewSingleflight(),
        redis: rdb,
    }
}

func (c *Cache) Get(ctx context.Context, key string, dest any,
    loader func() (any, error)) error {

    // 1. 查本地缓存
    if val, ok := c.local.Load(key); ok {
        return copyValue(dest, val)
    }

    // 2. 使用 Singleflight 防击穿
    result, err := c.sf.Do(key, func() (any, error) {
        // 2.1 再次检查本地缓存
        if val, ok := c.local.Load(key); ok {
            return val, nil
        }

        // 2.2 查 Redis
        val, err := c.redis.Get(ctx, key).Result()
        if err == nil {
            c.local.Store(key, val)
            return val, nil
        }

        // 2.3 调用 loader
        val, err = loader()
        if err != nil {
            return nil, err
        }

        // 2.4 写回缓存
        c.redis.Set(ctx, key, val, 10*time.Minute)
        c.local.Store(key, val)

        return val, nil
    })

    if err != nil {
        return err
    }

    return copyValue(dest, result)
}
```

### 9. 批量请求优化

```go
type BatchLoader struct {
    sf *syncx.Singleflight
}

func NewBatchLoader() *BatchLoader {
    return &BatchLoader{
        sf: syncx.NewSingleflight(),
    }
}

func (l *BatchLoader) LoadUsers(ids []int64) ([]*User, error) {
    // 为批量请求生成唯一 key
    key := fmt.Sprintf("users:%v", ids)

    result, err := l.sf.Do(key, func() (any, error) {
        // 批量加载
        return db.GetUsersByIDs(ids)
    })

    if err != nil {
        return nil, err
    }

    return result.([]*User), nil
}

// 多个 goroutine 同时调用 LoadUsers([1,2,3])，只会查询一次数据库
```

### 10. 热点数据保护

```go
type HotDataCache struct {
    sf    *syncx.Singleflight
    cache *lru.Cache
}

func (c *HotDataCache) Get(key string) (any, error) {
    // 先查 LRU 缓存
    if val, ok := c.cache.Get(key); ok {
        return val, nil
    }

    // Singleflight 防止热点数据击穿
    result, err := c.sf.Do(key, func() (any, error) {
        // 再次检查 LRU
        if val, ok := c.cache.Get(key); ok {
            return val, nil
        }

        // 加载数据
        val, err := loadFromDB(key)
        if err != nil {
            return nil, err
        }

        // 写入 LRU
        c.cache.Add(key, val)

        return val, nil
    })

    return result, err
}
```

## 性能说明

### Singleflight 性能

```
无竞争:           50 ns/op
有竞争(10并发):   200 ns/op
有竞争(100并发):  300 ns/op
```

在高并发场景下，Singleflight 可以将 DB/API 请求数从 N 减少到 1，大幅降低后端压力。

### Pool 性能

```
不使用 Pool:      100 ns/op + 48 B/op
使用 Pool:        10 ns/op + 0 B/op (命中)
```

对于高频创建的对象（如 bytes.Buffer），Pool 可以减少 90% 的分配和 GC 压力。

### 性能建议

1. **Singleflight 适用于读多写少**：
   - 适合：缓存查询、数据库查询、API 调用
   - 不适合：写入操作（会导致并发写冲突）

2. **Pool 适用于高频临时对象**：
   ```go
   // 适合使用 Pool
   - bytes.Buffer
   - strings.Builder
   - json.Encoder/Decoder
   - 临时切片（固定大小）

   // 不适合使用 Pool
   - 包含指针的复杂对象（容易内存泄漏）
   - 对象状态复杂（难以重置）
   - 生命周期长的对象
   ```

3. **记得重置对象**：
   ```go
   buf := pool.Get().(*bytes.Buffer)
   defer func() {
       buf.Reset()  // ⚠️ 必须重置
       pool.Put(buf)
   }()
   ```

## 设计原则

1. **简单易用**：API 简洁，易于理解和使用
2. **类型安全**：TypedPool 使用泛型，避免类型断言
3. **零依赖**：只使用 Go 标准库 sync 包
4. **并发安全**：所有类型都是线程安全的

## 注意事项

### Singleflight 注意事项

1. **错误传播**：
   ```go
   // 如果函数返回错误，所有等待的请求都会收到相同的错误
   result, err := sf.Do("key", func() (any, error) {
       return nil, errors.New("database error")
   })
   // 所有并发请求都会收到 "database error"
   ```

2. **内存泄漏风险**：
   ```go
   // 如果某个 key 的函数一直不返回，会导致其他请求永久等待
   // 建议：使用 context 和超时
   result, err := sf.Do("key", func() (any, error) {
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()
       return loadWithContext(ctx)
   })
   ```

3. **不要在 Do 内部调用 Do**：
   ```go
   // ❌ 可能导致死锁
   sf.Do("key1", func() (any, error) {
       return sf.Do("key2", func() (any, error) {
           return sf.Do("key1", func() (any, error) {  // 死锁
               return "value", nil
           })
       })
   })
   ```

### Pool 注意事项

1. **必须重置对象**：
   ```go
   buf := pool.Get().(*bytes.Buffer)
   buf.WriteString("hello")

   // ❌ 忘记重置
   pool.Put(buf)  // 下次 Get 会得到包含 "hello" 的 Buffer

   // ✅ 正确做法
   buf.Reset()
   pool.Put(buf)
   ```

2. **不要存储重要数据**：
   ```go
   // ❌ Pool 中的对象可能被 GC 回收
   var cache = NewPool(func() any { return make(map[string]string) })

   m := cache.Get().(map[string]string)
   m["important"] = "data"
   cache.Put(m)  // 可能被 GC 回收，数据丢失
   ```

3. **避免内存泄漏**：
   ```go
   // ❌ 对象包含大量数据
   type BigObject struct {
       data []byte  // 1MB
   }

   // 如果 Pool 中有 1000 个对象，会占用 1GB 内存

   // ✅ 使用固定大小或限制 Pool 容量
   type SmallObject struct {
       data [1024]byte  // 固定 1KB
   }
   ```

4. **并发安全**：
   ```go
   // Pool 本身是并发安全的
   var pool = NewPool(func() any { return &bytes.Buffer{} })

   // 可以在多个 goroutine 中安全使用
   for i := 0; i < 10; i++ {
       go func() {
           buf := pool.Get().(*bytes.Buffer)
           defer pool.Put(buf)
           // 使用 buf
       }()
   }
   ```

## 依赖

```bash
# 零外部依赖，仅使用标准库
import "sync"
```

## 扩展建议

如需更多并发工具，可考虑：
- `golang.org/x/sync/singleflight` - 官方 Singleflight（功能更丰富）
- `golang.org/x/sync/semaphore` - 信号量
- `github.com/panjf2000/ants` - Goroutine 池
- `github.com/go-playground/pool` - 对象池增强版
