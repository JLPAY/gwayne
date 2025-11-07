package middleware

import (
	"fmt"
	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/rsakey"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
	"net/http"
	"strings"
)

func JWTauth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// jwt 鉴权取头部信息 token 登录时回返回token信息
		// 从请求头中获取JWT
		authHeader := c.GetHeader("Authorization")
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			klog.Errorf("Auth Invalid token: %s", authHeader)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format"})
			c.Abort()
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
			c.Abort()
			return
		}

		// 获取 JWT 声明
		claims := token.Claims.(jwt.MapClaims)
		username := claims["aud"].(string)
		user, err := models.GetUserDetail(username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		// 将用户信息放入上下文
		c.Set("User", user)
		c.Next()
	}

}
