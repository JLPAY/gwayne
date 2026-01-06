package k8sgpt

import (
	"net/http"
	"strconv"

	"github.com/JLPAY/gwayne/models"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// ListAIBackends 获取所有AI后端列表
func ListAIBackends(c *gin.Context) {
	backends, err := models.GetAllAIBackends()
	if err != nil {
		klog.Errorf("获取AI后端列表失败: %v", err)
		errorMsg := "获取AI后端列表失败: " + err.Error()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data":    backends,
	})
}

// GetAIBackend 获取单个AI后端详情
func GetAIBackend(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		errorMsg := "无效的ID参数"
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	backend, err := models.GetAIBackendByID(id)
	if err != nil {
		klog.Errorf("获取AI后端失败: %v", err)
		errorMsg := "AI后端不存在"
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data":    backend,
	})
}

// CreateAIBackend 创建AI后端
func CreateAIBackend(c *gin.Context) {
	var backend models.AIBackend
	if err := c.ShouldBindJSON(&backend); err != nil {
		klog.Errorf("解析请求体失败: %v", err)
		errorMsg := "请求参数错误: " + err.Error()
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	// 验证必填字段
	if backend.Name == "" {
		errorMsg := "后端名称不能为空"
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	if backend.Provider == "" {
		errorMsg := "提供商不能为空"
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	// 检查名称是否已存在
	existing, _ := models.GetAIBackendByName(backend.Name)
	if existing != nil {
		errorMsg := "后端名称已存在"
		c.JSON(http.StatusConflict, gin.H{
			"code":    409,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	id, err := models.AddAIBackend(&backend)
	if err != nil {
		klog.Errorf("创建AI后端失败: %v", err)
		errorMsg := "创建AI后端失败: " + err.Error()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	backend.ID = id
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data":    backend,
	})
}

// UpdateAIBackend 更新AI后端
func UpdateAIBackend(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		errorMsg := "无效的ID参数"
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	var backend models.AIBackend
	if err := c.ShouldBindJSON(&backend); err != nil {
		klog.Errorf("解析请求体失败: %v", err)
		errorMsg := "请求参数错误: " + err.Error()
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	backend.ID = id

	// 检查是否存在
	existing, err := models.GetAIBackendByID(id)
	if err != nil {
		errorMsg := "AI后端不存在"
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	// 如果名称改变，检查新名称是否已存在
	if backend.Name != existing.Name {
		existingByName, _ := models.GetAIBackendByName(backend.Name)
		if existingByName != nil {
			errorMsg := "后端名称已存在"
			c.JSON(http.StatusConflict, gin.H{
				"code":    409,
				"message": "error",
				"msg":     errorMsg,
				"error":   errorMsg,
				"data": gin.H{
					"message": errorMsg,
				},
			})
			return
		}
	}

	err = models.UpdateAIBackend(&backend)
	if err != nil {
		klog.Errorf("更新AI后端失败: %v", err)
		errorMsg := "更新AI后端失败: " + err.Error()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data":    backend,
	})
}

// DeleteAIBackend 删除AI后端
func DeleteAIBackend(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		errorMsg := "无效的ID参数"
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	err = models.DeleteAIBackend(id)
	if err != nil {
		klog.Errorf("删除AI后端失败: %v", err)
		errorMsg := "删除AI后端失败: " + err.Error()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"message": "删除成功",
		},
	})
}

// SetDefaultAIBackend 设置默认AI后端
func SetDefaultAIBackend(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		errorMsg := "无效的ID参数"
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	// 检查是否存在
	_, err = models.GetAIBackendByID(id)
	if err != nil {
		errorMsg := "AI后端不存在"
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	err = models.SetDefaultAIBackend(id)
	if err != nil {
		klog.Errorf("设置默认AI后端失败: %v", err)
		errorMsg := "设置默认AI后端失败: " + err.Error()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "error",
			"msg":     errorMsg,
			"error":   errorMsg,
			"data": gin.H{
				"message": errorMsg,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data": gin.H{
			"message": "设置默认后端成功",
		},
	})
}

