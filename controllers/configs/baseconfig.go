package configs

import (
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/gin-gonic/gin"
	"net/http"
)

type ResponseResult struct {
	Data map[string]interface{} `json:"data"`
}

// 前端服务获取服务的配置配置信息
func ListBase(c *gin.Context) {
	configMap := make(map[string]interface{})

	configMap["appUrl"] = config.Conf.App.AppUrl
	configMap["betaUrl"] = config.Conf.App.BetaUrl

	configMap["enableDBLogin"] = true
	configMap["appLabelKey"] = "wayne-app"
	configMap["enableRobin"] = false
	configMap["ldapLogin"] = config.Conf.Auth.Ldap.Enabled
	configMap["oauth2Login"] = config.Conf.Auth.Oauth2.Enabled
	configMap["enableApiKeys"] = true

	// 登录框标题
	configMap["system.title"] = "gwayne"
	configMap["system.oauth2-title"] = "GitHub Login"
	configMap["system.api-name-generate-rule"] = "join"

	data := ResponseResult{configMap}
	c.JSON(http.StatusOK, data)
}
