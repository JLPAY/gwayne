package oauth2

import (
	"context"
	"fmt"

	"github.com/JLPAY/gwayne/controllers/auth"
	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/myoauth2"
	"k8s.io/klog/v2"
)

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 实现了OAuth2认证方式
type OAuth2AuthProvider struct{}

func init() {
	auth.Register(models.AuthTypeOAuth2, &OAuth2AuthProvider{})
	klog.Info("OAuth2AuthProvider Registered")
}

func (p *OAuth2AuthProvider) Authenticate(authModel models.AuthModel) (*models.User, error) {
	if !config.Conf.Auth.Oauth2.Enabled {
		return nil, fmt.Errorf("OAuth2 authentication is disabled")
	}

	code := authModel.OAuth2Code
	//klog.Info("OAuth2Code: ", code)

	// 获取 OAuth2 配置信息
	oauth2Config, ok := myoauth2.OAutherMap[authModel.OAuth2Name]
	if !ok {
		return nil, fmt.Errorf("OAuth2 service %s not found", authModel.OAuth2Name)
	}

	// 通过 OAuth2 Code 获取 Token
	klog.Infof("OAuth2: Exchanging authorization code for token, code: %s", code)
	token, err := oauth2Config.Exchange(context.Background(), authModel.OAuth2Code)
	if err != nil {
		klog.Errorf("Failed to exchange code for token: %v", err)
		return nil, fmt.Errorf("oauth2 get token by code (%s) error.%v", code, err)
	}
	klog.Infof("OAuth2: Token obtained successfully, AccessToken: %s, TokenType: %s, Expiry: %v",
		token.AccessToken[:min(20, len(token.AccessToken))]+"...", token.TokenType, token.Expiry)

	// 使用获取到的 OAuth2 令牌获取用户信息（UserInfo 方法内部会使用服务的 ApiURL）
	klog.Infof("OAuth2: Fetching user info from provider")
	userInfo, err := oauth2Config.UserInfo(token)
	if err != nil {
		klog.Errorf("Failed to get user info from OAuth2 provider: %v", err)
		return nil, fmt.Errorf("failed to get user info from OAuth2 provider: %v", err)
	}
	klog.Infof("OAuth2: User info received - Name: %s, Email: %s, Display: %s",
		userInfo.Name, userInfo.Email, userInfo.Display)

	// 将获取到的用户信息映射到 User 结构体
	user := &models.User{
		Name:    userInfo.Name,
		Email:   userInfo.Email, // 假设返回的用户信息中有 email 字段
		Admin:   false,          // 根据需要设定用户权限
		Display: userInfo.Display,
	}
	klog.Infof("OAuth2: User object created - Name: %s, Email: %s, Display: %s, Admin: %v",
		user.Name, user.Email, user.Display, user.Admin)

	return user, nil
}
