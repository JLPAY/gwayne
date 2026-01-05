package models

import "time"

const (
	TableNamePermission = "permission"

	PermissionCreate = "CREATE"
	PermissionUpdate = "UPDATE"
	PermissionRead   = "READ"
	PermissionDelete = "DELETE"

	PermissionTypeApp                   = "APP"
	PermissionTypeAppUser               = "APPUSER"
	PermissionTypeDeployment            = "DEPLOYMENT"
	PermissionTypeSecret                = "SECRET"
	PermissionTypeService               = "SERVICE"
	PermissionTypeConfigMap             = "CONFIGMAP"
	PermissionTypeCronjob               = "CRONJOB"
	PermissionTypePersistentVolumeClaim = "PVC"
	PermissionTypeNamespace             = "NAMESPACE"
	PermissionTypeNamespaceUser         = "NAMESPACEUSER"
	PermissionTypeWebHook               = "WEBHOOK"
	PermissionTypeStatefulset           = "STATEFULSET"
	PermissionTypeDaemonSet             = "DAEMONSET"
	PermissionBill                      = "BILL"
	PermissionTypeAPIKey                = "APIKEY"
	PermissionTypeIngress               = "INGRESS"
	PermissionTypeHPA                   = "HPA"
	PermissionBlank                     = "_"

	// Kubernetes resource permission
	PermissionTypeKubeConfigMap                = "KUBECONFIGMAP"
	PermissionTypeKubeDaemonSet                = "KUBEDAEMONSET"
	PermissionTypeKubeDeployment               = "KUBEDEPLOYMENT"
	PermissionTypeKubeEvent                    = "KUBEEVENT"
	PermissionTypeKubeHorizontalPodAutoscaler  = "KUBEHORIZONTALPODAUTOSCALER"
	PermissionTypeKubeIngress                  = "KUBEINGRESS"
	PermissionTypeKubeJob                      = "KUBEJOB"
	PermissionTypeKubeCronJob                  = "KUBECRONJOB"
	PermissionTypeKubeNamespace                = "KUBENAMESPACE"
	PermissionTypeKubeNode                     = "KUBENODE"
	PermissionTypeKubePersistentVolumeClaim    = "KUBEPERSISTENTVOLUMECLAIM"
	PermissionTypeKubePersistentVolume         = "KUBEPERSISTENTVOLUME"
	PermissionTypeKubePod                      = "KUBEPOD"
	PermissionTypeKubeReplicaSet               = "KUBEREPLICASET"
	PermissionTypeKubeSecret                   = "KUBESECRET"
	PermissionTypeKubeService                  = "KUBESERVICE"
	PermissionTypeKubeStatefulSet              = "KUBESTATEFULSET"
	PermissionTypeKubeEndpoint                 = "KUBEENDPOINTS"
	PermissionTypeKubeStorageClass             = "KUBESTORAGECLASS"
	PermissionTypeKubeRole                     = "KUBEROLE"
	PermissionTypeKubeRoleBinding              = "KUBEROLEBINDING"
	PermissionTypeKubeClusterRole              = "KUBECLUSTERROLE"
	PermissionTypeKubeClusterRoleBinding       = "KUBECLUSTERROLEBINDING"
	PermissionTypeKubeServiceAccount           = "KUBESERVICEACCOUNT"
	PermissionTypeKubeCustomResourceDefinition = "KUBECUSTOMRESOURCEDEFINITION"
)

type Permission struct {
	Id         int64     `gorm:"primaryKey" json:"id,omitempty"`
	Name       string    `gorm:"size:200;index" json:"name,omitempty"`
	Comment    string    `gorm:"type:text" json:"comment,omitempty"`
	CreateTime time.Time `gorm:"autoCreateTime" json:"createTime,omitempty"`
	UpdateTime time.Time `gorm:"autoUpdateTime" json:"updateTime,omitempty"`

	Groups []*Group `gorm:"many2many:permission_groups;" json:"groups,omitempty"` // 假设关联表为 permission_groups
}

func (*Permission) TableName() string {
	return TableNamePermission
}

func AddPermission(permission *Permission) (int64, error) {
	if err := DB.Create(permission).Error; err != nil {
		return 0, err
	}
	return permission.Id, nil
}

func GetPermissionById(id int64) (*Permission, error) {
	var permission Permission
	if err := DB.Find(&permission, id).Error; err != nil {
		return nil, err
	}
	return &permission, nil
}

func UpdatePermissionById(permission *Permission) error {
	var existing Permission
	if err := DB.First(&existing, permission.Id).Error; err != nil {
		return err
	}
	permission.UpdateTime = time.Now() // 设置更新时间
	return DB.Save(permission).Error
}

func DeletePermission(id int64) error {
	var permission Permission
	if err := DB.First(&permission, id).Error; err != nil {
		return err
	}
	return DB.Delete(&permission).Error
}
