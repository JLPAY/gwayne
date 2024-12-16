package persistentvolume

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ListPersistentVolume 列出持久卷
func ListPersistentVolume(cli *kubernetes.Clientset, listOptions metav1.ListOptions) ([]corev1.PersistentVolume, error) {
	pvList, err := cli.CoreV1().PersistentVolumes().List(context.TODO(), listOptions)
	if err != nil {
		return nil, err
	}
	return pvList.Items, nil
}

func CreatePersistentVolume(cli *kubernetes.Clientset, pv *corev1.PersistentVolume) (*corev1.PersistentVolume, error) {
	pvCreated, err := cli.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return pvCreated, nil
}

func UpdatePersistentVolume(cli *kubernetes.Clientset, pv *corev1.PersistentVolume) (*corev1.PersistentVolume, error) {
	pvCreated, err := cli.CoreV1().PersistentVolumes().Update(context.TODO(), pv, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return pvCreated, nil
}

func DeletePersistentVolume(cli *kubernetes.Clientset, name string) error {
	return cli.CoreV1().PersistentVolumes().Delete(context.TODO(), name, metav1.DeleteOptions{})
}

func GetPersistentVolumeByName(cli *kubernetes.Clientset, name string) (*corev1.PersistentVolume, error) {
	return cli.CoreV1().
		PersistentVolumes().
		Get(context.TODO(), name, metav1.GetOptions{})
}
