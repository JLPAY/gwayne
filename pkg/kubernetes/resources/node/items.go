package node

// InspectItemType 巡检项目类型
type InspectItemType string

const (
	ItemMemory        InspectItemType = "memory"          // 内存使用率（kubelet workingSet）
	ItemDisk          InspectItemType = "disk"            // 磁盘使用率（node-exporter 全分区）
	ItemCPU           InspectItemType = "cpu"             // CPU 使用率（node-exporter）
	ItemLoad          InspectItemType = "load"            // 系统负载（node_load1 / CPU 核数）
	ItemPressure      InspectItemType = "pressure"        // 节点压力条件（Memory/Disk/PID Pressure）
	ItemFD            InspectItemType = "fd"              // 文件描述符使用率（node_filefd）
	ItemPod           InspectItemType = "pod"             // Pod 数量 / 可分配上限
	ItemCertAPIServer InspectItemType = "cert_apiserver"  // API Server serving 证书过期（集群级）
)

// ItemMeta 巡检项目录（代码内注册，作为默认值来源）
type ItemMeta struct {
	Type             InspectItemType
	Name             string  // i18n key
	Unit             string  // "%" 或 "天"
	DefaultWarning   float64 // 告警阈值（含义随项目而定：使用率为 %，证书为剩余天数）
	DefaultCritical  float64
	EnabledByDefault bool
	Source           string // kubelet / prometheus / node-status / k8s-api / tls
	ClusterScoped    bool   // true 表示集群级项目（如 API Server 证书），不挂在单节点上
	// Invert 表示阈值语义反转：值越小越危险（如证书剩余天数）。默认 false（值越大越危险）。
	Invert bool
}

// ItemRegistry 所有可配置巡检项（顺序即前端展示顺序）
var ItemRegistry = []ItemMeta{
	{Type: ItemMemory, Name: "MENU.NODE_INSPECT.MEMORY", Unit: "%", DefaultWarning: 80, DefaultCritical: 90, EnabledByDefault: true, Source: "kubelet"},
	{Type: ItemDisk, Name: "MENU.NODE_INSPECT.DISK", Unit: "%", DefaultWarning: 80, DefaultCritical: 90, EnabledByDefault: true, Source: "prometheus"},
	{Type: ItemCPU, Name: "MENU.NODE_INSPECT.CPU", Unit: "%", DefaultWarning: 80, DefaultCritical: 90, EnabledByDefault: true, Source: "prometheus"},
	{Type: ItemLoad, Name: "MENU.NODE_INSPECT.LOAD", Unit: "%", DefaultWarning: 70, DefaultCritical: 90, EnabledByDefault: true, Source: "prometheus"},
	{Type: ItemPressure, Name: "MENU.NODE_INSPECT.PRESSURE", Unit: "", DefaultWarning: 0, DefaultCritical: 0, EnabledByDefault: true, Source: "node-status"},
	{Type: ItemFD, Name: "MENU.NODE_INSPECT.FD", Unit: "%", DefaultWarning: 80, DefaultCritical: 90, EnabledByDefault: true, Source: "prometheus"},
	{Type: ItemPod, Name: "MENU.NODE_INSPECT.POD", Unit: "%", DefaultWarning: 80, DefaultCritical: 90, EnabledByDefault: true, Source: "k8s-api"},
	{Type: ItemCertAPIServer, Name: "MENU.NODE_INSPECT.CERT_APISERVER", Unit: "天", DefaultWarning: 30, DefaultCritical: 7, EnabledByDefault: true, Source: "tls", ClusterScoped: true, Invert: true},
}

// ItemThreshold 单个巡检项的生效配置（已与默认值合并）
type ItemThreshold struct {
	Enabled  bool
	Warning  float64
	Critical float64
}

// DefaultItemThresholds 返回全部巡检项的默认阈值与启用状态
func DefaultItemThresholds() map[InspectItemType]ItemThreshold {
	m := make(map[InspectItemType]ItemThreshold, len(ItemRegistry))
	for _, it := range ItemRegistry {
		m[it.Type] = ItemThreshold{Enabled: it.EnabledByDefault, Warning: it.DefaultWarning, Critical: it.DefaultCritical}
	}
	return m
}

// CheckResult 单个巡检项的检查结果
type CheckResult struct {
	Type    InspectItemType `json:"type"`
	Name    string          `json:"name"`              // i18n key
	Status  string          `json:"status"`            // ok / warning / critical
	Percent float64         `json:"percent,omitempty"` // 用于趋势图/进度条；证书项为剩余天数
	Value   string          `json:"value"`             // 人类可读展示，如 "85.2%" / "还有 23 天"
	Detail  string          `json:"detail,omitempty"`
}
