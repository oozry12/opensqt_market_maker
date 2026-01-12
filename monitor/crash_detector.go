package monitor

import (
	"context"
	"opensqt/config"
	"opensqt/exchange"
	"opensqt/logger"
	"strings"
	"sync"
	"time"
)

// CrashLevel 暴跌级别
type CrashLevel int

const (
	CrashNone     CrashLevel = iota // 无暴跌
	CrashMild                       // 轻度暴跌
	CrashSevere                     // 严重暴跌
)

// String 返回暴跌级别描述
func (c CrashLevel) String() string {
	switch c {
	case CrashNone:
		return "无暴跌"
	case CrashMild:
		return "轻度暴跌"
	case CrashSevere:
		return "严重暴跌"
	default:
		return "未知"
	}
}

// CrashConfig 暴跌检测配置
type CrashConfig struct {
	Enabled         bool
	MAWindow        int
	LongMAWindow    int
	MinUptrendCandles int
	MildCrashRate   float64
	SevereCrashRate float64
	KlineInterval   string
}

// CrashDetector 暴跌检测器
// 用于识别单边上涨趋势中的暴跌行情，触发做空
type CrashDetector struct {
	cfg      *config.Config
	exchange exchange.IExchange
	symbol   string

	// K线数据缓存
	candles []*exchange.Candle
	mu      sync.RWMutex

	// 检测结果
	currentLevel      CrashLevel
	ma20              float64
	ma60              float64
	uptrendCandles       int     // 连续上涨K线数
	crashRate         float64 // 暴跌幅度
	lastDetectionTime time.Time

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCrashDetector 创建暴跌检测器
func NewCrashDetector(cfg *config.Config, ex exchange.IExchange, symbol string) *CrashDetector {
	return &CrashDetector{
		cfg:          cfg,
		exchange:     ex,
		symbol:       symbol,
		candles:      make([]*exchange.Candle, 0, 100),
		currentLevel: CrashNone,
	}
}

// Start 启动检测器
func (d *CrashDetector) Start(ctx context.Context) error {
	d.ctx, d.cancel = context.WithCancel(ctx)

	if err := d.loadHistoricalData(); err != nil {
		logger.Warn("⚠️ [暴跌检测] 加载历史数据失败: %v", err)
	}

	d.wg.Add(1)
	go d.subscribeKlineStream()

	logger.Info("✅ [暴跌检测] 已启动")
	return nil
}

// Stop 停止检测器
func (d *CrashDetector) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
	logger.Info("✅ [暴跌检测] 已停止")
}

// GetCrashLevel 获取当前暴跌级别
func (d *CrashDetector) GetCrashLevel() CrashLevel {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentLevel
}

// ShouldOpenShort 是否应该开空仓
// 新逻辑：只要检测到暴跌即可，不再要求单边上涨趋势
func (d *CrashDetector) ShouldOpenShort() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cfg := d.getConfigLocked()

	if !cfg.Enabled {
		return false
	}

	// 只要检测到暴跌（轻度或严重）即可开空仓
	return d.currentLevel != CrashNone
}

// GetCrashRate 获取暴跌幅度
func (d *CrashDetector) GetCrashRate() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.crashRate
}

// GetUptrendCandles 获取连续上涨K线数
func (d *CrashDetector) GetUptrendCandles() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.uptrendCandles
}

// IsEnabled 检查是否启用
func (d *CrashDetector) IsEnabled() bool {
	return d.cfg.Trading.CrashDetection.Enabled
}

// getConfig 获取配置
func (d *CrashDetector) getConfig() CrashConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.getConfigLocked()
}

