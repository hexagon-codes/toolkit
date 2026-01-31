# Slice 切片工具

提供常用切片操作的泛型函数，简化切片处理。

## 特性

- ✅ 泛型支持（Go 1.18+）
- ✅ 去重、查找、删除
- ✅ 反转、打乱、分块
- ✅ 集合操作（并集、交集、差集）
- ✅ 聚合函数（求和、最大值、最小值）
- ✅ 高阶函数（Any、All、GroupBy）
- ✅ 零外部依赖

## 快速开始

### 基本操作

```go
import "github.com/everyday-items/toolkit/util/slice"

// 去重
nums := []int{1, 2, 2, 3, 3, 3}
unique := slice.Unique(nums)
// 输出: [1, 2, 3]

// 查找元素
exists := slice.Contains([]string{"a", "b", "c"}, "b")  // true

// 查找索引
index := slice.IndexOf([]int{1, 2, 3}, 2)  // 1
index := slice.LastIndexOf([]int{1, 2, 2, 3}, 2)  // 2

// 删除元素
nums := []int{1, 2, 3, 4, 5}
result := slice.Remove(nums, 3)  // [1, 2, 4, 5]
result := slice.RemoveAt(nums, 2)  // [1, 2, 4, 5]
result := slice.RemoveAll([]int{1, 2, 2, 3}, 2)  // [1, 3]
```

### 数组变换

```go
// 反转
nums := []int{1, 2, 3, 4, 5}
reversed := slice.Reverse(nums)  // [5, 4, 3, 2, 1]

// 打乱（简化版随机）
shuffled := slice.Shuffle(nums)  // [3, 1, 5, 2, 4]

// 分块
nums := []int{1, 2, 3, 4, 5, 6, 7}
chunks := slice.Chunk(nums, 3)
// 输出: [[1, 2, 3], [4, 5, 6], [7]]

// 扁平化
nested := [][]int{{1, 2}, {3, 4}, {5}}
flat := slice.Flatten(nested)  // [1, 2, 3, 4, 5]
```

### 集合操作

```go
a := []int{1, 2, 3, 4}
b := []int{3, 4, 5, 6}

// 并集
union := slice.Union(a, b)  // [1, 2, 3, 4, 5, 6]

// 交集
intersect := slice.Intersect(a, b)  // [3, 4]

// 差集（在 a 中但不在 b 中）
diff := slice.Difference(a, b)  // [1, 2]

// 判断相等
equal := slice.Equal([]int{1, 2, 3}, []int{1, 2, 3})  // true
```

### 聚合函数

```go
nums := []int{1, 2, 3, 4, 5}

// 求和
sum := slice.Sum(nums)  // 15

// 最大值
max := slice.Max(nums)  // 5

// 最小值
min := slice.Min(nums)  // 1

// 浮点数求和
floats := []float64{1.5, 2.5, 3.0}
sum := slice.SumFloat(floats)  // 7.0
```

### 高阶函数

```go
nums := []int{1, 2, 3, 4, 5}

// 判断是否有元素满足条件
hasEven := slice.Any(nums, func(n int) bool {
    return n%2 == 0
})  // true

// 判断是否所有元素都满足条件
allPositive := slice.All(nums, func(n int) bool {
    return n > 0
})  // true

// 分组
type User struct {
    Name string
    Age  int
}

users := []User{
    {"Alice", 30},
    {"Bob", 25},
    {"Charlie", 30},
}

grouped := slice.GroupBy(users, func(u User) int {
    return u.Age
})
// 输出: map[25:[{Bob 25}] 30:[{Alice 30} {Charlie 30}]]

// 计数
counts := slice.CountBy(users, func(u User) int {
    return u.Age
})
// 输出: map[25:1 30:2]
```

### 获取首尾元素

```go
nums := []int{1, 2, 3, 4, 5}

// 获取第一个元素
first, ok := slice.First(nums)  // 1, true

// 获取最后一个元素
last, ok := slice.Last(nums)  // 5, true

// 空切片
empty := []int{}
first, ok := slice.First(empty)  // 0, false
```

## API 文档

### 基本操作

