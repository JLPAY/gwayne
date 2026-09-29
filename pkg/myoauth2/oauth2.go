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
	LoginUrl     string            // opsmanage 登录页 URL，用于发起 SSO 流程
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

// 初始化所有 OAuth2 服务
func NewOAuth2Service() {
	if len(config.Conf.Auth.Oauth2) == 0 {
		klog.Infof("No OAuth2 services configured.")
		return
	}

	for name, conf := range config.Conf.Auth.Oauth2 {
		if !conf.Enabled {
			klog.Infof("OAuth2 service '%s' is not enabled, skipping.", name)
			continue
		}
		initOAuth2Provider(name, conf)
	}
}

// 初始化单个 OAuth2 provider
func initOAuth2Provider(name string, conf config.Oauth2Conf) {
	// 如果 name 为空，使用 section key 作为 name
	if conf.Name == "" {
		conf.Name = name
		klog.Infof("OAuth2 Name not configured for '%s', using section key as name", name)
	}

	// 解析 Scopes
	scopes := []string{}
	if conf.Scopes != "" {
		scopesStr := strings.TrimSpace(conf.Scopes)
		if scopesStr != "" {
			scopesList := strings.Split(scopesStr, ",")
			for _, scope := range scopesList {
				scope = strings.TrimSpace(scope)
				if scope != "" {
					scopes = append(scopes, scope)
				}
			}
		}
	}

	info := &OAuth2Info{
		ClientId:     conf.ClientId,
		ClientSecret: conf.ClientSecret,
		Scopes:       scopes,
		AuthUrl:      conf.AuthURL,
		TokenUrl:     conf.TokenURL,
		ApiUrl:       conf.ApiURL,
		LoginUrl:     conf.LoginURL,
		Enabled:      conf.Enabled,
	}

	// 解析 API 字段映射
	info.ApiMapping = make(map[string]string)
	if conf.ApiMapping != "" {
		for _, mapping := range strings.Split(conf.ApiMapping, ",") {
			parts := strings.Split(mapping, ":")
			if len(parts) == 2 {
				info.ApiMapping[parts[0]] = parts[1]
			}
		}
	}

	// 将 OAuth2Info 存储到全局映射
	OAuth2Infos[name] = info

	// 创建 OAuth2 配置
	redirectURL := fmt.Sprintf("%s/login/oauth2/%s", conf.RedirectURL, name)
	oauth2Config := oauth2.Config{
		ClientID:     info.ClientId,
		ClientSecret: info.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  info.AuthUrl,
			TokenURL: info.TokenUrl,
		},
		RedirectURL: redirectURL,
		Scopes:      info.Scopes,
	}

	klog.Infof("OAuth2 provider '%s' redirect_uri: %s", name, redirectURL)

	// 创建 OAuth2 默认实现
	OAutherMap[name] = &OAuth2Default{
		Config:     &oauth2Config,
		ApiUrl:     info.ApiUrl,
		ApiMapping: info.ApiMapping,
	}

	klog.Infof("OAuth2 service '%s' initialized successfully, redirect_url: %s", name, oauth2Config.RedirectURL)
}
