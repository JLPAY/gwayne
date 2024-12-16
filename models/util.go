package models

import (
	"fmt"
	"github.com/JLPAY/gwayne/pkg/pagequery"
	"gorm.io/gorm"
	"strings"
)

// 获取数总数
func GetTotal(queryTable interface{}, q *pagequery.QueryParam) (int64, error) {
	// 构建基础查询
	qs := DB.Model(queryTable)

	// 应用过滤条件
	qs = BuildFilter(qs, q.Query)

	// 分组
	if len(q.Groupby) != 0 {
		// 将切片拼接成逗号分隔的字符串
		groupByStr := strings.Join(q.Groupby, ",")
		qs = qs.Group(groupByStr)
	}

	// 统计总数
	var count int64
	if err := qs.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func GetAll(queryTable interface{}, list interface{}, q *pagequery.QueryParam) error {
	// 构建基础查询
	qs := DB.Model(queryTable)

	// 应用过滤条件
	qs = BuildFilter(qs, q.Query)

	// 关联查询
	if q.Relate != "" {
		if q.Relate == "all" {
			qs = qs.Preload("related_model")
		} else {
			qs = qs.Preload(q.Relate)
		}
	}

	// 分组
	if len(q.Groupby) != 0 {
		// 将切片拼接成逗号分隔的字符串
		groupByStr := strings.Join(q.Groupby, ",")
		qs = qs.Group(groupByStr)
	}

	// 排序
	if q.Sortby != "" {
		qs = qs.Order(q.Sortby)
	}

	// 应用分页
	qs = qs.Offset(int(q.Offset())).Limit(int(q.Limit()))

	// 查询结果
	if err := qs.Find(list).Error; err != nil {
		return err
	}
	return nil
}

// BuildFilter 构建过滤条件
func BuildFilter(db *gorm.DB, query map[string]interface{}) *gorm.DB {
	for key, value := range query {
		// 这里简单的按键值对添加过滤条件，复杂的过滤条件可以根据需要进一步扩展
		if value != nil {
			db = db.Where(fmt.Sprintf("%s = ?", key), value)
		}
	}
	return db
}
