package client

import (
	"testing"
	"time"

	"k8s.io/klog/v2"
)

// TestLazyClientBasic 测试懒加载客户端基本功能
func TestLazyClientBasic(t *testing.T) {
	// 初始化懒加载管理器
	manager := NewLazyClientManager()
	if manager == nil {
		t.Fatal("Failed to create lazy client manager")
	}

	// 测试获取不存在的集群
	_, err := manager.GetCluster("non-existent-cluster")
	if err == nil {
		t.Error("Expected error for non-existent cluster, but got nil")
	}

	// 测试统计信息
	stats := GetLazyStats()
	if stats == nil {
		t.Error("Expected stats, but got nil")
	}

	// 测试监控器
	memoryMonitor := GetMemoryMonitor()
	if memoryMonitor == nil {
		t.Error("Expected memory monitor, but got nil")
	}

	// 测试性能监控器
	perfMonitor := GetPerformanceMonitor()
	if perfMonitor == nil {
		t.Error("Expected performance monitor, but got nil")
	}

	// 测试配置优化器
	configOptimizer := GetConfigOptimizer()
	if configOptimizer == nil {
		t.Error("Expected config optimizer, but got nil")
	}

	t.Log("All basic tests passed")
}

// TestLazyClientManager 测试懒加载客户端管理器
func TestLazyClientManager(t *testing.T) {
	manager := NewLazyClientManager()

	// 测试清理功能
	manager.cleanup()

	// 测试停止功能
	manager.Stop()

	t.Log("Manager tests passed")
}

// TestCacheFactory 测试缓存工厂
func TestCacheFactory(t *testing.T) {
	// 创建一个模拟的CacheFactory
	cacheFactory := &CacheFactory{
		stopChan: make(chan struct{}),
	}

	// 测试Close方法
	cacheFactory.Close()

	t.Log("CacheFactory tests passed")
}

// TestConfigOptimizer 测试配置优化器
func TestConfigOptimizer(t *testing.T) {
	optimizer := NewConfigOptimizer()

	// 测试获取不存在的配置
	_, err := optimizer.GetClusterConfig("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent config, but got nil")
	}

	// 测试统计信息
	stats := optimizer.GetStats()
	if stats == nil {
		t.Error("Expected stats, but got nil")
	}

	// 测试清理功能
	optimizer.CleanupOldConfigs(1 * time.Hour)

	t.Log("ConfigOptimizer tests passed")
}

// TestMemoryMonitor 测试内存监控器
func TestMemoryMonitor(t *testing.T) {
	monitor := NewMemoryMonitor()

	// 测试收集统计
	stats := monitor.CollectStats()
	if stats == nil {
		t.Error("Expected stats, but got nil")
	}

	// 测试获取内存使用量
	memoryMB := monitor.GetMemoryUsageMB()
	if memoryMB < 0 {
		t.Error("Expected positive memory usage")
	}

	// 测试获取系统内存
	sysMemoryMB := monitor.GetSystemMemoryMB()
	if sysMemoryMB < 0 {
		t.Error("Expected positive system memory")
	}

	t.Log("MemoryMonitor tests passed")
}

// TestPerformanceMonitor 测试性能监控器
func TestPerformanceMonitor(t *testing.T) {
	monitor := NewPerformanceMonitor()

	// 测试记录请求
	monitor.RecordRequest(100*time.Millisecond, false)
	monitor.RecordRequest(200*time.Millisecond, true)

	// 测试获取统计
	stats := monitor.GetStats()
	if stats == nil {
		t.Error("Expected stats, but got nil")
	}

	// 验证统计数据
	if stats["total_requests"] != int64(2) {
		t.Errorf("Expected 2 total requests, got %v", stats["total_requests"])
	}

	if stats["error_count"] != int64(1) {
		t.Errorf("Expected 1 error, got %v", stats["error_count"])
	}

	t.Log("PerformanceMonitor tests passed")
}

// BenchmarkLazyClient 性能基准测试
func BenchmarkLazyClient(b *testing.B) {
	manager := NewLazyClientManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 模拟获取集群（这里会失败，因为没有真实的集群）
		_, _ = manager.GetCluster("test-cluster")
	}
}

// ExampleUsage 使用示例
func ExampleUsage() {
	// 获取懒加载客户端
	lazyClient := GetLazyClient()

	// 获取集群客户端（这里会失败，因为没有真实的集群）
	_, err := lazyClient.Client("test-cluster")
	if err != nil {
		klog.Infof("Expected error for test cluster: %v", err)
	}

	// 获取统计信息
	stats := GetComprehensiveStats()
	klog.Infof("System stats: %+v", stats)
}
