package cond

// SwitchBuilder 提供类型安全的 switch 表达式构建器
type SwitchBuilder[T comparable, R any] struct {
	value   T
	result  R
	matched bool
}

// Switch 创建一个 Switch 构建器
//
// 参数:
//   - value: 要匹配的值
//
// 返回:
//   - *SwitchBuilder[T, R]: Switch 构建器
//
// 示例:
//
//	result := cond.Switch[string, string](status).
//	    Case("active", "活跃").
//	    Case("inactive", "非活跃").
//	    Default("未知")
func Switch[T comparable, R any](value T) *SwitchBuilder[T, R] {
	return &SwitchBuilder[T, R]{value: value}
}

// Case 添加一个匹配分支
//
// 参数:
//   - caseVal: 要匹配的值
//   - result: 匹配成功时返回的结果
//
// 返回:
//   - *SwitchBuilder[T, R]: Switch 构建器（支持链式调用）
//
// 示例:
//
//	Switch[int, string](code).
//	    Case(200, "OK").
//	    Case(404, "Not Found")
func (s *SwitchBuilder[T, R]) Case(caseVal T, result R) *SwitchBuilder[T, R] {
	if !s.matched && s.value == caseVal {
		s.result = result
		s.matched = true
	}
	return s
}

// CaseFunc 添加一个带函数的匹配分支（延迟求值）
//
// 参数:
//   - caseVal: 要匹配的值
//   - fn: 匹配成功时执行的函数
//
// 返回:
//   - *SwitchBuilder[T, R]: Switch 构建器（支持链式调用）
func (s *SwitchBuilder[T, R]) CaseFunc(caseVal T, fn func() R) *SwitchBuilder[T, R] {
	if !s.matched && s.value == caseVal {
		s.result = fn()
		s.matched = true
	}
	return s
}

// CaseIn 添加多个可能匹配的值
//
// 参数:
//   - result: 匹配成功时返回的结果
//   - caseVals: 可能匹配的值列表
//
// 返回:
//   - *SwitchBuilder[T, R]: Switch 构建器（支持链式调用）
//
// 示例:
//
//	Switch[string, string](ext).
//	    CaseIn("图片", ".jpg", ".png", ".gif").
//	    CaseIn("文档", ".doc", ".pdf")
func (s *SwitchBuilder[T, R]) CaseIn(result R, caseVals ...T) *SwitchBuilder[T, R] {
	if !s.matched {
		for _, v := range caseVals {
			if s.value == v {
				s.result = result
				s.matched = true
				break
			}
		}
	}
	return s
}

// Default 设置默认值（当没有匹配时使用）
//
// 参数:
//   - result: 默认返回的结果
//
// 返回:
//   - R: 最终结果（匹配的值或默认值）
//
// 示例:
//
//	result := Switch[int, string](code).
//	    Case(200, "OK").
//	    Default("Unknown")
func (s *SwitchBuilder[T, R]) Default(result R) R {
	if s.matched {
		return s.result
	}
	return result
}

// DefaultFunc 设置默认值函数（延迟求值）
//
// 参数:
//   - fn: 返回默认值的函数
//
// 返回:
//   - R: 最终结果
func (s *SwitchBuilder[T, R]) DefaultFunc(fn func() R) R {
	if s.matched {
		return s.result
	}
	return fn()
}

// Result 获取结果（不设置默认值，未匹配时返回零值）
//
// 返回:
//   - R: 匹配的结果或零值
func (s *SwitchBuilder[T, R]) Result() R {
	return s.result
}

// Matched 返回是否有匹配
//
// 返回:
//   - bool: 是否已匹配
func (s *SwitchBuilder[T, R]) Matched() bool {
	return s.matched
}

// SwitchFuncBuilder 使用函数判断来匹配（类似 switch true）。
type SwitchFuncBuilder[R any] struct {
	result  R
	matched bool
}

// SwitchTrue 创建一个基于条件的 Switch 构建器
//
// 返回:
//   - *SwitchFuncBuilder[R]: Switch 构建器
//
// 示例:
//
//	grade := cond.SwitchTrue[string]().
//	    When(score >= 90, "A").
//	    When(score >= 80, "B").
//	    When(score >= 60, "C").
//	    Default("F")
func SwitchTrue[R any]() *SwitchFuncBuilder[R] {
	return &SwitchFuncBuilder[R]{}
}

// When 添加一个条件分支
//
// 参数:
//   - condition: 条件表达式
//   - result: 条件为 true 时返回的结果
//
// 返回:
//   - *SwitchFuncBuilder[R]: Switch 构建器
func (s *SwitchFuncBuilder[R]) When(condition bool, result R) *SwitchFuncBuilder[R] {
	if !s.matched && condition {
		s.result = result
		s.matched = true
	}
	return s
}

// WhenFunc 添加一个条件分支（延迟求值）
//
// 参数:
//   - condition: 条件表达式
//   - fn: 条件为 true 时执行的函数
//
// 返回:
//   - *SwitchFuncBuilder[R]: Switch 构建器
func (s *SwitchFuncBuilder[R]) WhenFunc(condition bool, fn func() R) *SwitchFuncBuilder[R] {
	if !s.matched && condition {
		s.result = fn()
		s.matched = true
	}
	return s
}

// Default 设置默认值
//
// 参数:
//   - result: 默认结果
//
// 返回:
//   - R: 最终结果
func (s *SwitchFuncBuilder[R]) Default(result R) R {
	if s.matched {
		return s.result
	}
	return result
}

// DefaultFunc 设置默认值函数
//
// 参数:
//   - fn: 返回默认值的函数
//
// 返回:
//   - R: 最终结果
func (s *SwitchFuncBuilder[R]) DefaultFunc(fn func() R) R {
	if s.matched {
		return s.result
	}
	return fn()
}

// Result 获取结果
//
// 返回:
//   - R: 匹配的结果或零值
func (s *SwitchFuncBuilder[R]) Result() R {
	return s.result
}
