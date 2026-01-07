package monitor

import (
	"context"
	"opensqt/config"
	"opensqt/exchange"
	"opensqt/logger"
	"sync"
	"time"
)

// DowntrendLevel 下跌趋势级别
type DowntrendLevel int

const (
	DowntrendNone     DowntrendLevel = iota // 无下跌趋势
	DowntrendMild                           // 轻度下跌（均线压制）
	DowntrendSevere                         // 严重阴跌（均线压制+连续收阴）
)

// String 返回趋势级别描述
func (d DowntrendLevel) String() string {
	switch d {
	case DowntrendNone:
		return "正常"
	case DowntrendMild:
		return "轻度下跌"
	case DowntrendSevere:
		return "严重阴跌"
	default:
		return "未知"
	}
}

// DowntrendDetector 阴跌检测器
// 用于识别"钝刀子割肉"的缓慢下跌行情
type DowntrendDetector struct {
	cfg      *config.Config
	exchange exchange.IExchange
	symbol   string

	// K线数据缓存
	candles []*exchange.Candle
	mu      sync.RWMutex

	// 检测结果
	currentLevel      DowntrendLevel
	ma20              float64 // 20周期均线
	consecutiveDowns  int     // 连续下跌K线数
	lastDetectionTime time.Time

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// DowntrendConfig 阴跌检测配置
type DowntrendConfig struct {
	Enabled              bool    // 是否启用
	MAWindow             int     // 均线周期（默认20）
	MildThreshold        float64 // 轻度下跌阈值（默认0.98，即低于均线2%）
	SevereThreshold      float64 // 严重下跌阈值（默认0.985，即低于均线1.5%）
	ConsecutiveDownCount int     // 连续收阴K线数（默认6）
	MildMultiplier       float64 // 轻度下跌买入乘数（默认0.8）
	SevereMultiplier     float64 // 严重阴跌买入乘数（默认0.6）
	SevereWindowRatio    float64 // 严重阴跌时买单窗口比例（默认0.3）
	KlineInterval        string  // K线周期（默认"5m"）
}

// NewDowntrendDetector 创建阴跌检测器
func NewDowntrendDetector(cfg *config.Config, ex exchange.IExchange, symbol string) *DowntrendDetector {
	return &DowntrendDetector{
		cfg:          cfg,
		exchange:     ex,
		symbol:       symbol,
		candles:      make([]*exchange.Candle, 0, 50),
		currentLevel: DowntrendNone,
	}
}

// Start 启动检测器
func (d *DowntrendDetector) Start(ctx context.Context) error {
	d.ctx, d.cancel = context.WithCancel(ctx)

	// 加载历史K线
	if err := d.loadHistoricalData(); err != nil {
		logger.Warn("⚠️ [阴跌检测] 加载历史数据失败: %v", err)
	}

	// 订阅K线流
	d.wg.Add(1)
	go d.subscribeKlineStream()

	logger.Info("✅ [阴跌检测] 已启动 (均线周期: %d, 连续收阴: %d根)",
		d.getConfig().MAWindow, d.getConfig().ConsecutiveDownCount)

	return nil
}

// Stop 停止检测器
func (d *DowntrendDetector) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
	logger.Info("✅ [阴跌检测] 已停止")
}

// GetDowntrendLevel 获取当前下跌趋势级别
func (d *DowntrendDetector) GetDowntrendLevel() DowntrendLevel {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentLevel
}

// GetBuyMultiplier 获取买入数量乘数
// 根据下跌趋势级别返回相应的乘数
func (d *DowntrendDetector) GetBuyMultiplier() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cfg := d.getConfigLocked()

	switch d.currentLevel {
	case DowntrendMild:
		return cfg.MildMultiplier
	case DowntrendSevere:
		return cfg.SevereMultiplier
	default:
		return 1.0
	}
}

// GetWindowRatio 获取买单窗口比例
// 严重阴跌时减少挂单数量
func (d *DowntrendDetector) GetWindowRatio() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.currentLevel == DowntrendSevere {
		return d.getConfigLocked().SevereWindowRatio
	}
	return 1.0
}

