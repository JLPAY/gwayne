package app

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

// 定义返回数据结构体
type Detail struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type DataResponse struct {
	Total   int      `json:"total"`
	Details []Detail `json:"details"`
}

type ApiResponse struct {
	Data DataResponse `json:"data"`
}

func AppStatistics(c *gin.Context) {
	// 构造响应数据
	response := ApiResponse{
		Data: DataResponse{
			Total: 0,
			Details: []Detail{
				{
					Name:  "demo",
					Count: 0,
				},
			},
		},
	}

	// 返回 JSON 响应
	c.JSON(http.StatusOK, response)
}

func Names(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": []string{""}})
}

func UserStatistics(c *gin.Context) {
	// 构造响应结构体
	response := gin.H{
		"data": gin.H{
			"total": 0,
		},
	}

	// 返回 JSON 响应
	c.JSON(http.StatusOK, response)
}
