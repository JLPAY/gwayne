package db

import (
	"fmt"
	"github.com/JLPAY/gwayne/controllers/auth"
	"github.com/JLPAY/gwayne/models"
	"github.com/JLPAY/gwayne/pkg/encode"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
)

// 实现了数据库认证方式
type DBAuthProvider struct{}

func init() {
	auth.Register(models.AuthTypeDB, &DBAuthProvider{})
	klog.Info("DBAuthProvider Registered")
}

func (dbAuth *DBAuthProvider) Authenticate(authModel models.AuthModel) (*models.User, error) {
	// 根据用户名查找用户
	user, err := models.GetUserByName(authModel.Username)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("username or password error!")
		}
		return nil, err
	}

	// 校验密码
	if user.Password == "" || user.Salt == "" {
		return nil, fmt.Errorf("user dons't support db login!")
	}

	passwordHashed := encode.EncodePassword(authModel.Password, user.Salt)

	if passwordHashed != user.Password {
		return nil, fmt.Errorf("username or password error!")
	}
	return user, nil
}
