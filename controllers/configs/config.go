package configs

import (
	"net/http"
	"reflect"
	"strings"

	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/gin-gonic/gin"
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

// sensitiveKeyWords 命中即脱敏：签名密钥、私钥路径、凭据、数据库连接、LDAP 绑定 DN 等
var sensitiveKeyWords = []string{
	"password", "secret", "appkey", "privatekey", "binddn",
	"database.host", "database.dbuser", "database.dbname", "database.charset",
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, w := range sensitiveKeyWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

func buildJSONMap(v reflect.Value, output map[string]interface{}, parentKey string) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			fieldValue := v.Field(i)
			jsonTag := field.Tag.Get("ini") // 获取 ini 标签
			if jsonTag == "" {
				jsonTag = field.Name // 没有标签则使用字段名
			}

			// 生成新的键
			newKey := parentKey + jsonTag

			if isSensitiveKey(newKey) {
				output[newKey] = "*******"
				continue
			}

			if fieldValue.Kind() == reflect.Struct || fieldValue.Kind() == reflect.Map {
				// 嵌套结构体/Map 递归处理（如 Auth.Oauth2 是 map，需深入脱敏）
				buildJSONMap(fieldValue, output, newKey+".")
			} else {
				output[newKey] = fieldValue.Interface()
			}
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			key := k.Interface()
			keyStr := reflect.ValueOf(key).String()
			newKey := parentKey + keyStr
			mapValue := v.MapIndex(k)

			if isSensitiveKey(newKey) {
				output[newKey] = "*******"
				continue
			}

			if mapValue.Kind() == reflect.Struct || mapValue.Kind() == reflect.Map {
				buildJSONMap(mapValue, output, newKey+".")
			} else {
				output[newKey] = mapValue.Interface()
			}
		}
	}
}
