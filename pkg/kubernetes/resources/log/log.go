package log

import (
	"bytes"
	"context"
	"fmt"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

func GetLogsByPod(cli *kubernetes.Clientset, namespace, podName string, opt *corev1.PodLogOptions) ([]byte, error) {
	logRequest := cli.CoreV1().Pods(namespace).GetLogs(podName, opt)
	stream, err := logRequest.Stream(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for pod %s: %w", podName, err)
	}
	defer stream.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, stream)
	if err != nil {
		return nil, err
	}

	//klog.V(2).Infof("get logs for pod %s: \n%s", podName, buf.String())
	return buf.Bytes(), nil
}
