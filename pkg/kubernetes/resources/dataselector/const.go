package dataselector

type PropertyName string

// 分隔符
const ListFilterExprSep = "__"

const (
	NameProperty              PropertyName = "name"
	CreationTimestampProperty PropertyName = "creationTimestamp"
	NamespaceProperty         PropertyName = "namespace"
	StatusProperty            PropertyName = "status"
	ReferenceUIDProperty      PropertyName = "referenceUID"

	// Pod Property
	PodIPProperty    PropertyName = "podIP"
	NodeNameProperty PropertyName = "nodeName"
)
