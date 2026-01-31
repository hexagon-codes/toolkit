package encoding

import (
	"net/url"
	"strings"
)

// URLEncode URL 编码
func URLEncode(s string) string {
	return url.QueryEscape(s)
}

// URLDecode URL 解码
func URLDecode(s string) (string, error) {
	return url.QueryUnescape(s)
}

// URLPathEncode URL 路径编码
func URLPathEncode(s string) string {
	return url.PathEscape(s)
}

// URLPathDecode URL 路径解码
func URLPathDecode(s string) (string, error) {
	return url.PathUnescape(s)
}

// BuildQuery 从 map 构建查询字符串
func BuildQuery(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	var parts []string
	for k, v := range params {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
	}

	return strings.Join(parts, "&")
}

// ParseQuery 解析查询字符串为 map
func ParseQuery(query string) (map[string]string, error) {
	values, err := url.ParseQuery(query)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(values))
	for k, v := range values {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}

	return result, nil
}

// ParseQueryValues 解析查询字符串为 map（支持多值）
func ParseQueryValues(query string) (map[string][]string, error) {
	values, err := url.ParseQuery(query)
	if err != nil {
		return nil, err
	}
	return values, nil
}

// JoinURL 安全地连接 URL 路径
func JoinURL(base string, paths ...string) string {
	if len(paths) == 0 {
		return base
	}

	// 移除 base 末尾的斜杠
	base = strings.TrimRight(base, "/")

	for _, p := range paths {
		// 移除路径两端的斜杠
		p = strings.Trim(p, "/")
		if p != "" {
			base = base + "/" + p
		}
	}

	return base
}
