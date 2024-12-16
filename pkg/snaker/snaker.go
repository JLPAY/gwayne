// Package snaker provides methods to convert CamelCase names to snake_case and back.
package snaker

import (
	"strings"
	"unicode"
)

// CamelToSnake converts a given string to snake case
// 将输入的 CamelCase 字符串转换为 snake_case 格式
func CamelToSnake(s string) string {
	var result string
	var words []string
	var lastPos int
	rs := []rune(s) // 将输入字符串转换为 rune 切片，以便处理 Unicode 字符

	for i := 0; i < len(rs); i++ {
		// 检查当前字符是否为大写字母，并且前面已有字符
		if i > 0 && unicode.IsUpper(rs[i]) {
			// 检查是否以常见首字母缩写（如 API、HTTP 等）开头
			if initialism := startsWithInitialism(s[lastPos:]); initialism != "" {
				// 如果是常见缩写，将其作为一个单词加入结果中
				words = append(words, initialism)

				// 跳过缩写的字符，更新上次处理的位置
				i += len(initialism) - 1
				lastPos = i
				continue
			}

			// 否则，将当前大写字符前的子字符串作为一个单词加入结果中
			words = append(words, s[lastPos:i])
			lastPos = i
		}
	}

	// 将最后一个单词（大写字母后面的部分）加入结果中
	if s[lastPos:] != "" {
		words = append(words, s[lastPos:])
	}

	// 将所有单词转换为小写并以 "_" 拼接起来
	for k, word := range words {
		if k > 0 {
			result += "_"
		}

		result += strings.ToLower(word)
	}

	return result
}

// 将 snake_case 格式的字符串转换为 CamelCase 格式
func snakeToCamel(s string, upperCase bool) string {
	var result string

	// 通过 "_" 分割 snake_case 字符串为多个单词
	words := strings.Split(s, "_")

	for i, word := range words {
		// 如果需要大写或当前是第一个单词，进行首字母大写
		if upperCase || i > 0 {
			// 如果是常见的缩写，直接将其转换为大写形式
			if upper := strings.ToUpper(word); commonInitialisms[upper] {
				result += upper
				continue
			}
		}

		// 否则，正常的大小写转换：首字母大写，其余小写
		if (upperCase || i > 0) && len(word) > 0 {
			w := []rune(word)
			w[0] = unicode.ToUpper(w[0])
			result += string(w)
		} else {
			result += word
		}
	}

	return result
}

// SnakeToCamel returns a string converted from snake case to uppercase
// 将 snake_case 格式的字符串转换为首字母大写的 CamelCase 格式
func SnakeToCamel(s string) string {
	return snakeToCamel(s, true)
}

// SnakeToCamelLower returns a string converted from snake case to lowercase
// 将 snake_case 格式的字符串转换为首字母小写的 camelCase 格式
func SnakeToCamelLower(s string) string {
	return snakeToCamel(s, false)
}

// startsWithInitialism returns the initialism if the given string begins with it
// 检查给定的字符串是否以常见的首字母缩写开头
func startsWithInitialism(s string) string {
	var initialism string
	// the longest initialism is 5 char, the shortest 2
	for i := 1; i <= 5; i++ {
		if len(s) > i-1 && commonInitialisms[s[:i]] {
			initialism = s[:i]
		}
	}
	return initialism
}

// commonInitialisms, taken from
// https://github.com/golang/lint/blob/206c0f020eba0f7fbcfbc467a5eb808037df2ed6/lint.go#L731
// 存储常见的首字母缩写列表，包含一些常见的缩写如 API、HTML、DNS 等
var commonInitialisms = map[string]bool{
	"ACL":   true,
	"API":   true,
	"ASCII": true,
	"CPU":   true,
	"CSS":   true,
	"DNS":   true,
	"EOF":   true,
	"GUID":  true,
	"HTML":  true,
	"HTTP":  true,
	"HTTPS": true,
	"ID":    true,
	"IP":    true,
	"JSON":  true,
	"LHS":   true,
	"OS":    true,
	"QPS":   true,
	"RAM":   true,
	"RHS":   true,
	"RPC":   true,
	"SLA":   true,
	"SMTP":  true,
	"SQL":   true,
	"SSH":   true,
	"TCP":   true,
	"TLS":   true,
	"TTL":   true,
	"UDP":   true,
	"UI":    true,
	"UID":   true,
	"UUID":  true,
	"URI":   true,
	"URL":   true,
	"UTF8":  true,
	"VM":    true,
	"XML":   true,
	"XMPP":  true,
	"XSRF":  true,
	"XSS":   true,
}
