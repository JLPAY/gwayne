package dataselector

import "github.com/JLPAY/gwayne/pkg/pagequery"

func DataSelectPage(dataList []DataCell, q *pagequery.QueryParam) *pagequery.Page {
	// 创建一个 DataSelector，包含了原始数据和查询参数
	SelectableData := DataSelector{
		GenericDataList: dataList,
		DataSelectQuery: q,
	}

	// Pipeline: Filter -> Sort -> Paginate
	// 先过滤，再排序
	filtered := SelectableData.Filter().Sort()
	// 获取过滤后的数据总数
	filteredTotal := len(filtered.GenericDataList)

	// 计算分页的起始和结束位置
	start, end := q.Offset(), q.Offset()+q.Limit()

	// 确保分页不越界
	if start >= int64(filteredTotal) {
		start = int64(filteredTotal)
	}
	if end > int64(filteredTotal) {
		end = int64(filteredTotal)
	}

	// 根据分页区间切片数据
	pagedList := filtered.GenericDataList[start:end]

	// 返回分页结果
	return q.NewPage(int64(filteredTotal), pagedList)
}
