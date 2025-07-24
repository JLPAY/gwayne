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

	// Event Property
	ReasonProperty                  PropertyName = "reason"
	TypeProperty                    PropertyName = "type"
	MessageProperty                 PropertyName = "message"
	CountProperty                   PropertyName = "count"
	FirstTimestampProperty          PropertyName = "firstTimestamp"
	LastTimestampProperty           PropertyName = "lastTimestamp"
	SourceComponentProperty         PropertyName = "sourceComponent"
	SourceHostProperty              PropertyName = "sourceHost"
	InvolvedObjectKindProperty      PropertyName = "involvedObjectKind"
	InvolvedObjectNameProperty      PropertyName = "involvedObjectName"
	InvolvedObjectNamespaceProperty PropertyName = "involvedObjectNamespace"
)
