package initial

import (
	"flag"
	"fmt"
	"github.com/JLPAY/gwayne/pkg/config"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
)

// InitKlog 初始化日志系统
// 根据配置文件设置日志目录，初始化 klog 标志并配置不同的日志模式。
// 支持 debug 和 release 模式，分别输出日志到终端或文件。
// 默认情况下，日志输出到标准输出。
func InitKlog() {
	logfile := config.Conf.Log.LogPath

	if err := ensureLogDirExists(logfile); err != nil {
		klog.Fatalf("日志目录不存在: %v", err)
	}

	klog.InitFlags(nil)

	/*
		V(0): Info 级别日志，默认日志级别。
		V(1): Verbose 级别日志。比 V(0) 更详细，通常用于输出系统的某些操作步骤和信息。这种日志适合调试和了解程序运行的状态，但不是非常详细的调试信息。
		V(2) 级别的日志通常用于更详细的调试信息，适合开发人员在开发或调试时查看程序的内部状态。它通常包含更多的系统活动和函数调用信息。
		V(3): Trace 级别日志。
		V(4) 到 V(9): 更详细的调试信息
	*/

	switch config.Conf.App.RunMode {
	case "debug":
		// 在 debug 模式下，日志既输出到终端，也输出到文件
		// `logtostderr` 设为 false，日志输出到文件
		// `alsologtostderr` 设为 true，日志也会输出到标准错误流（终端）
		// 设置最大日志文件大小为 10MB
		// 设置日志文件路径
		flag.Set("logtostderr", "false")
		flag.Set("alsologtostderr", "true")
		flag.Set("log_file_max_size", "10000")
		flag.Set("log_file", config.Conf.Log.LogPath)

	case "release":
		// 在 release 模式下，日志只输出到文件
		// 设置最大日志文件大小为 10MB
		flag.Set("logtostderr", "false")
		flag.Set("log_file_max_size", "10000")
		flag.Set("log_file", config.Conf.Log.LogPath)
	default:
		klog.SetOutput(os.Stdout)
	}

	flag.Parse()
	klog.V(2).Info("日志初始化完成。")
}

func ensureLogDirExists(logFile string) error {
	dir := filepath.Dir(logFile)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建日志目录出错 %s: %v", dir, err)
		}
	}
	return nil
}
