package client

import (
	"runtime"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// MemoryMonitor 内存监控器
type MemoryMonitor struct {
	mu              sync.RWMutex
	lastMemoryStats *MemoryStats
	statsHistory    []*MemoryStats
	maxHistorySize  int
}

// MemoryStats 内存统计信息
type MemoryStats struct {
	Timestamp     time.Time
	Alloc        uint64
	TotalAlloc   uint64
	Sys          uint64
	NumGC        uint32
	NumGoroutine int
	ClusterCount  int
	InitializedCount int
}

// NewMemoryMonitor 创建内存监控器
func NewMemoryMonitor() *MemoryMonitor {
	return &MemoryMonitor{
		statsHistory:   make([]*MemoryStats, 0),
		maxHistorySize: 100, // 保留最近100个统计点
	}
}

// CollectStats 收集内存统计
func (mm *MemoryMonitor) CollectStats() *MemoryStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := &MemoryStats{
		Timestamp:     time.Now(),
		Alloc:        m.Alloc,
		TotalAlloc:   m.TotalAlloc,
		Sys:          m.Sys,
		NumGC:        m.NumGC,
		NumGoroutine: runtime.NumGoroutine(),
	}

	// 获取集群统计
	if lazyManager != nil {
		clusterCount := 0
		initializedCount := 0
		
		lazyManager.managers.Range(func(key, value interface{}) bool {
			clusterCount++
			lazyClusterManager := value.(*LazyClusterManager)
			if lazyClusterManager.initialized {
				initializedCount++
			}
			return true
		})
		
		stats.ClusterCount = clusterCount
		stats.InitializedCount = initializedCount
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.lastMemoryStats = stats
	mm.statsHistory = append(mm.statsHistory, stats)

	// 保持历史记录大小
	if len(mm.statsHistory) > mm.maxHistorySize {
		mm.statsHistory = mm.statsHistory[1:]
	}

	return stats
}

// GetLastStats 获取最新的内存统计
func (mm *MemoryMonitor) GetLastStats() *MemoryStats {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.lastMemoryStats
}

// GetStatsHistory 获取统计历史
func (mm *MemoryMonitor) GetStatsHistory() []*MemoryStats {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	
	history := make([]*MemoryStats, len(mm.statsHistory))
	copy(history, mm.statsHistory)
	return history
}

// GetMemoryUsageMB 获取内存使用量（MB）
func (mm *MemoryMonitor) GetMemoryUsageMB() float64 {
	stats := mm.GetLastStats()
	if stats == nil {
		return 0
	}
	return float64(stats.Alloc) / 1024 / 1024
}

// GetSystemMemoryMB 获取系统内存（MB）
func (mm *MemoryMonitor) GetSystemMemoryMB() float64 {
	stats := mm.GetLastStats()
	if stats == nil {
		return 0
	}
	return float64(stats.Sys) / 1024 / 1024
}

// PerformanceMonitor 性能监控器
type PerformanceMonitor struct {
	mu              sync.RWMutex
	requestCount    int64
	errorCount      int64
	avgResponseTime time.Duration
	lastRequestTime time.Time
}

// NewPerformanceMonitor 创建性能监控器
func NewPerformanceMonitor() *PerformanceMonitor {
	return &PerformanceMonitor{}
}

// RecordRequest 记录请求
func (pm *PerformanceMonitor) RecordRequest(duration time.Duration, isError bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.requestCount++
	if isError {
		pm.errorCount++
	}
	
	// 计算平均响应时间
	if pm.avgResponseTime == 0 {
		pm.avgResponseTime = duration
	} else {
		pm.avgResponseTime = (pm.avgResponseTime + duration) / 2
	}
	
	pm.lastRequestTime = time.Now()
}

// GetStats 获取性能统计
func (pm *PerformanceMonitor) GetStats() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_requests"] = pm.requestCount
	stats["error_count"] = pm.errorCount
	stats["success_rate"] = float64(pm.requestCount-pm.errorCount) / float64(pm.requestCount) * 100
	stats["avg_response_time_ms"] = pm.avgResponseTime.Milliseconds()
	stats["last_request_time"] = pm.lastRequestTime

	return stats
}

// 全局监控器实例
var (
	globalMemoryMonitor     *MemoryMonitor
	globalPerformanceMonitor *PerformanceMonitor
	monitorOnce             sync.Once
)

// GetMemoryMonitor 获取全局内存监控器
func GetMemoryMonitor() *MemoryMonitor {
	monitorOnce.Do(func() {
		globalMemoryMonitor = NewMemoryMonitor()
		globalPerformanceMonitor = NewPerformanceMonitor()
		
		// 启动定期监控
		go startPeriodicMonitoring()
	})
	return globalMemoryMonitor
}

// GetPerformanceMonitor 获取全局性能监控器
func GetPerformanceMonitor() *PerformanceMonitor {
	monitorOnce.Do(func() {
		globalMemoryMonitor = NewMemoryMonitor()
		globalPerformanceMonitor = NewPerformanceMonitor()
		
		// 启动定期监控
		go startPeriodicMonitoring()
	})
	return globalPerformanceMonitor
}

// startPeriodicMonitoring 启动定期监控
func startPeriodicMonitoring() {
	ticker := time.NewTicker(30 * time.Second) // 每30秒收集一次统计
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 收集内存统计
			if globalMemoryMonitor != nil {
				stats := globalMemoryMonitor.CollectStats()
				
				// 记录内存使用情况
				memoryMB := float64(stats.Alloc) / 1024 / 1024
				sysMemoryMB := float64(stats.Sys) / 1024 / 1024
				
				klog.V(2).Infof("Memory Usage: %.2f MB, System: %.2f MB, Clusters: %d/%d, Goroutines: %d",
					memoryMB, sysMemoryMB, stats.InitializedCount, stats.ClusterCount, stats.NumGoroutine)
				
				// 内存使用警告
				if memoryMB > 500 { // 500MB警告阈值
					klog.Warningf("High memory usage detected: %.2f MB", memoryMB)
				}
			}
		}
	}
}

// GetComprehensiveStats 获取综合统计信息
func GetComprehensiveStats() map[string]interface{} {
	stats := make(map[string]interface{})
	
	// 内存统计
	if globalMemoryMonitor != nil {
		memoryStats := globalMemoryMonitor.GetLastStats()
		if memoryStats != nil {
			stats["memory_usage_mb"] = float64(memoryStats.Alloc) / 1024 / 1024
			stats["system_memory_mb"] = float64(memoryStats.Sys) / 1024 / 1024
			stats["goroutines"] = memoryStats.NumGoroutine
			stats["gc_count"] = memoryStats.NumGC
		}
	}
	
	// 性能统计
	if globalPerformanceMonitor != nil {
		perfStats := globalPerformanceMonitor.GetStats()
		for k, v := range perfStats {
			stats["perf_"+k] = v
		}
	}
	
	// 懒加载统计
	lazyStats := GetLazyStats()
	for k, v := range lazyStats {
		stats["lazy_"+k] = v
	}
	
	// 配置缓存统计
	if globalConfigOptimizer != nil {
		configStats := globalConfigOptimizer.GetStats()
		for k, v := range configStats {
			stats["config_"+k] = v
		}
	}
	
	return stats
} 