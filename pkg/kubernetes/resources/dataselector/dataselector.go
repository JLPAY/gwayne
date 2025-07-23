package dataselector

import (
	"github.com/JLPAY/gwayne/pkg/pagequery"
	"k8s.io/klog/v2"
	"sort"
	"strings"
	"time"
)

// DataCell 接口定义了数据单元的基本操作，允许获取属性并与其他数据单元进行比较。
type DataCell interface {
	// GetProperty 获取指定属性的值，返回值必须实现 Compare 方法，方便进行排序操作。
	GetProperty(PropertyName) ComparableValue
}

// ComparableValue 接口用于表示一个可以与同类型值进行比较的数据值。
type ComparableValue interface {
	// Compare 方法用来比较当前值与另一个 ComparableValue 的大小，返回值：
	// 1 如果当前值大于其他值，0 如果相等，-1 如果当前值小于其他值
	Compare(ComparableValue) int
	// Contains 方法用于判断当前值是否包含或等于另一个 ComparableValue 的值
	Contains(ComparableValue) bool
}

// DataSelector 是数据选择器，包含了用于执行数据选择、过滤、排序的必要数据。
type DataSelector struct {
	// GenericDataList 存储所有待处理的数据单元。
	GenericDataList []DataCell
	// DataSelectQuery 存储数据选择的查询条件（例如过滤条件、排序规则等）。
	DataSelectQuery *pagequery.QueryParam
}

// Implementation of sort.Interface so that we can use built-in sort function (sort.Sort) for sorting SelectableData
// 实现 sort.Interface接口
/*
type Interface interface {
    Len() int           // 返回排序数据的长度
    Less(i, j int) bool // 比较第 i 个和第 j 个元素，返回 i 是否应排在 j 前面
    Swap(i, j int)      // 交换第 i 个和第 j 个元素
}
*/

// Len 返回数据列表的长度。
func (ds DataSelector) Len() int { return len(ds.GenericDataList) }

// Swap 交换两个数据单元在列表中的位置。
func (ds DataSelector) Swap(i, j int) {
	ds.GenericDataList[i], ds.GenericDataList[j] = ds.GenericDataList[j], ds.GenericDataList[i]
}

// Less compares 2 indices inside SelectableData and returns true if first index is larger.
func (ds DataSelector) Less(i, j int) bool {
	sort := ds.DataSelectQuery.Sortby

	if sort != "" {
		// 判断是否为升序或降序排序
		asc := true
		// strings.Index(sort, "-") 返回 "-" 在字符串中的索引位置。如果 "-" 在字符串的开头（即索引位置为 0），则意味着是降序排序
		if strings.Index(sort, "-") == 0 {
			asc = false
			sort = sort[1:] // 去掉负号，获取字段名称
		}
		a := ds.GenericDataList[i].GetProperty(PropertyName(sort))
		b := ds.GenericDataList[j].GetProperty(PropertyName(sort))

		// // 如果属性值为空，则跳过排序
		if a == nil || b == nil {
			return false
		}
		cmp := a.Compare(b)
		if cmp == 0 {
			// 如果值相同，则无需交换位置
			return false
		} else {
			// 返回是否需要交换，依据升序或降序
			return (cmp == -1 && asc) || (cmp == 1 && !asc)
		}
	}
	return false
}

// Filter 对 DataSelector 进行排序，支持链式调用。
func (ds *DataSelector) Sort() *DataSelector {
	sort.Sort(*ds)
	return ds
}

// Filter 根据查询条件对数据进行过滤，过滤后返回当前数据选择器，支持链式调用
func (ds *DataSelector) Filter() *DataSelector {
	filteredList := []DataCell{}

	// 遍历所有数据单元，应用过滤条件
	for _, c := range ds.GenericDataList {
		matches := true
		// 遍历所有查询条件，检查数据是否符合过滤要求
		for key, value := range ds.DataSelectQuery.Query {
			// 如果过滤条件中包含分隔符（例如针对嵌套字段的过滤），拆分并取第一个部分作为字段名
			if strings.Contains(key, ListFilterExprSep) {
				key = strings.Split(key, ListFilterExprSep)[0]
			}

			// 获取当前数据单元的属性值
			v := c.GetProperty(PropertyName(key))
			if v == nil {
				// 如果属性值为空，记录日志并跳过当前数据单元
				klog.Warningf("属性 %s 缺失，跳过当前过滤条件.", key)
				matches = false
				continue
			}

			// 使用 Contains 方法检查值是否符合过滤条件
			if !v.Contains(ParseToComparableValue(value)) {
				matches = false
				continue
			}
		}
		if matches {
			// 如果所有条件都匹配，则保留该数据单元
			filteredList = append(filteredList, c)
		}
	}

	// 更新过滤后的数据列表
	ds.GenericDataList = filteredList
	return ds
}

// ParseToComparableValue 将不同类型的值转换为 ComparableValue 接口类型
func ParseToComparableValue(value interface{}) ComparableValue {
	switch value.(type) {
	case string:
		return StdComparableString(value.(string))
	case int:
		return StdComparableInt(value.(int))
	case time.Time:
		return StdComparableTime(value.(time.Time))
	default:
		// 如果是无法处理的类型，记录警告并返回 nil
		//klog.Warningf("不支持的过滤值类型: %T", value.(type))
		return nil
	}
}
