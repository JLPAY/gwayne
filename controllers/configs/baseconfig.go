package configs

import (
	"net/http"
	"strings"

	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/gin-gonic/gin"
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
	// 使用 OAuth2 Name 配置生成标题
	if config.Conf.Auth.Oauth2.Enabled {
		oauth2Name := config.Conf.Auth.Oauth2.Name
		if oauth2Name == "" {
			oauth2Name = "oauth2"
		}
		// 返回 OAuth2 服务名称
		configMap["oauth2Name"] = oauth2Name
		// 返回 OAuth2 RedirectURL，用于前端跳转到后端
		configMap["oauth2RedirectURL"] = config.Conf.Auth.Oauth2.RedirectURL
		// 将名称首字母大写，然后加上 " Login"
		oauth2Title := strings.ToUpper(oauth2Name[:1]) + oauth2Name[1:] + " Login"
		configMap["system.oauth2-title"] = oauth2Title
	} else {
		configMap["system.oauth2-title"] = "OAuth 2.0 Login"
	}
	configMap["system.api-name-generate-rule"] = "join"

	data := ResponseResult{configMap}
	c.JSON(http.StatusOK, data)
}
