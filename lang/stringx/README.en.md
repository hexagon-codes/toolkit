# stringx

`stringx` provides string conversion and manipulation helpers.

```go
text := stringx.BytesToString([]byte("hello"))
data := stringx.StringToBytes("world")
items := stringx.StringToSlice([]int{1, 2, 3})
```

`BytesToString` and `StringToBytes` use safe Go copying conversions. Results do not alias mutable input storage and can be passed across functions and goroutines normally. Performance-sensitive callers should benchmark first and keep any unsafe optimization local instead of exposing aliases from a general-purpose package.
