package tuple

// Zip2 将两个切片配对成元组切片
// 结果长度为两个切片中较短的那个
//
// 参数:
//   - s1: 第一个切片
//   - s2: 第二个切片
//
// 返回:
//   - []Tuple2[A, B]: 元组切片
//
// 示例:
//
//	names := []string{"Alice", "Bob", "Charlie"}
//	ages := []int{20, 25}
//	pairs := tuple.Zip2(names, ages)
//	// []Tuple2[string, int]{{Alice, 20}, {Bob, 25}}
func Zip2[A, B any](s1 []A, s2 []B) []Tuple2[A, B] {
	length := min(len(s1), len(s2))
	if length == 0 {
		return nil
	}
	result := make([]Tuple2[A, B], length)
	for i := range length {
		result[i] = Tuple2[A, B]{First: s1[i], Second: s2[i]}
	}
	return result
}

// Zip3 将三个切片配对成元组切片
// 结果长度为三个切片中最短的那个
//
// 参数:
//   - s1: 第一个切片
//   - s2: 第二个切片
//   - s3: 第三个切片
//
// 返回:
//   - []Tuple3[A, B, C]: 元组切片
//
// 示例:
//
//	names := []string{"Alice", "Bob"}
//	ages := []int{20, 25}
//	active := []bool{true, false}
//	triples := tuple.Zip3(names, ages, active)
func Zip3[A, B, C any](s1 []A, s2 []B, s3 []C) []Tuple3[A, B, C] {
	length := min(len(s1), len(s2), len(s3))
	if length == 0 {
		return nil
	}
	result := make([]Tuple3[A, B, C], length)
	for i := range length {
		result[i] = Tuple3[A, B, C]{First: s1[i], Second: s2[i], Third: s3[i]}
	}
	return result
}

// Zip4 将四个切片配对成元组切片
// 结果长度为四个切片中最短的那个
//
// 参数:
//   - s1: 第一个切片
//   - s2: 第二个切片
//   - s3: 第三个切片
//   - s4: 第四个切片
//
// 返回:
//   - []Tuple4[A, B, C, D]: 元组切片
func Zip4[A, B, C, D any](s1 []A, s2 []B, s3 []C, s4 []D) []Tuple4[A, B, C, D] {
	length := min(len(s1), len(s2), len(s3), len(s4))
	if length == 0 {
		return nil
	}
	result := make([]Tuple4[A, B, C, D], length)
	for i := range length {
		result[i] = Tuple4[A, B, C, D]{First: s1[i], Second: s2[i], Third: s3[i], Fourth: s4[i]}
	}
	return result
}

// Unzip2 将元组切片拆分成两个切片
//
// 参数:
//   - tuples: 元组切片
//
// 返回:
//   - []A: 第一个切片（所有 First 值）
//   - []B: 第二个切片（所有 Second 值）
//
// 示例:
//
//	pairs := []Tuple2[string, int]{{Alice, 20}, {Bob, 25}}
//	names, ages := tuple.Unzip2(pairs)
//	// names: []string{"Alice", "Bob"}
//	// ages: []int{20, 25}
func Unzip2[A, B any](tuples []Tuple2[A, B]) (first []A, second []B) {
	if len(tuples) == 0 {
		return nil, nil
	}
	s1 := make([]A, len(tuples))
	s2 := make([]B, len(tuples))
	for i, t := range tuples {
		s1[i] = t.First
		s2[i] = t.Second
	}
	return s1, s2
}

// Unzip3 将元组切片拆分成三个切片
//
// 参数:
//   - tuples: 元组切片
//
// 返回:
//   - []A: 第一个切片
//   - []B: 第二个切片
//   - []C: 第三个切片
func Unzip3[A, B, C any](tuples []Tuple3[A, B, C]) (first []A, second []B, third []C) {
	if len(tuples) == 0 {
		return nil, nil, nil
	}
	s1 := make([]A, len(tuples))
	s2 := make([]B, len(tuples))
	s3 := make([]C, len(tuples))
	for i, t := range tuples {
		s1[i] = t.First
		s2[i] = t.Second
		s3[i] = t.Third
	}
	return s1, s2, s3
}

// Unzip4 将元组切片拆分成四个切片
//
// 参数:
//   - tuples: 元组切片
//
// 返回:
//   - []A: 第一个切片
//   - []B: 第二个切片
//   - []C: 第三个切片
//   - []D: 第四个切片
func Unzip4[A, B, C, D any](tuples []Tuple4[A, B, C, D]) (first []A, second []B, third []C, fourth []D) {
	if len(tuples) == 0 {
		return nil, nil, nil, nil
	}
	s1 := make([]A, len(tuples))
	s2 := make([]B, len(tuples))
	s3 := make([]C, len(tuples))
	s4 := make([]D, len(tuples))
	for i, t := range tuples {
		s1[i] = t.First
		s2[i] = t.Second
		s3[i] = t.Third
		s4[i] = t.Fourth
	}
	return s1, s2, s3, s4
}

// ZipWithIndex 将切片元素与其索引配对
//
// 参数:
//   - slice: 输入切片
//
// 返回:
//   - []Tuple2[int, T]: 索引-元素对的切片
//
// 示例:
//
//	names := []string{"Alice", "Bob"}
//	indexed := tuple.ZipWithIndex(names)
//	// []Tuple2[int, string]{{0, Alice}, {1, Bob}}
func ZipWithIndex[T any](slice []T) []Tuple2[int, T] {
	if len(slice) == 0 {
		return nil
	}
	result := make([]Tuple2[int, T], len(slice))
	for i, v := range slice {
		result[i] = Tuple2[int, T]{First: i, Second: v}
	}
	return result
}
