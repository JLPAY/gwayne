package dataselector

import (
	"strings"
	"time"
)

// 定义一个整型比较类型
// 实现 ComparableValue 接口
type StdComparableInt int

// Compare 实现 ComparableValue 接口，比较两个 StdComparableInt 值
func (self StdComparableInt) Compare(otherV ComparableValue) int {
	other, ok := otherV.(StdComparableInt)
	if !ok {
		return -1 // 不同类型返回 -1 或其他错误值
	}
	return intsCompare(int(self), int(other))
}

// Contains 判断两个 StdComparableInt 是否相等
func (self StdComparableInt) Contains(otherV ComparableValue) bool {
	return self.Compare(otherV) == 0
}

// 比较两个 int 值，返回 -1, 0, 或 1
func intsCompare(a, b int) int {
	if a > b {
		return 1
	} else if a == b {
		return 0
	}
	return -1
}

// StdComparableString 定义一个字符串比较类型
type StdComparableString string

// Compare 实现 ComparableValue 接口，比较两个 StdComparableString 值
func (self StdComparableString) Compare(otherV ComparableValue) int {
	other, ok := otherV.(StdComparableString)
	if !ok {
		return -1 // 不同类型返回 -1 或其他错误值
	}
	return strings.Compare(string(self), string(other))
}

// Contains 判断一个字符串是否包含另一个字符串
func (self StdComparableString) Contains(otherV ComparableValue) bool {
	other, ok := otherV.(StdComparableString)
	if !ok {
		return false
	}
	return strings.Contains(string(self), string(other))
}

// StdComparableTime 定义一个时间戳比较类型
type StdComparableTime time.Time

// Compare 实现 ComparableValue 接口，比较两个 StdComparableTime 值
func (self StdComparableTime) Compare(otherV ComparableValue) int {
	other, ok := otherV.(StdComparableTime)
	if !ok {
		return -1 // 不同类型返回 -1 或其他错误值
	}
	return ints64Compare(time.Time(self).Unix(), time.Time(other).Unix())
}

// Contains 判断两个时间是否相等
func (self StdComparableTime) Contains(otherV ComparableValue) bool {
	return self.Compare(otherV) == 0
}

// ints64Compare 比较两个 int64 值，返回 -1, 0, 或 1
func ints64Compare(a, b int64) int {
	if a > b {
		return 1
	} else if a == b {
		return 0
	}
	return -1
}
