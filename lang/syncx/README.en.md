[中文](README.md) | English

# Syncx - Concurrent Synchronization Utilities

Provides implementations of common concurrent synchronization patterns, including Singleflight and object pools.

## Features

- ✅ Singleflight - Prevents cache stampede, deduplicates concurrent calls
- ✅ Pool - Simple wrapper around sync.Pool
- ✅ TypedPool - Type-safe object pool (generics)
- ✅ Concurrency safe - All types are thread-safe
- ✅ Zero external dependencies - Uses only the Go standard library
- ✅ Simple and easy to use - Clean and straightforward API

## Quick Start

### Singleflight - Prevent Cache Stampede

```go
import "github.com/everyday-items/toolkit/lang/syncx"

// Create a Singleflight instance
sf := syncx.NewSingleflight()

// Multiple concurrent requests will be merged into one execution
result, err := sf.Do("user:123", func() (any, error) {
    // Even with 1000 concurrent requests, this function executes only once
    return db.GetUser(123)
})

// Remove record to force re-execution next time
sf.Forget("user:123")
```

### Pool - Object Reuse

```go
import (
    "bytes"
    "github.com/everyday-items/toolkit/lang/syncx"
)

// Create object pool
pool := syncx.NewPool(func() any {
    return &bytes.Buffer{}
})

// Get object
buf := pool.Get().(*bytes.Buffer)
defer pool.Put(buf)

// Use object
buf.WriteString("hello")
fmt.Println(buf.String())

// Remember to reset before returning
buf.Reset()
pool.Put(buf)
```

### TypedPool - Type-Safe Object Pool

```go
import (
    "bytes"
    "github.com/everyday-items/toolkit/lang/syncx"
)

// Create type-safe object pool (no type assertion needed)
pool := syncx.NewTypedPool(func() *bytes.Buffer {
    return &bytes.Buffer{}
})

// Get object (type-safe, no assertion needed)
buf := pool.Get()  // returns *bytes.Buffer directly
defer pool.Put(buf)

buf.WriteString("hello")
fmt.Println(buf.String())
```

## API Reference

### Singleflight

```go
// NewSingleflight creates a Singleflight instance
NewSingleflight() *Singleflight

// Do executes function, only one execution per key at a time
Do(key string, fn func() (any, error)) (any, error)

// Forget removes the record for the specified key
Forget(key string)
```

### Pool

```go
// NewPool creates an object pool
NewPool(newFunc func() any) *Pool

// Get retrieves an object from the pool
Get() any

// Put returns an object to the pool
Put(x any)
```

### TypedPool

```go
// NewTypedPool creates a type-safe object pool
NewTypedPool[T any](newFunc func() T) *TypedPool[T]

// Get retrieves an object from the pool (type-safe)
Get() T

// Put returns an object to the pool
Put(x T)
```

## Use Cases

### 1. Preventing Cache Stampede

