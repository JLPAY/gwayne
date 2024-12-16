package models

import (
	"github.com/JLPAY/gwayne/pkg/encode"
	"gorm.io/gorm"
	"time"
)

type UserType int

const (
	DefaultUser UserType = iota // 普通用户
	SystemUser                  // 系统用户
	APIUser                     // API用户

	TableNameUser = "user" // 表名
)

var (
	APIKeyUser = User{
		Id:      0,
		Name:    "OpenAPI",
		Type:    APIUser,
		Display: "OpenAPI",
	}

	AnonymousUser = User{
		Id:      0,
		Name:    "Anonymous",
		Type:    DefaultUser,
		Display: "Anonymous",
	}
)

type User struct {
	Id         int64      `gorm:"primaryKey;autoIncrement" json:"id,omitempty"` // 主键自增
	Name       string     `gorm:"uniqueIndex;size:200" json:"name,omitempty"`   // 用户名唯一索引
	Password   string     `gorm:"size:255" json:"-"`                            // 密码，json 序列化时忽略
	Salt       string     `gorm:"size:32" json:"-"`                             // 密码盐，用于密码加密
	Email      string     `gorm:"size:200" json:"email,omitempty"`
	Display    string     `gorm:"size:200" json:"display,omitempty"`
	Comment    string     `gorm:"type:text" json:"comment,omitempty"`
	Type       UserType   `gorm:"type:int" json:"type"`                       // 用户类型
	Admin      bool       `gorm:"default:false" json:"admin"`                 // 是否为管理员
	LastLogin  *time.Time `gorm:"autoUpdateTime" json:"lastLogin,omitempty"`  // 最后登录时间
	LastIp     string     `gorm:"size:200" json:"lastIp,omitempty"`           // 最后登录 IP
	Deleted    bool       `gorm:"default:false" json:"deleted,omitempty"`     // 是否被删除
	CreateTime *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"` // 创建时间
	UpdateTime *time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"` // 更新时间
}

// 表名，不使用默认的复数形式
func (*User) TableName() string {
	return TableNameUser
}

func (u *User) GetTypeName() string {
	mapDict := map[UserType]string{
		DefaultUser: "default",
		SystemUser:  "system",
		APIUser:     "api",
	}
	name, ok := mapDict[u.Type]
	if ok == false {
		return ""
	}
	return name
}

// 新增用户
func AddUser(user *User) (id int64, err error) {
	if err := DB.Create(user).Error; err != nil {
		return 0, err
	}
	// 返回插入的 ID 和 nil 错误
	return int64(user.Id), nil
}

func GetAllUsers() ([]User, error) {
	users := []User{}
	err := DB.Select("id, name").Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

func GetUserById(id int64) (*User, error) {
	var user User
	if err := DB.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByName(name string) (*User, error) {
	var user User
	if err := DB.Where("name = ?", name).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserDetail(name string) (*User, error) {
	var user User
	if err := DB.Where("name = ?", name).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// 确保用户存在，没有则创建
func EnsureUser(user *User) (*User, error) {
	var existingUser User

	// 查询数据库中是否存在该用户
	if err := DB.Where("name = ?", user.Name).First(&existingUser).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 如果找不到该用户，则添加用户
			if err := DB.Create(user).Error; err != nil {
				return nil, err
			}
			// 返回新创建的用户
			return user, nil
		}
		// 其它错误处理
		return nil, err
	} else {
		// 如果找到了用户,更新用户 Email Display LastLogin  LastIp
		existingUser.Email = user.Email
		existingUser.Display = user.Display
		existingUser.LastLogin = user.LastLogin
		existingUser.LastIp = user.LastIp
		//err := DB.Save(existingUser).Error
		err := DB.Select("Email", "Display", "LastLogin", "LastIp").Updates(existingUser).Error
		if err != nil {
			return nil, err
		}
	}

	return &existingUser, nil
}

func UpdateUserAdmin(user *User) (err error) {
	v := &User{Id: user.Id}

	if err = DB.Where("id = ?", v.Id).First(&v).Error; err != nil {
		return
	}
	v.Admin = user.Admin
	return DB.Save(&v).Error
}

func ResetUserPassword(id int64, password string) (err error) {
	v := &User{Id: id}
	if err = DB.Where("id = ?", v.Id).First(&v).Error; err != nil {
		return
	}
	salt := encode.GetRandomString(10)
	passwordHashed := encode.EncodePassword(password, salt)

	v.Password = passwordHashed
	v.Salt = salt
	return DB.Save(v).Error
}

func UpdateUserById(user *User) (err error) {
	v := &User{Id: user.Id}
	if err = DB.Where("id = ?", v.Id).First(&v).Error; err != nil {
		return
	}
	v.Name = user.Name
	v.Email = user.Email
	v.Display = user.Display
	v.Comment = user.Comment

	return DB.Save(v).Error
}

func DeleteUser(id int64) (err error) {
	return DB.Delete(&User{}, id).Error
}
