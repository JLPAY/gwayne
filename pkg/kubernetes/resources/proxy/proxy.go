package proxy

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/JLPAY/gwayne/models/response"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"github.com/JLPAY/gwayne/pkg/kubernetes/client/api"
	"github.com/JLPAY/gwayne/pkg/kubernetes/resources/dataselector"
	"github.com/JLPAY/gwayne/pkg/pagequery"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func GetPage(kubeClient client.ResourceHandler, kind string, namespace string, q *pagequery.QueryParam) (*pagequery.Page, error) {
	objs, err := kubeClient.List(kind, namespace, q.LabelSelector)
	if err != nil {
		return nil, err
	}

	commonObjs := make([]dataselector.DataCell, 0)
	for _, obj := range objs {
		objCell, err := getRealObjCellByKind(kind, obj) // 转换为 DataCell 类型
		if err != nil {
			return nil, err
		}
		commonObjs = append(commonObjs, objCell)
	}

	sort.Slice(commonObjs, func(i, j int) bool {
		// 使用 NameProperty 对资源对象进行比较，排序方式是降序
		return commonObjs[j].GetProperty(dataselector.NameProperty).Compare(commonObjs[i].GetProperty(dataselector.NameProperty)) == 1
	})

	// 分页，返回一个 common.Page 类型的分页数据
	return dataselector.DataSelectPage(commonObjs, q), nil
}

// 根据资源名称（name）和对象（object）返回一个 DataCell 类型的值
func getRealObjCellByKind(name api.ResourceName, object runtime.Object) (dataselector.DataCell, error) {
	// 根据资源的种类（name）来决定如何处理资源对象
	switch name {
	case api.ResourceNamePod:
		// 如果资源类型是 Pod，将其转换为 PodCell 类型
		obj, ok := object.(*corev1.Pod)
		if !ok {
			// 如果 object 不是 *v1.Pod 类型，返回错误
			return nil, fmt.Errorf("expected *v1.Pod, but got %T", object)
		}
		return PodCell(*obj), nil

	case api.ResourceNameEvent:
		// 如果资源类型是 Event，将其转换为 EventCell 类型
		obj, ok := object.(*corev1.Event)
		if !ok {
			// 如果 object 不是 *v1.Event 类型，返回错误
			return nil, fmt.Errorf("expected *v1.Event, but got %T", object)
		}
		return EventCell(*obj), nil

	default:
		// 默认处理其他资源类型
		// 将对象序列化为 JSON 字节数组
		objByte, err := json.Marshal(object)
		if err != nil {
			return nil, err
		}

		// 创建一个通用的 ObjectCell 结构体，并将 JSON 反序列化到该结构体中
		var commonObj ObjectCell
		err = json.Unmarshal(objByte, &commonObj)
		if err != nil {
			// 如果反序列化失败，返回错误
			return nil, fmt.Errorf("failed to unmarshal to ObjectCell: %v", err)
		}
		return commonObj, nil
	}
}

func GetNames(kubeClient client.ResourceHandler, kind string, namespace string) ([]response.NamesObject, error) {
	objs, err := kubeClient.List(kind, namespace, "")
	if err != nil {
		return nil, err
	}

	commonObjs := make([]response.NamesObject, 0)
	for _, obj := range objs {
		objByte, err := json.Marshal(obj)
		if err != nil {
			return nil, err
		}
		var commonObj ObjectCell
		err = json.Unmarshal(objByte, &commonObj)
		if err != nil {
			return nil, err
		}
		commonObjs = append(commonObjs, response.NamesObject{
			Name: commonObj.Name,
		})
	}

	sort.Slice(commonObjs, func(i, j int) bool {
		return commonObjs[i].Name < commonObjs[j].Name
	})

	return commonObjs, nil
}
