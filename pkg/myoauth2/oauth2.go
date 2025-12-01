// 定义 oauth2 接口
package myoauth2

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/JLPAY/gwayne/pkg/config"
	"golang.org/x/oauth2"
	"k8s.io/klog/v2"
)

func init() {
	NewOAuth2Service()
}

// OAuth2 配置信息和授权处理器的全局映射
var (
	OAuth2Infos = make(map[string]*OAuth2Info) // 存储 OAuth2 服务配置信息
	OAutherMap  = make(map[string]OAuther)     // 存储 OAuth2 认证接口实现
)

const (
	OAuth2TypeDefault = "oauth2"
)

// OAuth2 用户的基本信息
type BasicUserInfo struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Display string `json:"display"`
}

// OAuth2 服务的配置信息
type OAuth2Info struct {
	ClientId     string            // 客户端 ID
	ClientSecret string            // 客户端 Secret
	Scopes       []string          // 授权的 Scope
	AuthUrl      string            // OAuth2 授权 URL
	TokenUrl     string            // OAuth2 Token URL
	ApiUrl       string            // 获取用户信息的 API URL
	Enabled      bool              // 是否启用 OAuth2 服务
	ApiMapping   map[string]string // API 字段映射
}

// 接口定义了 OAuth2 服务的方法
type OAuther interface {
	// 通过令牌获取用户信息
	UserInfo(token *oauth2.Token) (*BasicUserInfo, error)

	// 生成 OAuth2 授权 URL
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
	//使用授权码换取 OAuth2 访问令牌
	Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
	// 基于访问令牌生成一个 HTTP 客户端，便于访问 OAuth2 提供商的 API
	Client(ctx context.Context, t *oauth2.Token) *http.Client
}

// 初始化 Auth2Service
func NewOAuth2Service() {
	// 如果 OAuth2 服务未启用，跳过
	if !config.Conf.Auth.Oauth2.Enabled {
		klog.Infof("OAuth2 service is not enabled, skipping.")
		return
	}

	// 获取 OAuth2 服务名称，如果未配置则使用默认值
	name := config.Conf.Auth.Oauth2.Name
	if name == "" {
		name = OAuth2TypeDefault
		klog.Infof("OAuth2 Name not configured, using default name: %s", name)
	}

	// 加载 OAuth2 配置信息
	info := &OAuth2Info{
		ClientId:     config.Conf.Auth.Oauth2.ClientId,
		ClientSecret: config.Conf.Auth.Oauth2.ClientSecret,
		Scopes:       strings.Split(config.Conf.Auth.Oauth2.Scopes, ","),
		AuthUrl:      config.Conf.Auth.Oauth2.AuthURL,
		TokenUrl:     config.Conf.Auth.Oauth2.TokenURL,
		ApiUrl:       config.Conf.Auth.Oauth2.ApiURL,
		Enabled:      config.Conf.Auth.Oauth2.Enabled,
	}

	// 解析 API 字段映射
	info.ApiMapping = make(map[string]string)
	if config.Conf.Auth.Oauth2.ApiMapping != "" {
		for _, mapping := range strings.Split(config.Conf.Auth.Oauth2.ApiMapping, ",") {
			parts := strings.Split(mapping, ":")
			if len(parts) == 2 {
				info.ApiMapping[parts[0]] = parts[1]
			}
		}
	}

	// 将 OAuth2Info 存储到全局映射，使用配置的 name 作为 key
	OAuth2Infos[name] = info

	// 创建 OAuth2 配置，使用配置的 name 组合 redirect_url
	oauth2Config := oauth2.Config{
		ClientID:     info.ClientId,
		ClientSecret: info.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  info.AuthUrl,
			TokenURL: info.TokenUrl,
		},
		RedirectURL: fmt.Sprintf("%s/login/oauth2/%s", config.Conf.Auth.Oauth2.RedirectURL, name),
		Scopes:      info.Scopes,
	}

	// 创建 OAuth2 默认实现，使用配置的 name 作为 key
	OAutherMap[name] = &OAuth2Default{
		Config:     &oauth2Config,
		ApiUrl:     info.ApiUrl,
		ApiMapping: info.ApiMapping,
	}

	klog.Infof("OAuth2 service '%s' initialized successfully, redirect_url: %s", name, oauth2Config.RedirectURL)
}