// getConfigLocked 获取配置（内部方法，需已持有锁）
func (d *CrashDetector) getConfigLocked() CrashConfig {
	cfg := d.cfg.Trading.CrashDetection

	result := CrashConfig{
		Enabled:          cfg.Enabled,
		MAWindow:         cfg.MAWindow,
		LongMAWindow:     cfg.LongMAWindow,
		MinUptrendCandles: cfg.MinUptrendCandles,
		MildCrashRate:    cfg.MildCrashRate,
		SevereCrashRate:  cfg.SevereCrashRate,
		KlineInterval:    cfg.KlineInterval,
	}

	if result.MAWindow <= 0 {
		result.MAWindow = 20
	}
	if result.LongMAWindow <= 0 {
		result.LongMAWindow = 60
	}
	if result.MinUptrendCandles <= 0 {
		result.MinUptrendCandles = 5
	}
	if result.MildCrashRate <= 0 {
		result.MildCrashRate = 0.05
	}
	if result.SevereCrashRate <= 0 {
		result.SevereCrashRate = 0.10
	}
	if result.KlineInterval == "" {
		result.KlineInterval = "1h"
	}

	return result
}

// loadHistoricalData 加载历史K线数据
func (d *CrashDetector) loadHistoricalData() error {
	cfg := d.getConfig()
	limit := cfg.LongMAWindow + cfg.MinUptrendCandles + 10

	candles, err := d.exchange.GetHistoricalKlines(d.ctx, d.symbol, cfg.KlineInterval, limit)
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.candles = candles
	d.mu.Unlock()

	d.detect()

	logger.Info("✅ [暴跌检测] 已加载 %d 根历史K线", len(candles))
	return nil
}

// subscribeKlineStream 订阅K线流
func (d *CrashDetector) subscribeKlineStream() {
	defer d.wg.Done()

	cfg := d.getConfig()

	err := d.exchange.StartKlineStream(d.ctx, []string{d.symbol}, cfg.KlineInterval, func(candle *exchange.Candle) {
		if candle == nil || candle.Symbol != d.symbol {
			return
		}
		d.onCandleUpdate(candle)
	})

	if err != nil {
		logger.Warn("⚠️ [暴跌检测] 订阅K线流失败: %v", err)
		// 如果K线流已在运行，尝试注册回调
		if strings.Contains(err.Error(), "K线流已在运行") || strings.Contains(err.Error(), "K线流未启动") {
			logger.Info("🔄 [暴跌检测] K线流已在运行，尝试注册回调...")
			err = d.exchange.RegisterKlineCallback("CrashDetector", func(candle interface{}) {
				if candle == nil {
					return
				}
				c, ok := candle.(*exchange.Candle)
				if !ok || c.Symbol != d.symbol {
					return
				}
				d.onCandleUpdate(c)
			})
			if err != nil {
				logger.Error("❌ [暴跌检测] 注册回调失败: %v", err)
				d.fallbackPolling()
			} else {
				logger.Info("✅ [暴跌检测] 已注册K线回调")
			}
		} else {
			d.fallbackPolling()
		}
	}
}

// fallbackPolling 降级轮询模式
func (d *CrashDetector) fallbackPolling() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			if err := d.loadHistoricalData(); err != nil {
				logger.Warn("⚠️ [暴跌检测] 轮询更新失败: %v", err)
			}
		}
	}
}

// onCandleUpdate K线更新回调
func (d *CrashDetector) onCandleUpdate(candle *exchange.Candle) {
	d.mu.Lock()

	cfg := d.getConfigLocked()
	maxCandles := cfg.LongMAWindow + cfg.MinUptrendCandles + 10

	if candle.IsClosed {
		d.candles = append(d.candles, candle)
		if len(d.candles) > maxCandles {
			d.candles = d.candles[len(d.candles)-maxCandles:]
		}
	} else {
		if len(d.candles) > 0 && !d.candles[len(d.candles)-1].IsClosed {
			d.candles[len(d.candles)-1] = candle
		} else {
			d.candles = append(d.candles, candle)
		}
	}

	d.mu.Unlock()

	if candle.IsClosed {
		d.detect()
	}
}

