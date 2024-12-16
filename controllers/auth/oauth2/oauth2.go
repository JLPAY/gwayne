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
		return nil, fmt.Errorf("OAuth2 authentication is disabled")
	}

	// 通过 OAuth2 Code 获取 Token
	token, err := oauth2Config.Exchange(context.Background(), authModel.OAuth2Code)
	if err != nil {
		klog.Errorf("Failed to exchange code for token: %v", err)
		return nil, fmt.Errorf("oauth2 get token by code (%s) error.%v", code, err)
	}

	// 使用 Token 访问 API 获取用户信息
	client := oauth2Config.Client(context.Background(), token)
	resp, err := client.Get(config.Conf.Auth.Oauth2.ApiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info from OAuth2 provider: %v", err)
	}
	defer resp.Body.Close()

	// 使用获取到的 OAuth2 令牌获取用户信息
	userInfo, err := oauth2Config.UserInfo(token)
	if err != nil {
		//c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to fetch user info: %v", err)})
		return nil, fmt.Errorf("failed to get user info from OAuth2 provider: %v", err)
	}

	// 将获取到的用户信息映射到 User 结构体
	user := &models.User{
		Name:    userInfo.Name,
		Email:   userInfo.Email, // 假设返回的用户信息中有 email 字段
		Admin:   false,          // 根据需要设定用户权限
		Display: userInfo.Display,
	}

	return user, nil
}
