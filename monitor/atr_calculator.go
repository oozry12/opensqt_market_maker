package monitor

import (
	"context"
	"math"
	"opensqt/exchange"
	"opensqt/logger"
	"sync"
	"time"
)

// ATRCalculator ATR（平均真实波幅）计算器
// 用于动态调整网格间距
type ATRCalculator struct {
	exchange exchange.IExchange
	symbol   string
	interval string // K线周期，如 "1m", "5m", "15m"
	period   int    // ATR周期，默认14

	// ATR缓存
	currentATR float64
	lastUpdate time.Time
	mu         sync.RWMutex

	// K线数据缓存
	candles []*exchange.Candle

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewATRCalculator 创建ATR计算器
func NewATRCalculator(ex exchange.IExchange, symbol, interval string, period int) *ATRCalculator {
	if period <= 0 {
		period = 14 // 默认14周期
	}
	if interval == "" {
		interval = "5m" // 默认5分钟K线
	}

	return &ATRCalculator{
		exchange: ex,
		symbol:   symbol,
		interval: interval,
		period:   period,
		candles:  make([]*exchange.Candle, 0, period+1),
	}
}

// Start 启动ATR计算器
func (a *ATRCalculator) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)

	// 1. 加载历史K线数据计算初始ATR
	if err := a.loadHistoricalData(); err != nil {
		logger.Warn("⚠️ [ATR] 加载历史数据失败: %v，将使用默认值", err)
	}

	// 2. 订阅K线流实时更新
	a.wg.Add(1)
	go a.subscribeKlineStream()

	logger.Info("✅ [ATR] 计算器已启动 (周期: %s, ATR周期: %d)", a.interval, a.period)
	return nil
}

// Stop 停止ATR计算器
func (a *ATRCalculator) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()
	logger.Info("✅ [ATR] 计算器已停止")
}

// GetATR 获取当前ATR值
func (a *ATRCalculator) GetATR() float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.currentATR
}

// GetLastUpdate 获取最后更新时间
func (a *ATRCalculator) GetLastUpdate() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastUpdate
}

// loadHistoricalData 加载历史K线数据
func (a *ATRCalculator) loadHistoricalData() error {
	// 获取足够的历史K线（ATR周期 + 1）
	limit := a.period + 5 // 多获取几根以防万一
	candles, err := a.exchange.GetHistoricalKlines(a.ctx, a.symbol, a.interval, limit)
	if err != nil {
		return err
	}

	if len(candles) < a.period+1 {
		logger.Warn("⚠️ [ATR] 历史K线数量不足: %d < %d", len(candles), a.period+1)
		return nil
	}

	a.mu.Lock()
	a.candles = candles
	a.mu.Unlock()

	// 计算初始ATR
	a.calculateATR()

	logger.Info("✅ [ATR] 已加载 %d 根历史K线，初始ATR: %.4f", len(candles), a.GetATR())
	return nil
}

// subscribeKlineStream 订阅K线流
func (a *ATRCalculator) subscribeKlineStream() {
	defer a.wg.Done()

	// 使用交易所的K线流
	err := a.exchange.StartKlineStream(a.ctx, []string{a.symbol}, a.interval, func(candle *exchange.Candle) {
		if candle == nil || candle.Symbol != a.symbol {
			return
		}
		a.onCandleUpdate(candle)
	})

	if err != nil {
		logger.Error("❌ [ATR] 订阅K线流失败: %v", err)
		// 降级：使用定时轮询
		a.fallbackPolling()
	}
}

// fallbackPolling 降级轮询模式
func (a *ATRCalculator) fallbackPolling() {
	// 根据K线周期确定轮询间隔
	pollInterval := 1 * time.Minute
	switch a.interval {
	case "1m":
		pollInterval = 30 * time.Second
	case "5m":
		pollInterval = 1 * time.Minute
	case "15m":
		pollInterval = 5 * time.Minute
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			if err := a.loadHistoricalData(); err != nil {
				logger.Warn("⚠️ [ATR] 轮询更新失败: %v", err)
			}
		}
	}
}

// onCandleUpdate K线更新回调
func (a *ATRCalculator) onCandleUpdate(candle *exchange.Candle) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if candle.IsClosed {
		// 完结的K线：追加到列表
		a.candles = append(a.candles, candle)

		// 保留足够数量的K线
		maxCandles := a.period + 5
		if len(a.candles) > maxCandles {
			a.candles = a.candles[len(a.candles)-maxCandles:]
		}

		// 重新计算ATR
		a.calculateATRLocked()
	} else {
		// 未完结的K线：更新最后一根
		if len(a.candles) > 0 && !a.candles[len(a.candles)-1].IsClosed {
			a.candles[len(a.candles)-1] = candle
		} else {
			a.candles = append(a.candles, candle)
		}
	}
}

// calculateATR 计算ATR（加锁版本）
func (a *ATRCalculator) calculateATR() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calculateATRLocked()
}

// calculateATRLocked 计算ATR（内部方法，需要已持有锁）
func (a *ATRCalculator) calculateATRLocked() {
	if len(a.candles) < a.period+1 {
		return
	}

	// 计算True Range序列
	trValues := make([]float64, 0, a.period)

	// 只使用完结的K线
	closedCandles := make([]*exchange.Candle, 0)
	for _, c := range a.candles {
		if c.IsClosed {
			closedCandles = append(closedCandles, c)
		}
	}

	if len(closedCandles) < a.period+1 {
		return
	}

	// 从最新的K线开始计算
	startIdx := len(closedCandles) - a.period - 1
	for i := startIdx + 1; i < len(closedCandles); i++ {
		current := closedCandles[i]
		previous := closedCandles[i-1]

		// True Range = max(High-Low, |High-PrevClose|, |Low-PrevClose|)
		tr := a.calculateTrueRange(current.High, current.Low, previous.Close)
		trValues = append(trValues, tr)
	}

	if len(trValues) < a.period {
		return
	}

	// 计算ATR（简单移动平均）
	var sum float64
	for _, tr := range trValues[len(trValues)-a.period:] {
		sum += tr
	}
	a.currentATR = sum / float64(a.period)
	a.lastUpdate = time.Now()

	logger.Debug("📊 [ATR] 更新: %.4f (基于 %d 根K线)", a.currentATR, len(trValues))
}

// calculateTrueRange 计算单根K线的True Range
func (a *ATRCalculator) calculateTrueRange(high, low, prevClose float64) float64 {
	// TR = max(High-Low, |High-PrevClose|, |Low-PrevClose|)
	hl := high - low
	hpc := math.Abs(high - prevClose)
	lpc := math.Abs(low - prevClose)

	return math.Max(hl, math.Max(hpc, lpc))
}