// detect 执行暴跌检测
// 新逻辑：检测任意2根K线的平均跌幅是否大于阈值
func (d *CrashDetector) detect() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cfg := d.getConfigLocked()

	// 只保留已关闭的K线
	closedCandles := make([]*exchange.Candle, 0)
	for _, c := range d.candles {
		if c.IsClosed {
			closedCandles = append(closedCandles, c)
		}
	}

	// 至少需要2根K线才能计算跌幅
	if len(closedCandles) < 2 {
		return
	}

	// 计算均线（用于显示，不影响触发逻辑）
	if len(closedCandles) >= cfg.MAWindow {
		var sum20 float64
		startIdx20 := len(closedCandles) - cfg.MAWindow
		for i := startIdx20; i < len(closedCandles); i++ {
			sum20 += closedCandles[i].Close
		}
		d.ma20 = sum20 / float64(cfg.MAWindow)
	}

	if len(closedCandles) >= cfg.LongMAWindow {
		var sum60 float64
		startIdx60 := len(closedCandles) - cfg.LongMAWindow
		for i := startIdx60; i < len(closedCandles); i++ {
			sum60 += closedCandles[i].Close
		}
		d.ma60 = sum60 / float64(cfg.LongMAWindow)
	}

	currentPrice := closedCandles[len(closedCandles)-1].Close

	// 🔥 新逻辑：检测任意2根K线的平均跌幅
	// 遍历最近的N根K线，找出任意2根K线的最大平均跌幅
	maxAvgDropRate := 0.0
	lookbackWindow := 10 // 检查最近10根K线
	if len(closedCandles) < lookbackWindow {
		lookbackWindow = len(closedCandles)
	}

	// 遍历所有可能的2根K线组合
	for i := len(closedCandles) - lookbackWindow; i < len(closedCandles)-1; i++ {
		for j := i + 1; j < len(closedCandles); j++ {
			// 计算这2根K线的平均跌幅
			// 跌幅 = (开盘价 - 收盘价) / 开盘价
			drop1 := (closedCandles[i].Open - closedCandles[i].Close) / closedCandles[i].Open
			drop2 := (closedCandles[j].Open - closedCandles[j].Close) / closedCandles[j].Open
			
			// 只考虑下跌的K线（收盘价 < 开盘价）
			if drop1 > 0 && drop2 > 0 {
				avgDropRate := (drop1 + drop2) / 2.0
				if avgDropRate > maxAvgDropRate {
					maxAvgDropRate = avgDropRate
				}
			}
		}
	}

	d.crashRate = maxAvgDropRate

	// 统计连续上涨K线数（用于显示，不影响触发逻辑）
	d.uptrendCandles = 0
	for i := len(closedCandles) - 1; i >= 0 && d.uptrendCandles < cfg.MinUptrendCandles+5; i-- {
		if closedCandles[i].Close > closedCandles[i].Open {
			d.uptrendCandles++
		} else {
			break
		}
	}

	oldLevel := d.currentLevel

	// 🔥 简化触发条件：只要平均跌幅达到阈值即可
	// 不再要求单边上涨趋势
	if d.crashRate >= cfg.SevereCrashRate {
		d.currentLevel = CrashSevere
	} else if d.crashRate >= cfg.MildCrashRate {
		d.currentLevel = CrashMild
	} else {
		d.currentLevel = CrashNone
	}

	d.lastDetectionTime = time.Now()

	// 调试日志
	logger.Debug("🔍 [暴跌检测] 价格:%.4f, MA20:%.4f, MA60:%.4f, 最大平均跌幅:%.2f%%, 级别:%s",
		currentPrice, d.ma20, d.ma60, d.crashRate*100, d.currentLevel.String())

	// 状态变化时输出警告
	if d.currentLevel != oldLevel {
		switch d.currentLevel {
		case CrashSevere:
			logger.Warn("🔻🔻🔻 [暴跌检测] 严重暴跌！检测到2根K线平均跌幅 %.2f%%",
				d.crashRate*100)
		case CrashMild:
			logger.Warn("🔻🔻 [暴跌检测] 轻度暴跌，检测到2根K线平均跌幅 %.2f%%",
				d.crashRate*100)
		case CrashNone:
			logger.Info("✅ [暴跌检测] 无暴跌，最大平均跌幅 %.2f%%", d.crashRate*100)
		}
	}
}

// GetStatus 获取检测状态
func (d *CrashDetector) GetStatus() (level CrashLevel, ma20 float64, ma60 float64, uptrendCandles int, crashRate float64) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	level = d.currentLevel
	ma20 = d.ma20
	ma60 = d.ma60
	uptrendCandles = d.uptrendCandles
	crashRate = d.crashRate

	return
}
