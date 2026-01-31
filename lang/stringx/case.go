package stringx

import (
	"strings"
	"unicode"
)

// CamelCase 转换为小驼峰格式
// "hello_world" → "helloWorld"
// "Hello World" → "helloWorld"
// "hello-world" → "helloWorld"
func CamelCase(s string) string {
	if s == "" {
		return ""
	}

	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(strings.ToLower(words[0]))

	for i := 1; i < len(words); i++ {
		if len(words[i]) > 0 {
			builder.WriteString(strings.ToUpper(words[i][:1]))
			builder.WriteString(strings.ToLower(words[i][1:]))
		}
	}

	return builder.String()
}

// PascalCase 转换为大驼峰格式
// "hello_world" → "HelloWorld"
// "hello world" → "HelloWorld"
// "hello-world" → "HelloWorld"
func PascalCase(s string) string {
	if s == "" {
		return ""
	}

	words := splitWords(s)
	var builder strings.Builder

	for _, word := range words {
		if len(word) > 0 {
			builder.WriteString(strings.ToUpper(word[:1]))
			builder.WriteString(strings.ToLower(word[1:]))
		}
	}

	return builder.String()
}

// SnakeCase 转换为蛇形格式
// "HelloWorld" → "hello_world"
// "helloWorld" → "hello_world"
// "hello-world" → "hello_world"
func SnakeCase(s string) string {
	return joinWords(splitWords(s), "_")
}

// KebabCase 转换为短横线格式
// "HelloWorld" → "hello-world"
// "helloWorld" → "hello-world"
// "hello_world" → "hello-world"
func KebabCase(s string) string {
	return joinWords(splitWords(s), "-")
}

// ScreamingSnakeCase 转换为全大写蛇形格式
// "HelloWorld" → "HELLO_WORLD"
// "helloWorld" → "HELLO_WORLD"
func ScreamingSnakeCase(s string) string {
	return strings.ToUpper(SnakeCase(s))
}

// TitleCase 转换为标题格式
// "hello_world" → "Hello World"
// "helloWorld" → "Hello World"
func TitleCase(s string) string {
	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, word := range words {
		if i > 0 {
			builder.WriteString(" ")
		}
		if len(word) > 0 {
			builder.WriteString(strings.ToUpper(word[:1]))
			builder.WriteString(strings.ToLower(word[1:]))
		}
	}

	return builder.String()
}

// splitWords 将字符串分割为单词列表
// 正确处理 UTF-8 多字节字符
func splitWords(s string) []string {
	var words []string
	var currentWord strings.Builder
	var prevRune rune
	var hasPrev bool

	for _, r := range s {
		if r == '_' || r == '-' || r == ' ' || r == '\t' {
			// 分隔符
			if currentWord.Len() > 0 {
				words = append(words, currentWord.String())
				currentWord.Reset()
			}
			hasPrev = false
		} else if unicode.IsUpper(r) {
			// 大写字母：可能是新单词的开始
			if currentWord.Len() > 0 && hasPrev {
				// 检查是否是连续大写（如 "XMLParser"）
				if !unicode.IsUpper(prevRune) {
					words = append(words, currentWord.String())
					currentWord.Reset()
				}
			}
			currentWord.WriteRune(r)
			prevRune = r
			hasPrev = true
		} else {
			// 检查前一个是大写且当前是小写（如 "XMLParser" 中的 "L" 和 "P"）
			if hasPrev && currentWord.Len() > 1 {
				str := currentWord.String()
				runes := []rune(str)
				lastRune := runes[len(runes)-1]
				if unicode.IsUpper(lastRune) && unicode.IsLower(r) {
					// 把最后一个大写字母移到新单词
					words = append(words, string(runes[:len(runes)-1]))
					currentWord.Reset()
					currentWord.WriteRune(lastRune)
				}
			}
			currentWord.WriteRune(r)
			prevRune = r
			hasPrev = true
		}
	}

	if currentWord.Len() > 0 {
		words = append(words, currentWord.String())
	}

	return words
}

// joinWords 用分隔符连接单词（小写）
func joinWords(words []string, sep string) string {
	if len(words) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, word := range words {
		if i > 0 {
			builder.WriteString(sep)
		}
		builder.WriteString(strings.ToLower(word))
	}

	return builder.String()
}
