package main

import (
	"context"
	"fmt"
	"github.com/JLPAY/gwayne/pkg/config"
	"github.com/JLPAY/gwayne/pkg/initial"
	"github.com/JLPAY/gwayne/pkg/rsakey"
	"github.com/JLPAY/gwayne/routers"
	"k8s.io/klog/v2"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {

	cfg := config.GetConfig()

	// 初始化日志
	initial.InitKlog()

	// 初始化DB
	initial.InitDb()

	// 初始化rsa密钥
	rsakey.InitRsaKey()

	// 初始化 K8S Client
	initial.InitClient()

	router := routers.InitRouter()

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.App.HttpPort),
		Handler: router,
		//ReadTimeout:    cfg.App.ReadTimeout,
		//WriteTimeout:   cfg.App.WriteTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			klog.Errorf("Listen: %s\n", err)
			os.Exit(1)
		}
	}()
	klog.Infof("Server started on :%d", cfg.App.HttpPort)

	// 创建一个通道，用于接收终止信号
	quit := make(chan os.Signal, 1)

	// 接收信号（SIGINT, SIGTERM 等）
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞，直到接收到信号
	<-quit
	klog.Info("shutdown Server ...")

	// 创建一个 5 秒的上下文，用于等待当前请求完成
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 优雅关闭服务器（等待 5 秒内处理完成正在进行的请求）
	if err := srv.Shutdown(ctx); err != nil {
		klog.Fatal("server Shutdown:", err)
	}

	klog.Info("server exiting")
	// 确保所有日志都被写入
	defer klog.Flush()
}
