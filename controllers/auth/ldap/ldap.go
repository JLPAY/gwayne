package ldap

import (
	"errors"

	"github.com/JLPAY/gwayne/controllers/auth"
	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/myldap"
	"k8s.io/klog/v2"
)

// 统一对外错误信息：不区分「用户不存在 / 多条匹配 / 密码错误」，
// 避免被用作存在性或数量预言来枚举目录账户；内部细节只进日志。
const errInvalidCredentials = "invalid username or password"

// 实现了LDAP认证方式
type LDAPAuthProvider struct{}

func init() {
	auth.Register(models.AuthTypeLDAP, &LDAPAuthProvider{})
	klog.Info("LDAPAuthProvider Registered")
}

func (l *LDAPAuthProvider) Authenticate(authModel models.AuthModel) (*models.User, error) {
	if !config.Conf.Auth.Ldap.Enabled {
		return nil, errors.New(errInvalidCredentials)
	}

	// 连接到 LDAP 服务器
	ldapClient, err := myldap.NewLDAPClient(config.Conf.Auth.Ldap)
	if err != nil {
		klog.Errorf("ldap connect error: %v", err)
		return nil, errors.New(errInvalidCredentials)
	}
	defer ldapClient.Close()

	// 查找用户（username 在 SearchUsers 内部已做过滤器转义）
	entrys, err := ldapClient.SearchUsers(config.Conf.Auth.Ldap.Filter, authModel.Username)
	if err != nil {
		// 这里包含过滤器编译错误、user not found 等内部细节，不回显给客户端
		klog.Warningf("ldap search user %q error: %v", authModel.Username, err)
		return nil, errors.New(errInvalidCredentials)
	}

	if len(entrys) == 0 {
		klog.Warning("Not found an entry.")
		return nil, errors.New(errInvalidCredentials)
	} else if len(entrys) != 1 {
		klog.Warning("Found more than one entry.")
		return nil, errors.New(errInvalidCredentials)
	}

	// 验证密码
	userDN := entrys[0].DN
	err = ldapClient.Conn.Bind(userDN, authModel.Password)
	if err != nil {
		klog.Warningf("ldap bind failed for DN %s: %v", userDN, err)
		return nil, errors.New(errInvalidCredentials)
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
