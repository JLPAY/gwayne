package pagequery

type QueryParam struct {
	PageNo   int64                  `json:"pageNo"`   // 当前页码
	PageSize int64                  `json:"pageSize"` // 每页条数
	Query    map[string]interface{} `json:"query"`    // 查询条件
	Sortby   string                 `json:"sortby"`   // 排序字段
	Groupby  []string               `json:"groupby"`  // 分组字段
	Relate   string                 `json:"relate"`   // 关联条件
	// only for kubernetes resource
	LabelSelector string `json:"-"` // Kubernetes 使用的字段，避免序列化
}

func (q *QueryParam) Offset() int64 {
	offset := (q.PageNo - 1) * q.PageSize
	if offset < 0 {
		offset = 0
	}
	return offset
}

func (q *QueryParam) Limit() int64 {
	return q.PageSize
}

func (q *QueryParam) NewPage(count int64, list interface{}) *Page {
	// 计算总页数
	totalPage := count / q.PageSize
	if count%q.PageSize > 0 {
		totalPage++
	}
	return &Page{
		PageNo:     q.PageNo,
		PageSize:   q.PageSize,
		TotalPage:  totalPage,
		TotalCount: count,
		List:       list,
	}
}
