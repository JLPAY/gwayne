package configs

import (
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/gin-gonic/gin"
	"net/http"
	"reflect"
	"strings"
)

// @Title GetConfig
// @Description get system config
// @Success 200 {object} Config success
// @router /system [get]
func ListSystem(c *gin.Context) {
	cfg := config.GetConfig()

	// 获取并打印配置项 JSON 格式
	result := convertToJSON(cfg)

	c.JSON(http.StatusOK, result)
}

func convertToJSON(config config.Config) ResponseResult {
	output := make(map[string]interface{})

	// 遍历配置项，并将 Section 和 Key 组合，最终输出完整的 Section.Key = value 格式的配置项
	buildJSONMap(reflect.ValueOf(config), output, "")

	return ResponseResult{output}
}

func buildJSONMap(v reflect.Value, output map[string]interface{}, parentKey string) {
	// 确保 v 是一个结构体
	if v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			fieldValue := v.Field(i)
			jsonTag := field.Tag.Get("ini") // 获取 ini 标签
			if jsonTag == "" {
				jsonTag = field.Name // 没有标签则使用字段名
			}

			// 生成新的键
			newKey := parentKey + jsonTag

			// 检查字段名是否包含 "password" 或 "secret"
			if strings.Contains(strings.ToLower(newKey), "password") ||
				strings.Contains(strings.ToLower(newKey), "secret") {
				// 替换为 "*******"
				output[newKey] = "*******" // 替换为 "*****"
			} else {
				if fieldValue.Kind() == reflect.Struct {
					// 递归处理嵌套结构体
					buildJSONMap(fieldValue, output, newKey+".")
				} else {
					// 直接添加到输出
					output[newKey] = fieldValue.Interface()
				}
			}
		}
	}
}