```go
// Unique 去重（保持顺序）
Unique[T comparable](slice []T) []T

// Contains 判断切片是否包含元素
Contains[T comparable](slice []T, item T) bool

// IndexOf 查找元素索引，不存在返回 -1
IndexOf[T comparable](slice []T, item T) int

// LastIndexOf 查找元素最后一次出现的索引
LastIndexOf[T comparable](slice []T, item T) int

// Remove 移除第一个匹配的元素
Remove[T comparable](slice []T, item T) []T

// RemoveAll 移除所有匹配的元素
RemoveAll[T comparable](slice []T, item T) []T

// RemoveAt 移除指定索引的元素
RemoveAt[T any](slice []T, index int) []T
```

### 数组变换

```go
// Reverse 反转切片
Reverse[T any](slice []T) []T

// Shuffle 打乱切片顺序
Shuffle[T any](slice []T) []T

// Chunk 将切片分成多个子切片
Chunk[T any](slice []T, size int) [][]T

// Flatten 扁平化二维切片
Flatten[T any](slices [][]T) []T
```

### 集合操作

```go
// Union 并集
Union[T comparable](slice1, slice2 []T) []T

// Intersect 交集
Intersect[T comparable](slice1, slice2 []T) []T

// Difference 差集（在 slice1 中但不在 slice2 中）
Difference[T comparable](slice1, slice2 []T) []T

// Equal 判断两个切片是否相等
Equal[T comparable](slice1, slice2 []T) bool
```

### 聚合函数

```go
// Sum 求和（整数）
Sum[T int | int64 | int32](slice []T) T

// SumFloat 求和（浮点数）
SumFloat[T float32 | float64](slice []T) T

// Max 获取最大值
Max[T int | int64 | int32 | float32 | float64](slice []T) T

// Min 获取最小值
Min[T int | int64 | int32 | float32 | float64](slice []T) T
```

### 高阶函数

```go
// GroupBy 分组
GroupBy[T any, K comparable](slice []T, keyFunc func(T) K) map[K][]T

// CountBy 计数
CountBy[T any, K comparable](slice []T, keyFunc func(T) K) map[K]int

// Any 判断是否有元素满足条件
Any[T any](slice []T, predicate func(T) bool) bool

// All 判断是否所有元素都满足条件
All[T any](slice []T, predicate func(T) bool) bool
```

### 首尾元素

```go
// First 获取第一个元素
First[T any](slice []T) (T, bool)

// Last 获取最后一个元素
Last[T any](slice []T) (T, bool)
```

## 使用场景

### 1. 数据去重

```go
// 用户ID去重
userIDs := []int64{1, 2, 2, 3, 3, 3, 4}
uniqueIDs := slice.Unique(userIDs)
// 输出: [1, 2, 3, 4]

// 字符串去重
tags := []string{"go", "python", "go", "java"}
uniqueTags := slice.Unique(tags)
// 输出: ["go", "python", "java"]
```

### 2. 权限检查

```go
func HasPermission(userPerms []string, required string) bool {
    return slice.Contains(userPerms, required)
}

// 使用
userPerms := []string{"read", "write", "delete"}
if HasPermission(userPerms, "write") {
    // 允许写入
}
```

### 3. 数据分页

```go
func PaginateData[T any](data []T, page, pageSize int) []T {
    chunks := slice.Chunk(data, pageSize)

    if page < 1 || page > len(chunks) {
        return []T{}
    }

    return chunks[page-1]
}

// 使用
users := []User{...}  // 100个用户
page2Users := PaginateData(users, 2, 10)  // 第2页，每页10个
```

### 4. 数据统计

```go
type Order struct {
    ID     int
    Amount float64
    Status string
}

orders := []Order{...}

// 按状态分组
grouped := slice.GroupBy(orders, func(o Order) string {
    return o.Status
})
// grouped["completed"] = [...已完成订单...]
// grouped["pending"] = [...待处理订单...]

// 统计每个状态的订单数
counts := slice.CountBy(orders, func(o Order) string {
    return o.Status
})
// counts["completed"] = 15
// counts["pending"] = 8
```

### 5. 数据过滤和验证

```go
// 检查是否所有价格都为正数
prices := []float64{10.5, 20.0, 30.5}
allPositive := slice.All(prices, func(p float64) bool {
    return p > 0
})

// 检查是否有任何价格超过100
hasExpensive := slice.Any(prices, func(p float64) bool {
    return p > 100
})
```

### 6. 数据合并和比较

