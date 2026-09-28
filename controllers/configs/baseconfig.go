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

	// 收集所有已启用的 OAuth2 provider
	providers := []map[string]string{}
	for name, conf := range config.Conf.Auth.Oauth2 {
		if !conf.Enabled {
			continue
		}
		displayName := conf.Name
		if displayName == "" {
			displayName = name
		}
		title := strings.ToUpper(displayName[:1]) + displayName[1:] + " Login"
		providers = append(providers, map[string]string{
			"name":        displayName,
			"redirectURL": conf.RedirectURL,
			"title":       title,
		})
	}
	configMap["oauth2Login"] = len(providers) > 0
	configMap["oauth2Providers"] = providers
	configMap["enableApiKeys"] = true

	// 登录框标题
	configMap["system.title"] = "gwayne"
	if len(providers) == 1 {
		// 单 provider 向后兼容
		configMap["oauth2Name"] = providers[0]["name"]
		configMap["oauth2RedirectURL"] = providers[0]["redirectURL"]
		configMap["system.oauth2-title"] = providers[0]["title"]
	} else if len(providers) > 1 {
		configMap["system.oauth2-title"] = "OAuth 2.0 Login"
	} else {
		configMap["system.oauth2-title"] = "OAuth 2.0 Login"
	}
	configMap["system.api-name-generate-rule"] = "join"

	data := ResponseResult{configMap}
	c.JSON(http.StatusOK, data)
}