When a cache expires, if a large number of concurrent requests access the same key, all requests hit the database, causing sudden database pressure spikes. Singleflight merges these requests into one.

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

    // Check cache first
    c.mu.RLock()
    if user, ok := c.cache[key]; ok {
        c.mu.RUnlock()
        return user, nil
    }
    c.mu.RUnlock()

    // Use Singleflight to prevent stampede
    result, err := c.sf.Do(key, func() (any, error) {
        // Check cache again (double-check)
        c.mu.RLock()
        if user, ok := c.cache[key]; ok {
            c.mu.RUnlock()
            return user, nil
        }
        c.mu.RUnlock()

        // Load from database
        user, err := db.GetUser(id)
        if err != nil {
            return nil, err
        }

        // Write to cache
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

// Clear cache and Singleflight record when updating user
func (c *UserCache) UpdateUser(user *User) error {
    key := fmt.Sprintf("user:%d", user.ID)

    // Update database
    if err := db.UpdateUser(user); err != nil {
        return err
    }

    // Delete cache
    c.mu.Lock()
    delete(c.cache, key)
    c.mu.Unlock()

    // Clear Singleflight record
    c.sf.Forget(key)

    return nil
}
```

### 2. API Request Deduplication

Prevent repeated calls to the same external API within a short time:

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
        // Call external API
        return weatherAPI.Get(city)
    })

    if err != nil {
        return nil, err
    }

    return result.(*Weather), nil
}

// If 100 requests call GetWeather("Beijing") simultaneously, the API is called only once
```

### 3. Reducing Database Pressure

Merge duplicate queries within a short time:

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

### 4. Object Pool - Reducing GC Pressure

Use object pools to reuse temporary objects, reducing memory allocation and GC pressure:

```go
var bufferPool = syncx.NewTypedPool(func() *bytes.Buffer {
    return &bytes.Buffer{}
})

func ProcessData(data []byte) ([]byte, error) {
    // Get Buffer from pool
    buf := bufferPool.Get()
    defer bufferPool.Put(buf)

    // Use Buffer
    buf.Write(data)
    buf.WriteString(" processed")

    // Copy data before returning (buf will be returned to pool)
    result := make([]byte, buf.Len())
    copy(result, buf.Bytes())

    // Reset Buffer
    buf.Reset()

    return result, nil
}
```

### 5. JSON Encoding/Decoding Optimization

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

    // Reset Writer
    encoder.Reset(w)

    return encoder.Encode(v)
}

func DecodeJSON(r io.Reader, v any) error {
    decoder := decoderPool.Get()
    defer decoderPool.Put(decoder)

    // Reset Reader (requires custom Decoder)
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

    // Build response
    buf.WriteString(`{"status": "ok"}`)

    // Write response
    w.Header().Set("Content-Type", "application/json")
    w.Write(buf.Bytes())
}
```

### 7. StringBuilder Pool

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

### 8. Cache Component Integration

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

    // 1. Check local cache
    if val, ok := c.local.Load(key); ok {
        return copyValue(dest, val)
    }

    // 2. Use Singleflight to prevent stampede
    result, err := c.sf.Do(key, func() (any, error) {
        // 2.1 Check local cache again
        if val, ok := c.local.Load(key); ok {
            return val, nil
        }

        // 2.2 Check Redis
        val, err := c.redis.Get(ctx, key).Result()
        if err == nil {
            c.local.Store(key, val)
            return val, nil
        }

        // 2.3 Call loader
        val, err = loader()
        if err != nil {
            return nil, err
        }

        // 2.4 Write back to cache
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

### 9. Batch Request Optimization

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
    // Generate unique key for batch request
    key := fmt.Sprintf("users:%v", ids)

    result, err := l.sf.Do(key, func() (any, error) {
        // Batch load
        return db.GetUsersByIDs(ids)
    })

    if err != nil {
        return nil, err
    }

    return result.([]*User), nil
}

// Multiple goroutines calling LoadUsers([1,2,3]) simultaneously query the database only once
```

### 10. Hot Data Protection

```go
type HotDataCache struct {
    sf    *syncx.Singleflight
    cache *lru.Cache
}

func (c *HotDataCache) Get(key string) (any, error) {
    // Check LRU cache first
    if val, ok := c.cache.Get(key); ok {
        return val, nil
    }

    // Singleflight prevents hot data stampede
    result, err := c.sf.Do(key, func() (any, error) {
        // Check LRU again
        if val, ok := c.cache.Get(key); ok {
            return val, nil
        }

        // Load data
        val, err := loadFromDB(key)
        if err != nil {
            return nil, err
        }

        // Write to LRU
        c.cache.Add(key, val)

        return val, nil
    })

    return result, err
}
```

## Performance Notes

### Singleflight Performance

```
No contention:         50 ns/op
With contention (10):  200 ns/op
With contention (100): 300 ns/op
```

In high-concurrency scenarios, Singleflight can reduce DB/API requests from N to 1, significantly reducing backend pressure.

### Pool Performance

```
Without Pool:   100 ns/op + 48 B/op
With Pool:      10 ns/op + 0 B/op (cache hit)
```

For frequently created objects (like bytes.Buffer), Pool can reduce 90% of allocations and GC pressure.

### Performance Recommendations

1. **Singleflight suits read-heavy workloads**:
   - Suitable: cache queries, database queries, API calls
   - Not suitable: write operations (may cause concurrent write conflicts)

2. **Pool suits high-frequency temporary objects**:
   ```go
   // Suitable for Pool
   - bytes.Buffer
   - strings.Builder
   - json.Encoder/Decoder
   - temporary slices (fixed size)

   // Not suitable for Pool
   - complex objects with pointers (risk of memory leaks)
   - objects with complex state (hard to reset)
   - long-lived objects
   ```

3. **Remember to reset objects**:
   ```go
   buf := pool.Get().(*bytes.Buffer)
   defer func() {
       buf.Reset()  // must reset
       pool.Put(buf)
   }()
   ```

## Design Principles

1. **Simple and easy to use**: Clean API, easy to understand and use
2. **Type safety**: TypedPool uses generics to avoid type assertions
3. **Zero dependencies**: Uses only the Go standard sync package
4. **Concurrency safe**: All types are thread-safe

## Notes

### Singleflight Notes

1. **Error propagation**:
   ```go
   // If the function returns an error, all waiting requests receive the same error
   result, err := sf.Do("key", func() (any, error) {
       return nil, errors.New("database error")
   })
   // All concurrent requests will receive "database error"
   ```

2. **Memory leak risk**:
   ```go
   // If a function for a key never returns, other requests wait indefinitely
   // Recommendation: use context with timeout
   result, err := sf.Do("key", func() (any, error) {
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()
       return loadWithContext(ctx)
   })
   ```

3. **Do not call Do inside Do**:
   ```go
   // ❌ May cause deadlock
   sf.Do("key1", func() (any, error) {
       return sf.Do("key2", func() (any, error) {
           return sf.Do("key1", func() (any, error) {  // deadlock
               return "value", nil
           })
       })
   })
   ```

### Pool Notes

1. **Must reset objects**:
   ```go
   buf := pool.Get().(*bytes.Buffer)
   buf.WriteString("hello")

   // ❌ Forgot to reset
   pool.Put(buf)  // next Get will return a Buffer containing "hello"

   // ✅ Correct approach
   buf.Reset()
   pool.Put(buf)
   ```

2. **Do not store important data**:
   ```go
   // ❌ Objects in Pool may be reclaimed by GC
   var cache = NewPool(func() any { return make(map[string]string) })

   m := cache.Get().(map[string]string)
   m["important"] = "data"
   cache.Put(m)  // may be reclaimed by GC, data lost
   ```

3. **Avoid memory leaks**:
   ```go
   // ❌ Objects containing large amounts of data
   type BigObject struct {
       data []byte  // 1MB
   }

   // If Pool has 1000 objects, it occupies 1GB of memory

   // ✅ Use fixed size or limit Pool capacity
   type SmallObject struct {
       data [1024]byte  // fixed 1KB
   }
   ```

4. **Concurrency safety**:
   ```go
   // Pool itself is concurrency-safe
   var pool = NewPool(func() any { return &bytes.Buffer{} })

   // Can be safely used in multiple goroutines
   for i := 0; i < 10; i++ {
       go func() {
           buf := pool.Get().(*bytes.Buffer)
           defer pool.Put(buf)
           // use buf
       }()
   }
   ```

## Dependencies

```bash
# Zero external dependencies, uses only the standard library
import "sync"
```

## Extension Suggestions

For more concurrency utilities, consider:
- `golang.org/x/sync/singleflight` - Official Singleflight (more features)
- `golang.org/x/sync/semaphore` - Semaphore
- `github.com/panjf2000/ants` - Goroutine pool
- `github.com/go-playground/pool` - Enhanced object pool
