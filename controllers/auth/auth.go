package auth

import (
	"fmt"
	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/myoauth2"
	"github.com/JLPAY/gwayne/pkg/rsakey"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"k8s.io/klog/v2"
	"net/http"
	"strings"
	"time"
)

// Authenticator 定义了认证器的接口
type Authenticator interface {
	Authenticate(authModel models.AuthModel) (*models.User, error)
}

// 注册器，用于注册认证器
var registry = make(map[string]Authenticator)

// Register 用于注册认证器
func Register(name string, authenticator Authenticator) {
	if _, exists := registry[name]; exists {
		// 如果已注册，直接返回
		return
	}
	registry[name] = authenticator
}

// 用于绑定前端传递的登录请求体
type LoginData struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginToken struct {
	Token string `json:"token" binding:"required"`
}

type LoginResponse struct {
	Data LoginToken `json:"data"`
}

// 处理用户登录请求
func Login(c *gin.Context) {
	var loginData LoginData

	// oauth2 的回调接口为 GET： /login/oauth2/oauth2?code=104f908a1b5f3ee3
	if err := c.ShouldBindJSON(&loginData); err != nil && c.Request.Method != http.MethodGet {
		c.JSON(http.StatusBadRequest, gin.H{"error": "登录参数无效"})
		return
	}

	// 从 URL 中获取认证类型
	authType := c.Param("type")
	oauth2Name := c.Param("name")
	next := c.Param("next")
	//state := c.DefaultQuery("state", "5x2zlMe")

	klog.Info("next:", next)

	// 如果认证类型为空或用户名为 'admin'，默认使用数据库认证
	if authType == "" || loginData.Username == "admin" {
		authType = models.AuthTypeDB
	}

	klog.Infof("auth type is %s", authType)

	// 查找对应的认证器
	authenticator, exists := registry[authType]
	if !exists {
		klog.Errorf("不支持的认证类型 %s", authType)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("不支持的认证类型 (%s)", authType)})
		return
	}

	// 创建认证模型
	authModel := models.AuthModel{
		Username: loginData.Username,
		Password: loginData.Password,
	}

	if authType == models.AuthTypeOAuth2 {
		oauther, ok := myoauth2.OAutherMap[oauth2Name]
		if !ok {
			klog.Warningf("oauth2 type (%s) is not supported . ", oauth2Name)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("不支持的认证类型 (%s)", oauth2Name)})
			return
		}
		// 获取回调授权码
		code := c.DefaultQuery("code", "")
		if code == "" {
			// 如果没有获取到 code，重定向到 OAuth2 授权 URL
			// 生成 OAuth2 授权 URL
			authURL := oauther.AuthCodeURL(next, oauth2.AccessTypeOnline)
			// 打印出生成的跳转 URL
			klog.Info("Redirecting to URL: ", authURL)

			c.Redirect(http.StatusFound, oauther.AuthCodeURL(next, oauth2.AccessTypeOnline))
			return
		}

		authModel.OAuth2Code = code
		authModel.OAuth2Name = oauth2Name
	}

	// 调用认证方法
	user, err := authenticator.Authenticate(authModel)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// 更新用户登录信息
	now := time.Now()
	user.LastIp = c.ClientIP()
	user.LastLogin = &now // 确保这是一个有效的指针
	user, err = models.EnsureUser(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 生成JWT
	apiToken, err := generateJWT(user)
	if err != nil {
		klog.Errorf("Error generating JWT: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error generating JWT"})
		return
	}

	loginResponse := LoginResponse{
		Data: LoginToken{
			Token: apiToken,
		},
	}

	c.JSON(http.StatusOK, loginResponse)
}

func Logout(c *gin.Context) {
}

func CurrentUser(c *gin.Context) {
	// 从请求头中获取JWT
	authHeader := c.GetHeader("Authorization")
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		klog.Errorf("Auth Invalid token: %s", authHeader)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format"})
		return
	}

	tokenString := parts[1]
	// 解析JWT
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 检查签名方法是否正确,是否是使用 RSA 私钥
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// 使用公钥验证签名
		return rsakey.RsaPublicKey, nil
	})
	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
		return
	}
	//klog.Infof("token: %v", token)

	// 获取 JWT 声明
	claims := token.Claims.(jwt.MapClaims)
	username := claims["aud"].(string)
	user, err := models.GetUserDetail(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// 生成JWT
func generateJWT(user *models.User) (string, error) {
	// 使用jwt-go生成JWT令牌 ,default token exp time is 3600s.
	expSecond := config.Conf.App.TokenLifeTime

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		// 签发者
		"iss": "gwayne",
		// 签发时间
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Duration(expSecond) * time.Second).Unix(),
		"aud": user.Name,
	})

	return token.SignedString(rsakey.RsaPrivateKey)
}
