package models

import (
	"fmt"
	"time"
)

type GroupType int

const (
	AppGroupType GroupType = iota
	NamespaceGroupType

	TableNameGroup = "group"
)

const (
	GroupAdmin     = "管理员"
	GroupViewer    = "访客"
	GroupDeveloper = "项目开发"
)

type Group struct {
	Id      int64     `gorm:"primaryKey;autoIncrement" json:"id,omitempty"`
	Name    string    `gorm:"size:200;index" json:"name,omitempty"`
	Comment string    `gorm:"type:text" json:"comment,omitempty"`
	Type    GroupType `gorm:"type:integer" json:"type,omitempty"`

	CreateTime *time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"`
	UpdateTime *time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"`

	// 用于权限的关联查询
	Permissions []*Permission `gorm:"many2many:group_permissions;" json:"permissions,omitempty"`
}

func (g *Group) String() string {
	return fmt.Sprintf("[%d]%s", g.Id, g.Name)
}

func (g *Group) TableName() string {
	return TableNameGroup
}

// 向数据库中添加新用户组，并在成功时返回最后插入的 ID
func AddGroup(group *Group) (int64, error) {
	if err := DB.Create(group).Error; err != nil {
		return 0, err
	}
	return group.Id, nil
}