// GetMA20 获取当前MA20值
func (d *DowntrendDetector) GetMA20() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ma20
}

// GetConsecutiveDowns 获取连续下跌K线数
func (d *DowntrendDetector) GetConsecutiveDowns() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.consecutiveDowns
}

// IsEnabled 检查是否启用
func (d *DowntrendDetector) IsEnabled() bool {
	return d.cfg.Trading.DowntrendDetection.Enabled
}

// getConfig 获取配置（加锁版本）
func (d *DowntrendDetector) getConfig() DowntrendConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.getConfigLocked()
}

// getConfigLocked 获取配置（内部方法，需已持有锁）
func (d *DowntrendDetector) getConfigLocked() DowntrendConfig {
	cfg := d.cfg.Trading.DowntrendDetection

	// 设置默认值
	result := DowntrendConfig{
		Enabled:              cfg.Enabled,
		MAWindow:             cfg.MAWindow,
		MildThreshold:        cfg.MildThreshold,
		SevereThreshold:      cfg.SevereThreshold,
		ConsecutiveDownCount: cfg.ConsecutiveDownCount,
		MildMultiplier:       cfg.MildMultiplier,
		SevereMultiplier:     cfg.SevereMultiplier,
		SevereWindowRatio:    cfg.SevereWindowRatio,
		KlineInterval:        cfg.KlineInterval,
	}

	if result.MAWindow <= 0 {
		result.MAWindow = 20
	}
	if result.MildThreshold <= 0 {
		result.MildThreshold = 0.98
	}
	if result.SevereThreshold <= 0 {
		result.SevereThreshold = 0.985
	}
	if result.ConsecutiveDownCount <= 0 {
		result.ConsecutiveDownCount = 6
	}
	if result.MildMultiplier <= 0 {
		result.MildMultiplier = 0.8
	}
	if result.SevereMultiplier <= 0 {
		result.SevereMultiplier = 0.6
	}
	if result.SevereWindowRatio <= 0 {
		result.SevereWindowRatio = 0.3
	}
	if result.KlineInterval == "" {
		result.KlineInterval = "5m"
	}

	return result
}

// loadHistoricalData 加载历史K线数据
func (d *DowntrendDetector) loadHistoricalData() error {
	cfg := d.getConfig()
	limit := cfg.MAWindow + cfg.ConsecutiveDownCount + 5

	candles, err := d.exchange.GetHistoricalKlines(d.ctx, d.symbol, cfg.KlineInterval, limit)
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.candles = candles
	d.mu.Unlock()

	// 执行初始检测
	d.detect()

	logger.Info("✅ [阴跌检测] 已加载 %d 根历史K线，MA20: %.4f", len(candles), d.GetMA20())
	return nil
}

// subscribeKlineStream 订阅K线流
func (d *DowntrendDetector) subscribeKlineStream() {
	defer d.wg.Done()

	cfg := d.getConfig()

	err := d.exchange.StartKlineStream(d.ctx, []string{d.symbol}, cfg.KlineInterval, func(candle *exchange.Candle) {
		if candle == nil || candle.Symbol != d.symbol {
			return
		}
		d.onCandleUpdate(candle)
	})

	if err != nil {
		logger.Warn("⚠️ [阴跌检测] 订阅K线流失败: %v，使用轮询模式", err)
		d.fallbackPolling()
	}
}

// fallbackPolling 降级轮询模式
func (d *DowntrendDetector) fallbackPolling() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			if err := d.loadHistoricalData(); err != nil {
				logger.Warn("⚠️ [阴跌检测] 轮询更新失败: %v", err)
			}
		}
	}
}

