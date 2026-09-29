package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/myoauth2"
	"github.com/JLPAY/gwayne/pkg/rsakey"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// Authenticator 定义了认证器的接口
type Authenticator interface {
	Authenticate(authModel models.AuthModel) (*models.User, error)
}

// 注册器，用于注册认证器
var registry = make(map[string]Authenticator)

// stateStore 临时存储 OAuth2 state 参数与对应的 next URL 和 provider name
type stateEntry struct {
	next      string
	provider  string
	createdAt time.Time
}

var (
	stateMu    sync.Mutex
	stateStore = make(map[string]stateEntry)
)

func generateState(next string, provider string) string {
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)

	stateMu.Lock()
	stateStore[id] = stateEntry{next: next, provider: provider, createdAt: time.Now()}
	// 清理超过 10 分钟的过期条目
	for k, v := range stateStore {
		if time.Since(v.createdAt) > 10*time.Minute {
			delete(stateStore, k)
		}
	}
	stateMu.Unlock()

	return id
}

func consumeState(id string) (next string, provider string, ok bool) {
	stateMu.Lock()
	defer stateMu.Unlock()
	entry, exists := stateStore[id]
	if exists {
		delete(stateStore, id)
		return entry.next, entry.provider, true
	}
	return "", "", false
}

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
	next := c.Query("next")   // 用于初始请求
	state := c.Query("state") // OAuth2 回调时的 state 参数

	klog.Infof("Login request - authType: %s, oauth2Name: %s, next: %s, state: %s", authType, oauth2Name, next, state)

	// 如果是 OAuth2 认证，直接处理，不进行默认转换
	// 如果认证类型为空且不是 OAuth2，或用户名为 'admin'，默认使用数据库认证
	if authType == models.AuthTypeOAuth2 {
		// OAuth2 认证，保持 authType 不变
	} else if authType == "" || loginData.Username == "admin" {
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
		// 如果 oauth2Name 为空，尝试使用默认值
		if oauth2Name == "" {
			if len(myoauth2.OAutherMap) == 1 {
				// 只有一个 provider，自动使用
				for name := range myoauth2.OAutherMap {
					oauth2Name = name
				}
			} else if len(myoauth2.OAutherMap) > 1 {
				// 多个 provider 时必须指定
				klog.Errorf("Multiple OAuth2 services configured, please specify service name in URL")
				c.JSON(http.StatusBadRequest, gin.H{"error": "已配置多个 OAuth2 服务，请指定服务名称"})
				return
			} else {
				klog.Errorf("No OAuth2 services configured")
				c.JSON(http.StatusBadRequest, gin.H{"error": "未配置 OAuth2 服务"})
				return
			}
		}

		// 获取回调授权码
		code := c.DefaultQuery("code", "")
		klog.Infof("OAuth2 login - code: %s, oauth2Name: %s, state: %s", code, oauth2Name, state)

		if code == "" {
			// 初始请求：重定向到 OAuth2 授权
			oauther, ok := myoauth2.OAutherMap[oauth2Name]
			if !ok {
				klog.Errorf("OAuth2 service '%s' not found in OAutherMap. Available services: %v", oauth2Name, getOAuth2ServiceNames())
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("不支持的 OAuth2 服务 (%s)", oauth2Name)})
				return
			}

			// 检查是否配置了 LoginURL（opsmanage 模式）
			oauth2Info := myoauth2.OAuth2Infos[oauth2Name]
			if oauth2Info != nil && oauth2Info.LoginUrl != "" {
				// opsmanage 模式：构建 authorize URL 作为 redirect 参数
				// 流程：跳 opsmanage 登录页 → 用户登录 → opsmanage 跳 authorize URL → 自动授权 → 回调
				c.SetCookie("oauth2_next", next, 300, "/", "", false, true)
				c.SetCookie("oauth2_provider", oauth2Name, 300, "/", "", false, true)

				oauth2Conf := config.Conf.Auth.Oauth2[oauth2Name]
				authorizeURL := fmt.Sprintf("%s?client_id=%s&response_type=code&redirect_uri=%s",
					oauth2Info.AuthUrl,
					url.QueryEscape(oauth2Conf.ClientId),
					url.QueryEscape(oauth2Conf.RedirectURL),
				)
				loginURL := oauth2Info.LoginUrl + "?redirect=" + url.QueryEscape(authorizeURL)

				klog.Infof("OAuth2 (opsmanage mode) - redirecting to login page")
				klog.Infof("  - Provider: %s", oauth2Name)
				klog.Infof("  - LoginURL: %s", oauth2Info.LoginUrl)
				klog.Infof("  - Authorize URL (redirect target): %s", authorizeURL)
				c.Redirect(http.StatusFound, loginURL)
				return
			}

			// 标准 OAuth2 模式：生成随机 state，跳转到 authorize 端点
			stateID := generateState(next, oauth2Name)
			authURL := oauther.AuthCodeURL(stateID)
			klog.Infof("OAuth2 authorization request details:")
			klog.Infof("  - OAuth2 service: %s", oauth2Name)
			klog.Infof("  - State parameter: %s", stateID)
			klog.Infof("  - Generated auth URL: %s", authURL)
			klog.Infof("  - Redirecting to OAuth2 provider...")

			c.Redirect(http.StatusFound, authURL)
			return
		}

		// 回调请求：恢复 next URL 和 provider name
		// 优先从 state store 恢复（标准 OAuth2 模式），其次从 cookie 恢复（opsmanage 模式）
		if state != "" {
			if storedNext, storedProvider, ok := consumeState(state); ok {
				next = storedNext
				if storedProvider != "" {
					oauth2Name = storedProvider
				}
				klog.Infof("OAuth2 callback - restored from state store: provider=%s, next=%s", oauth2Name, next)
			}
		}

		// 如果 state store 没有，尝试从 cookie 恢复（opsmanage 模式）
		if next == "" {
			if cookieNext, err := c.Cookie("oauth2_next"); err == nil && cookieNext != "" {
				next = cookieNext
				// 清除 cookie
				c.SetCookie("oauth2_next", "", -1, "/", "", false, true)
			}
			if cookieProvider, err := c.Cookie("oauth2_provider"); err == nil && cookieProvider != "" {
				oauth2Name = cookieProvider
				c.SetCookie("oauth2_provider", "", -1, "/", "", false, true)
			}
			if next != "" {
				klog.Infof("OAuth2 callback - restored from cookie: provider=%s, next=%s", oauth2Name, next)
			}
		}

		// 查找 OAuth2 provider（使用恢复的 provider name）
		if _, ok := myoauth2.OAutherMap[oauth2Name]; !ok {
			klog.Errorf("OAuth2 service '%s' not found in OAutherMap. Available services: %v", oauth2Name, getOAuth2ServiceNames())
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("不支持的 OAuth2 服务 (%s)", oauth2Name)})
			return
		}

		authModel.OAuth2Code = code
		authModel.OAuth2Name = oauth2Name
	}

	// 调用认证方法
	user, err := authenticator.Authenticate(authModel)
	if err != nil {
		klog.Errorf("OAuth2 authentication failed: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// 打印认证成功后的用户信息
	if authType == models.AuthTypeOAuth2 {
		klog.Infof("OAuth2 authentication successful - User: Name=%s, Email=%s, Display=%s, Admin=%v",
			user.Name, user.Email, user.Display, user.Admin)
	}

	// 更新用户登录信息
	now := time.Now()
	user.LastIp = c.ClientIP()
	user.LastLogin = &now // 确保这是一个有效的指针
	user, err = models.EnsureUser(user)
	if err != nil {
		klog.Errorf("Failed to ensure user in database: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 打印更新后的用户信息
	if authType == models.AuthTypeOAuth2 {
		klog.Infof("OAuth2 user saved/updated - User ID: %d, Name: %s, Email: %s, LastLogin: %v, LastIp: %s",
			user.Id, user.Name, user.Email, user.LastLogin, user.LastIp)
	}

	// 生成JWT
	apiToken, err := generateJWT(user)
	if err != nil {
		klog.Errorf("Error generating JWT: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error generating JWT"})
		return
	}

	// 如果是 OAuth2 登录，重定向到前端
	if authType == models.AuthTypeOAuth2 {
		if next != "" {
			// 将 token 作为 sid 参数附加到回调 URL
			// 检查 next 是否已经包含查询参数
			separator := "?"
			if strings.Contains(next, "?") {
				separator = "&"
			}
			redirectURL := fmt.Sprintf("%s%ssid=%s", next, separator, apiToken)
			klog.Infof("OAuth2 login success, redirecting to: %s", redirectURL)
			c.Redirect(http.StatusFound, redirectURL)
			return
		}
		// 如果没有 next 参数，返回 JSON（向后兼容）
		klog.Warning("OAuth2 login success but no next parameter, returning JSON")
	}

	// 其他登录方式返回 JSON
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

	// 获取 JWT 声明（安全断言，避免 claims["aud"] 缺失或类型错误导致 panic）
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
		return
	}
	username, ok := claims["aud"].(string)
	if !ok || username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
		return
	}
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
		"iss": "gwayne",
		"iat": jwt.NewNumericDate(time.Now()),
		"exp": jwt.NewNumericDate(time.Now().Add(time.Duration(expSecond) * time.Second)),
		"aud": user.Name,
	})

	return token.SignedString(rsakey.RsaPrivateKey)
}

// 获取所有已注册的 OAuth2 服务名称（用于调试）
func getOAuth2ServiceNames() []string {
	names := make([]string, 0, len(myoauth2.OAutherMap))
	for name := range myoauth2.OAutherMap {
		names = append(names, name)
	}
	return names
}
