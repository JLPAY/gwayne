package initial

import (
	"github.com/JLPAY/gwayne/pkg/kubernetes/client"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"
)

func InitClient() {
	// 定期更新client, 5s执行一次 client.BuildApiserverClient
	go wait.Forever(client.BuildApiserverClient, 5*time.Second)
}