// onCandleUpdate K线更新回调
func (d *DowntrendDetector) onCandleUpdate(candle *exchange.Candle) {
	d.mu.Lock()

	cfg := d.getConfigLocked()
	maxCandles := cfg.MAWindow + cfg.ConsecutiveDownCount + 5

	if candle.IsClosed {
		// 完结的K线：追加
		d.candles = append(d.candles, candle)
		if len(d.candles) > maxCandles {
			d.candles = d.candles[len(d.candles)-maxCandles:]
		}
	} else {
		// 未完结：更新最后一根
		if len(d.candles) > 0 && !d.candles[len(d.candles)-1].IsClosed {
			d.candles[len(d.candles)-1] = candle
		} else {
			d.candles = append(d.candles, candle)
		}
	}

	d.mu.Unlock()

	// 只在K线完结时执行检测
	if candle.IsClosed {
		d.detect()
	}
}

// detect 执行阴跌检测
func (d *DowntrendDetector) detect() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cfg := d.getConfigLocked()

	// 获取完结的K线
	closedCandles := make([]*exchange.Candle, 0)
	for _, c := range d.candles {
		if c.IsClosed {
			closedCandles = append(closedCandles, c)
		}
	}

	if len(closedCandles) < cfg.MAWindow {
		return
	}

	// 1. 计算MA20
	var sum float64
	startIdx := len(closedCandles) - cfg.MAWindow
	for i := startIdx; i < len(closedCandles); i++ {
		sum += closedCandles[i].Close
	}
	d.ma20 = sum / float64(cfg.MAWindow)

	// 2. 获取当前价格（最新K线收盘价）
	currentPrice := closedCandles[len(closedCandles)-1].Close

	// 3. 计算连续收阴K线数
	d.consecutiveDowns = 0
	for i := len(closedCandles) - 1; i > 0 && d.consecutiveDowns < cfg.ConsecutiveDownCount+2; i-- {
		if closedCandles[i].Close < closedCandles[i-1].Close {
			d.consecutiveDowns++
		} else {
			break
		}
	}

	// 4. 判定趋势级别
	priceToMA := currentPrice / d.ma20
	oldLevel := d.currentLevel

	if priceToMA < cfg.SevereThreshold && d.consecutiveDowns >= cfg.ConsecutiveDownCount {
		// 严重阴跌：价格低于均线 + 连续收阴
		d.currentLevel = DowntrendSevere
	} else if priceToMA < cfg.MildThreshold {
		// 轻度下跌：价格被均线压制
		d.currentLevel = DowntrendMild
	} else {
		d.currentLevel = DowntrendNone
	}

	d.lastDetectionTime = time.Now()

	// 状态变化时打印日志
	if d.currentLevel != oldLevel {
		switch d.currentLevel {
		case DowntrendSevere:
			logger.Warn("🔻🔻 [阴跌检测] 严重阴跌！价格 %.4f < MA20 %.4f × %.2f，连续 %d 根收阴",
				currentPrice, d.ma20, cfg.SevereThreshold, d.consecutiveDowns)
			logger.Warn("   → 买入数量 ×%.1f，买单窗口 ×%.1f", cfg.SevereMultiplier, cfg.SevereWindowRatio)
		case DowntrendMild:
			logger.Warn("🔻 [阴跌检测] 轻度下跌，价格 %.4f < MA20 %.4f × %.2f",
				currentPrice, d.ma20, cfg.MildThreshold)
			logger.Warn("   → 买入数量 ×%.1f", cfg.MildMultiplier)
		case DowntrendNone:
			logger.Info("✅ [阴跌检测] 趋势恢复正常，价格 %.4f，MA20 %.4f", currentPrice, d.ma20)
		}
	}
}

// GetStatus 获取检测状态（用于日志打印）
func (d *DowntrendDetector) GetStatus() (level DowntrendLevel, ma20 float64, consecutiveDowns int, multiplier float64, windowRatio float64) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cfg := d.getConfigLocked()

	level = d.currentLevel
	ma20 = d.ma20
	consecutiveDowns = d.consecutiveDowns

	switch d.currentLevel {
	case DowntrendMild:
		multiplier = cfg.MildMultiplier
		windowRatio = 1.0
	case DowntrendSevere:
		multiplier = cfg.SevereMultiplier
		windowRatio = cfg.SevereWindowRatio
	default:
		multiplier = 1.0
		windowRatio = 1.0
	}

	return
}
