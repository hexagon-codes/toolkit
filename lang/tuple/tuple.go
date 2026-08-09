package tuple

// Tuple2 二元组，包含两个不同类型的值
type Tuple2[A, B any] struct {
	First  A
	Second B
}

// T2 创建一个二元组
//
// 参数:
//   - a: 第一个值
//   - b: 第二个值
//
// 返回:
//   - Tuple2[A, B]: 二元组
//
// 示例:
//
//	t := tuple.T2("name", 18)
//	t := tuple.T2(1, "one")
func T2[A, B any](a A, b B) Tuple2[A, B] {
	return Tuple2[A, B]{First: a, Second: b}
}

// Unpack 解包二元组，返回两个值
//
// 返回:
//   - A: 第一个值
//   - B: 第二个值
//
// 示例:
//
//	t := tuple.T2("name", 18)
//	name, age := t.Unpack()  // "name", 18
func (t Tuple2[A, B]) Unpack() (first A, second B) {
	return t.First, t.Second
}

// Swap 交换二元组的两个值
//
// 返回:
//   - Tuple2[B, A]: 交换后的二元组
//
// 示例:
//
//	t := tuple.T2("name", 18)
//	swapped := t.Swap()  // Tuple2[int, string]{18, "name"}
func (t Tuple2[A, B]) Swap() Tuple2[B, A] {
	return Tuple2[B, A]{First: t.Second, Second: t.First}
}

// Tuple3 三元组，包含三个不同类型的值
type Tuple3[A, B, C any] struct {
	First  A
	Second B
	Third  C
}

// T3 创建一个三元组
//
// 参数:
//   - a: 第一个值
//   - b: 第二个值
//   - c: 第三个值
//
// 返回:
//   - Tuple3[A, B, C]: 三元组
//
// 示例:
//
//	t := tuple.T3("name", 18, true)
func T3[A, B, C any](a A, b B, c C) Tuple3[A, B, C] {
	return Tuple3[A, B, C]{First: a, Second: b, Third: c}
}

// Unpack 解包三元组，返回三个值
//
// 返回:
//   - A: 第一个值
//   - B: 第二个值
//   - C: 第三个值
//
// 示例:
//
//	t := tuple.T3("name", 18, true)
//	name, age, active := t.Unpack()
func (t Tuple3[A, B, C]) Unpack() (first A, second B, third C) {
	return t.First, t.Second, t.Third
}

// Tuple4 四元组，包含四个不同类型的值
type Tuple4[A, B, C, D any] struct {
	First  A
	Second B
	Third  C
	Fourth D
}

// T4 创建一个四元组
//
// 参数:
//   - a: 第一个值
//   - b: 第二个值
//   - c: 第三个值
//   - d: 第四个值
//
// 返回:
//   - Tuple4[A, B, C, D]: 四元组
//
// 示例:
//
//	t := tuple.T4("name", 18, true, 3.14)
func T4[A, B, C, D any](a A, b B, c C, d D) Tuple4[A, B, C, D] {
	return Tuple4[A, B, C, D]{First: a, Second: b, Third: c, Fourth: d}
}

// Unpack 解包四元组，返回四个值
//
// 返回:
//   - A: 第一个值
//   - B: 第二个值
//   - C: 第三个值
//   - D: 第四个值
//
// 示例:
//
//	t := tuple.T4("name", 18, true, 3.14)
//	a, b, c, d := t.Unpack()
func (t Tuple4[A, B, C, D]) Unpack() (first A, second B, third C, fourth D) {
	return t.First, t.Second, t.Third, t.Fourth
}

// FromPair 从键值对创建二元组
//
// 参数:
//   - key: 键
//   - value: 值
//
// 返回:
//   - Tuple2[K, V]: 二元组
//
// 示例:
//
//	t := tuple.FromPair("name", "Alice")
func FromPair[K, V any](key K, value V) Tuple2[K, V] {
	return Tuple2[K, V]{First: key, Second: value}
}
