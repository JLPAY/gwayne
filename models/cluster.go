package models

import "time"

type ClusterStatus int32

const (
	ClusterStatusNormal      ClusterStatus = 0
	ClusterStatusMaintaining ClusterStatus = 1

	TableNameCluster = "cluster"
)

type Cluster struct {
	ID          int64         `gorm:"primary_key;auto_increment" json:"id,omitempty"`
	Name        string        `gorm:"unique;index;size:128" json:"name,omitempty"`
	DisplayName string        `gorm:"size:512;column:displayname;null" json:"displayname,omitempty"` // 展示名
	MetaData    string        `gorm:"column:meta_data;type:text;null" json:"metaData,omitempty"`
	Master      string        `gorm:"column:master;size:128" json:"master,omitempty"`
	KubeConfig  string        `gorm:"column:kube_config;type:text;null" json:"kubeConfig,omitempty"`
	Description string        `gorm:"column:description;size:512;null" json:"description,omitempty"`
	CreateTime  *time.Time    `gorm:"autoCreateTime" json:"createTime,omitempty"` // 创建时间
	UpdateTime  *time.Time    `gorm:"autoUpdateTime" json:"updateTime,omitempty"` // 更新时间
	User        string        `gorm:"column:user;size:128" json:"user,omitempty"`
	Deleted     bool          `gorm:"default:false" json:"deleted,omitempty"`
	Status      ClusterStatus `gorm:"default:0" json:"status"`
	//MetaDataObj ClusterMetaData `gorm:"-" json:"-"` // GORM 不会处理此字段
}

// 使用 GORM 自动创建表
func (Cluster) TableName() string {
	return TableNameCluster
}

// 根据是否已删除（deleted 参数）来检索 Cluster 名称列表
func GetClusterNames(deleted bool) ([]Cluster, error) {
	var clusters []Cluster
	err := DB.Where("deleted = ?", deleted).Select("id, name").Find(&clusters).Error
	return clusters, err
}

// 获取所有正常状态的集群
func GetAllNormalClusters() ([]Cluster, error) {
	var clusters []Cluster
	err := DB.Where("status = ? AND deleted = ?", ClusterStatusNormal, false).Find(&clusters).Error
	if err != nil {
		return nil, err
	}
	return clusters, nil
}

func AddCluster(cluster *Cluster) (int64, error) {
	err := DB.Create(cluster).Error
	if err != nil {
		return 0, err
	}
	return cluster.ID, nil
}

func UpdateClusterByName(cluster *Cluster) error {
	var existingCluster Cluster
	err := DB.Where("name = ?", cluster.Name).First(&existingCluster).Error
	if err != nil {
		return err
	}
	cluster.UpdateTime = &time.Time{} // 重置更新时间
	return DB.Save(cluster).Error
}

func DeleteClusterByName(name string, logical bool) error {
	var cluster Cluster
	err := DB.Where("name = ?", name).First(&cluster).Error
	if err != nil {
		return err
	}

	// 软删除
	if logical {
		cluster.Deleted = true
		return DB.Save(&cluster).Error
	}

	return DB.Delete(&cluster).Error
}

func GetClusterByName(name string) (*Cluster, error) {
	var cluster Cluster
	err := DB.Where("name = ?", name).First(&cluster).Error
	if err != nil {
		return nil, err
	}
	return &cluster, nil
}

func GetClusterById(id int64) (*Cluster, error) {
	var cluster Cluster
	err := DB.Where("id = ?", id).First(&cluster).Error
	if err != nil {
		return nil, err
	}
	return &cluster, nil
}
