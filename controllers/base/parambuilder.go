package base

import (
	"github.com/JLPAY/gwayne/pkg/pagequery"
	"github.com/JLPAY/gwayne/pkg/snaker"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"net/http"
	"strconv"
	"strings"
)

// 从 gin 的 context 中获取分页参数、过滤条件、排序方式等，生成一个 `QueryParam` 结构体。
func BuildQueryParam(ctx *gin.Context) *pagequery.QueryParam {
	no, size := buildPageParam(ctx)

	klog.V(3).Infof("分页参数no: %d, size: %d", no, size)

	qmap := map[string]interface{}{}
	deletedStr := ctx.DefaultQuery("deleted", "")
	if deletedStr != "" {
		deleted, err := strconv.ParseBool(deletedStr)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid deleted in query."})
		}
		qmap["deleted"] = deleted
	}

	// 处理 "filter" 参数，允许通过多个键值对（逗号分隔）进行查询
	filter := ctx.DefaultQuery("filter", "")
	if filter != "" {
		filters := strings.Split(filter, ",")
		for _, param := range filters {
			params := strings.Split(param, "=")
			if len(params) != 2 {
				// 忽略无效的过滤条件
				continue
			}
			key, value := params[0], params[1]
			// 兼容在filter中使用deleted参数
			if key == "deleted" {
				deleted, err := strconv.ParseBool(value)
				if err != nil {
					continue
				}
				qmap[key] = deleted
				continue
			}
			qmap[params[0]] = params[1]
		}
	}

	relate := ctx.DefaultQuery("relate", "")

	// 处理 "sortby" 参数，将 CamelCase 转换为 snake_case 格式
	//sortby := snaker.CamelToSnake(ctx.DefaultQuery("sortby", ""))
	sortby := snaker.SnakeToCamelLower(ctx.DefaultQuery("sortby", ""))

	klog.V(3).Infof("分布参数filter: %s,relate: %s, sortby: %s", filter, relate, sortby)

	return &pagequery.QueryParam{
		PageNo:   no,     // 当前页码
		PageSize: size,   // 每页大小
		Query:    qmap,   // 查询条件
		Sortby:   sortby, // 排序字段（已转换为 snake_case）
		Relate:   relate, // 关联查询参数
	}
}

func buildPageParam(ctx *gin.Context) (no int64, size int64) {
	// 获取分页参数
	pageNo := ctx.DefaultQuery("pageNo", strconv.Itoa(defaultPageNo))
	pageSize := ctx.DefaultQuery("pageSize", strconv.Itoa(defaultPageSize))

	no, err := strconv.ParseInt(pageNo, 10, 64)
	// pageNo must be greater than zero
	if err != nil || no < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pageNo in query."})
	}

	// pageSize must be greater than zero
	size, err = strconv.ParseInt(pageSize, 10, 64)
	if err != nil || size < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pageSize in query."})
	}
	return no, size
}
