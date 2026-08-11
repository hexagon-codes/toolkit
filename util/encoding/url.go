package encoding

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// ErrUnsafeURLPath 表示待连接路径包含目录跳转片段。
var ErrUnsafeURLPath = errors.New("encoding: unsafe URL path")

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

// BuildQuery 从 map 构建查询字符串（按 key 排序，保证输出确定性）
func BuildQuery(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	// 先对 key 排序，保证输出顺序确定
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(params))
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
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

// JoinURL 解析并连接 URL 路径，同时保留查询参数和片段。
func JoinURL(base string, paths ...string) (string, error) {
	if _, err := url.Parse(base); err != nil {
		return "", fmt.Errorf("join URL: %w", err)
	}
	if len(paths) == 0 {
		return base, nil
	}
	cleaned := make([]string, 0, len(paths))
	for _, element := range paths {
		element = strings.Trim(element, "/")
		if element != "" {
			for _, encodedSegment := range strings.Split(element, "/") {
				decodedSegment, err := url.PathUnescape(encodedSegment)
				if err != nil {
					return "", fmt.Errorf("join URL: %w", err)
				}
				for _, segment := range strings.Split(decodedSegment, "/") {
					if segment == "." || segment == ".." {
						return "", fmt.Errorf("%w: dot segment", ErrUnsafeURLPath)
					}
				}
			}
			cleaned = append(cleaned, element)
		}
	}
	if len(cleaned) == 0 {
		return base, nil
	}
	joined, err := url.JoinPath(base, cleaned...)
	if err != nil {
		return "", fmt.Errorf("join URL: %w", err)
	}
	return joined, nil
}