```go
// 合并两个标签列表
userTags := []string{"go", "python"}
systemTags := []string{"admin", "go"}

allTags := slice.Union(userTags, systemTags)
// 输出: ["go", "python", "admin"]

// 找出共同标签
commonTags := slice.Intersect(userTags, systemTags)
// 输出: ["go"]

// 找出用户独有的标签
userOnlyTags := slice.Difference(userTags, systemTags)
// 输出: ["python"]
```

### 7. 批量处理

```go
// 批量发送邮件
emails := []string{...}  // 1000个邮箱
batches := slice.Chunk(emails, 100)  // 每批100个

for _, batch := range batches {
    sendEmails(batch)
    time.Sleep(time.Second)  // 限流
}
```

### 8. 数据转换

```go
// 提取所有用户 ID
type User struct {
    ID   int
    Name string
}

users := []User{{1, "Alice"}, {2, "Bob"}, {3, "Charlie"}}

// 使用 GroupBy 的技巧来提取 ID
idMap := slice.GroupBy(users, func(u User) int {
    return u.ID
})
ids := make([]int, 0, len(idMap))
for id := range idMap {
    ids = append(ids, id)
}
// ids = [1, 2, 3]
```

### 9. 排行榜

```go
scores := []int{95, 87, 92, 88, 100, 76}

// 获取最高分
highest := slice.Max(scores)  // 100

// 获取最低分
lowest := slice.Min(scores)  // 76

// 计算平均分
average := float64(slice.Sum(scores)) / float64(len(scores))  // 89.67
```

### 10. 数据清洗

```go
// 移除所有空字符串
data := []string{"a", "", "b", "", "c"}
cleaned := slice.RemoveAll(data, "")
// 输出: ["a", "b", "c"]

// 移除特定值
numbers := []int{1, 2, 3, 0, 4, 0, 5}
nonZero := slice.RemoveAll(numbers, 0)
// 输出: [1, 2, 3, 4, 5]
```

## 泛型约束

### comparable 约束

需要比较相等性的函数使用 `comparable` 约束：

```go
// ✅ 支持
slice.Contains([]int{1, 2, 3}, 2)
slice.Contains([]string{"a", "b"}, "a")
slice.Unique([]float64{1.1, 2.2, 1.1})

// ❌ 不支持
type User struct { Name string }
slice.Contains([]User{{Name: "Alice"}}, User{Name: "Alice"})
// 错误：User 类型不满足 comparable 约束
```

### 数值约束

聚合函数使用数值类型约束：

```go
// ✅ 支持
slice.Sum([]int{1, 2, 3})
slice.Max([]float64{1.1, 2.2, 3.3})
slice.Min([]int32{10, 20, 30})

// ❌ 不支持
slice.Sum([]string{"a", "b"})  // 编译错误
```

## 性能

```
Unique():       O(n)
Contains():     O(n)
Remove():       O(n)
Reverse():      O(n)
Chunk():        O(n)
Union():        O(n+m)
Intersect():    O(n*m)
Sum():          O(n)
GroupBy():      O(n)
```

## 注意事项

1. **不修改原切片**：
   - 所有函数返回新切片
   - 原切片保持不变

2. **Shuffle 实现**：
   - 当前实现是简化版
   - 不使用真正的随机数
   - 不适合安全敏感场景

3. **空切片处理**：
   - `Max/Min` 返回零值
   - `First/Last` 返回 `false`

4. **内存占用**：
   - 返回新切片会分配内存
   - 大切片操作需注意性能

5. **并发安全**：
   - 函数本身无状态
   - 但切片本身不是并发安全的

## 依赖

```bash
# 零外部依赖，纯 Go 标准库
# 需要 Go 1.18+ （泛型支持）
```

## 扩展建议

如需更强大的切片操作，可考虑：
- `github.com/samber/lo` - Lodash 风格的 Go 工具库
- `golang.org/x/exp/slices` - Go 官方实验性切片包（Go 1.21+ 已进入标准库）
- `github.com/thoas/go-funk` - 函数式编程工具

## 升级建议

Go 1.21+ 可使用标准库 `slices` 包：

```go
import "slices"

// 标准库提供了更多功能
slices.Sort(nums)
slices.Reverse(nums)
slices.Contains(nums, 3)
slices.Index(nums, 3)
```
