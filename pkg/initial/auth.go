package initial

import (
	_ "github.com/JLPAY/gwayne/controllers/auth/db"
	_ "github.com/JLPAY/gwayne/controllers/auth/ldap"
	_ "github.com/JLPAY/gwayne/controllers/auth/oauth2"
)

/*
func init() {
	// 初始化认证器注册
	auth.Register(models.AuthTypeDB, &db.DBAuthProvider{})
	auth.Register(models.AuthTypeOAuth2, &oauth2.OAuth2AuthProvider{})
	auth.Register(models.AuthTypeLDAP, &ldap.LDAPAuthProvider{})

}
*/
