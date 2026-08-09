# stringx

`stringx` 提供字符串转换与常用处理函数。

```go
text := stringx.BytesToString([]byte("hello"))
data := stringx.StringToBytes("world")
items := stringx.StringToSlice([]int{1, 2, 3})
```

`BytesToString` 和 `StringToBytes` 使用 Go 的安全复制转换：结果不与可变输入共享底层内存，可跨函数和 goroutine 正常传递。性能敏感代码应先用 benchmark 证明复制是瓶颈，再在调用方局部优化，不在通用工具包暴露 `unsafe` 别名。
