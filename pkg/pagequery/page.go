package pagequery

type Page struct {
	PageNo     int64       `json:"pageNo"`     // 当前页码
	PageSize   int64       `json:"pageSize"`   // 每页条数
	TotalPage  int64       `json:"totalPage"`  // 总页数
	TotalCount int64       `json:"totalCount"` // 总记录数
	List       interface{} `json:"list"`       // 数据列表
}
