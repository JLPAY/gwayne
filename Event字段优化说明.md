# Event字段优化说明

## 问题描述

### 原始问题
1. **缺少Event专用处理逻辑**：后端返回的Event数据中缺少 `reason`、`source`、`message` 等关键字段
2. **Labels字段缺失**：Event的Labels字段没有被正确提取和显示
3. **通用ObjectCell限制**：使用通用的`ObjectCell`处理Event，无法提供Event特有的字段访问

### 具体缺失字段
- `reason`：事件原因
- `source`：事件来源（包含component和host）
- `message`：事件消息
- `type`：事件类型（Normal/Warning）
- `count`：事件计数
- `firstTimestamp`：首次发生时间
- `lastTimestamp`：最后发生时间
- `involvedObject`：相关对象信息
- `labels`：标签信息

## 解决方案

### 1. 创建Event专用Cell类型

创建了`EventCell`类型，专门处理Event资源：

```go
type EventCell corev1.Event

func (cell EventCell) GetProperty(name dataselector.PropertyName) dataselector.ComparableValue {
    switch name {
    case dataselector.ReasonProperty:
        return dataselector.StdComparableString(cell.Reason)
    case dataselector.MessageProperty:
        return dataselector.StdComparableString(cell.Message)
    case dataselector.TypeProperty:
        return dataselector.StdComparableString(cell.Type)
    case dataselector.CountProperty:
        return dataselector.StdComparableInt(int(cell.Count))
    // ... 其他Event特有属性
    }
}
```

### 2. 添加Event相关属性常量

在`dataselector/const.go`中添加了Event相关的属性常量：

```go
const (
    // Event Property
    ReasonProperty                    PropertyName = "reason"
    TypeProperty                      PropertyName = "type"
    MessageProperty                   PropertyName = "message"
    CountProperty                     PropertyName = "count"
    FirstTimestampProperty            PropertyName = "firstTimestamp"
    LastTimestampProperty             PropertyName = "lastTimestamp"
    SourceComponentProperty           PropertyName = "sourceComponent"
    SourceHostProperty                PropertyName = "sourceHost"
    InvolvedObjectKindProperty       PropertyName = "involvedObjectKind"
    InvolvedObjectNameProperty       PropertyName = "involvedObjectName"
    InvolvedObjectNamespaceProperty  PropertyName = "involvedObjectNamespace"
)
```

### 3. 修改资源处理逻辑

在`proxy.go`的`getRealObjCellByKind`函数中添加Event处理：

```go
case api.ResourceNameEvent:
    obj, ok := object.(*corev1.Event)
    if !ok {
        return nil, fmt.Errorf("expected *v1.Event, but got %T", object)
    }
    return EventCell(*obj), nil
```

### 4. 提供完整的Event字段访问

`EventCell`提供了以下方法：

```go
// 基础字段
func (cell EventCell) GetLabels() map[string]string
func (cell EventCell) GetAnnotations() map[string]string

// Event特有字段
func (cell EventCell) GetReason() string
func (cell EventCell) GetMessage() string
func (cell EventCell) GetType() string
func (cell EventCell) GetCount() int32
func (cell EventCell) GetSource() corev1.EventSource
func (cell EventCell) GetInvolvedObject() corev1.ObjectReference
func (cell EventCell) GetFirstTimestamp() *metav1.Time
func (cell EventCell) GetLastTimestamp() *metav1.Time
```

## 优化效果

### 修复前的问题
```json
{
  "name": "event-name",
  "namespace": "default",
  "creationTimestamp": "2024-01-01T00:00:00Z"
  // 缺少 reason, message, source, type, count 等字段
}
```

### 修复后的完整数据
```json
{
  "name": "event-name",
  "namespace": "default",
  "creationTimestamp": "2024-01-01T00:00:00Z",
  "reason": "Scheduled",
  "message": "Successfully assigned pod to node",
  "type": "Normal",
  "count": 1,
  "firstTimestamp": "2024-01-01T00:00:00Z",
  "lastTimestamp": "2024-01-01T00:00:00Z",
  "source": {
    "component": "default-scheduler",
    "host": "node-1"
  },
  "involvedObject": {
    "kind": "Pod",
    "name": "test-pod",
    "namespace": "default",
    "uid": "pod-uid"
  },
  "labels": {
    "app": "test",
    "version": "v1"
  }
}
```

## 测试验证

创建了完整的测试文件`event_test.go`，包含：

1. **属性访问测试**：验证所有Event字段的正确访问
2. **Labels测试**：验证Labels字段的正确提取
3. **Annotations测试**：验证Annotations字段的正确提取
4. **Event特有字段测试**：验证reason、message、type、count等字段

## 使用方式

### 前端调用
```javascript
// 获取Event列表
GET /api/v1/apps/0/_proxy/clusters/UAT/namespaces/default/events?pageNo=1&pageSize=10

// 支持按Event字段过滤
GET /api/v1/apps/0/_proxy/clusters/UAT/namespaces/default/events?filter=reason=Scheduled,type=Normal

// 支持按Event字段排序
GET /api/v1/apps/0/_proxy/clusters/UAT/namespaces/default/events?sortby=-lastTimestamp
```

### 支持的过滤字段
- `reason`：事件原因
- `type`：事件类型
- `message`：事件消息
- `sourceComponent`：来源组件
- `sourceHost`：来源主机
- `involvedObjectKind`：相关对象类型
- `involvedObjectName`：相关对象名称

### 支持的排序字段
- `reason`：按事件原因排序
- `type`：按事件类型排序
- `count`：按事件计数排序
- `firstTimestamp`：按首次发生时间排序
- `lastTimestamp`：按最后发生时间排序

## 性能优化

1. **类型安全**：使用强类型的`EventCell`，避免运行时类型转换错误
2. **内存效率**：直接使用`corev1.Event`，避免额外的序列化/反序列化
3. **扩展性**：为其他资源类型提供了可复用的模式

## 后续优化建议

1. **Event聚合**：考虑按相同reason和involvedObject聚合Event，减少重复显示
2. **Event过滤**：提供更丰富的过滤条件，如按时间范围、事件级别等
3. **Event统计**：提供Event统计信息，如各类型Event数量分布
4. **实时更新**：通过WebSocket提供Event的实时更新 