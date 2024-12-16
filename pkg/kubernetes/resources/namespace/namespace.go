package namespace

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CreateNotExitNamespace(cli *kubernetes.Clientset, ns *corev1.Namespace) (*corev1.Namespace, error) {
	// 检查命名空间是否存在
	_, err := cli.CoreV1().Namespaces().Get(context.TODO(), ns.Name, metaV1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// 创建命名空间
			return cli.CoreV1().Namespaces().Create(context.TODO(), ns, metaV1.CreateOptions{})
		}
		return nil, err
	}
	return nil, nil
}
