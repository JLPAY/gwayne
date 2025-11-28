package ldap

import (
	"fmt"

	"github.com/JLPAY/gwayne/controllers/auth"
	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/myldap"
	"k8s.io/klog/v2"
)

// 实现了LDAP认证方式
type LDAPAuthProvider struct{}

func init() {
	auth.Register(models.AuthTypeLDAP, &LDAPAuthProvider{})
	klog.Info("LDAPAuthProvider Registered")
}

func (l *LDAPAuthProvider) Authenticate(authModel models.AuthModel) (*models.User, error) {
	if !config.Conf.Auth.Ldap.Enabled {
		return nil, fmt.Errorf("Ldap authentication is disabled")
	}

	// 连接到 LDAP 服务器
	ldapClient, err := myldap.NewLDAPClient(config.Conf.Auth.Ldap)
	if err != nil {
		return nil, err
	}
	defer ldapClient.Close()

	// 查找用户
	entrys, err := ldapClient.SearchUsers(config.Conf.Auth.Ldap.Filter, authModel.Username)
	if err != nil {
		return nil, err
	}

	if len(entrys) == 0 {
		klog.Warning("Not found an entry.")
		return nil, fmt.Errorf("Not found an entry. ")
	} else if len(entrys) != 1 {
		klog.Warning("Found more than one entry.")
		return nil, fmt.Errorf("Found more than one entry. ")
	}

	// 验证密码
	userDN := entrys[0].DN
	err = ldapClient.Conn.Bind(userDN, authModel.Password)
	if err != nil {
		return nil, fmt.Errorf("invalid username or password")
	}

	// 映射 LDAP 用户信息到 User 结构体
	user := &models.User{
		Name:    authModel.Username,
		Email:   entrys[0].GetAttributeValue("mail"),
		Admin:   false, // 根据需要设置权限
		Display: entrys[0].GetAttributeValue("cn"),
	}

	//klog.Infof("user: %s ldap login!", user.Name)
	return user, nil
}
