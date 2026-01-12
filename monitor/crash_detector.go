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

// CrashLevel 开空级别（保留用于兼容）
type CrashLevel int

const (
	CrashNone   CrashLevel = iota // 未触发
	CrashMild                     // 轻度（在开空区域内）
	CrashSevere                   // 严重（价格很高）
)

// String 返回级别描述
func (c CrashLevel) String() string {
	switch c {
	case CrashNone:
		return "未触发"
	case CrashMild:
		return "开空区域"
	case CrashSevere:
		return "高位区域"
	default:
		return "未知"
	}
}

// ShortGridConfig 做空网格配置
type ShortGridConfig struct {
	Enabled           bool
	KlineInterval     string
	KlineCount        int     // 检查K线数量（默认5）
	MinMultiplier     float64 // 最小倍数（默认1.2）
	MaxMultiplier     float64 // 最大倍数（默认3.0）
	MaxShortPositions int     // 最大空仓数量（默认10）
}

// CrashDetector 开空检测器
// 新逻辑：以最近N根K线最高点为锚点，在指定倍数区域挂空单
type CrashDetector struct {
	cfg      *config.Config
	exchange exchange.IExchange
	symbol   string

	// K线数据缓存
	candles []*exchange.Candle
	mu      sync.RWMutex

	// 检测结果
	currentLevel    CrashLevel
	anchorHighest   float64 // 锚点：最近N根K线的最高点
	shortZoneMin    float64 // 做空区域最小价格（锚点 × 1.2）
	shortZoneMax    float64 // 做空区域最大价格（锚点 × 3.0）
	currentPrice    float64 // 当前价格
	shouldShort     bool    // 是否应该开空（当前价格在做空区域内）

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCrashDetector 创建开空检测器
func NewCrashDetector(cfg *config.Config, ex exchange.IExchange, symbol string) *CrashDetector {
	return &CrashDetector{
		cfg:          cfg,
		exchange:     ex,
		symbol:       symbol,
		candles:      make([]*exchange.Candle, 0, 20),
		currentLevel: CrashNone,
	}
}

// Start 启动检测器
func (d *CrashDetector) Start(ctx context.Context) error {
	d.ctx, d.cancel = context.WithCancel(ctx)

	if err := d.loadHistoricalData(); err != nil {
		logger.Warn("⚠️ [开空检测] 加载历史数据失败: %v", err)
	}

	d.wg.Add(1)
	go d.subscribeKlineStream()

	cfg := d.getConfig()
	logger.Info("✅ [开空检测] 已启动 - 锚点区域: %.1f倍 ~ %.1f倍", cfg.MinMultiplier, cfg.MaxMultiplier)
	return nil
}

// Stop 停止检测器
func (d *CrashDetector) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
	logger.Info("✅ [开空检测] 已停止")
}

// GetCrashLevel 获取当前级别
func (d *CrashDetector) GetCrashLevel() CrashLevel {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentLevel
}

// ShouldOpenShort 是否应该开空仓
// 只要做空区域有效就返回true，允许预先挂空单
func (d *CrashDetector) ShouldOpenShort() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.cfg.Trading.CrashDetection.Enabled {
		logger.Debug("🔍 [开空检测] 未启用")
		return false
	}

	// 只要锚点有效，就允许在做空区域挂空单
	result := d.anchorHighest > 0 && d.shortZoneMin > 0
	if !result {
		logger.Debug("🔍 [开空检测] 锚点无效: anchor=%.6f, shortZoneMin=%.6f", d.anchorHighest, d.shortZoneMin)
	}
	return result
}

// GetShortZone 获取做空区域
// 返回：锚点价格、最小价格、最大价格
func (d *CrashDetector) GetShortZone() (anchor, minPrice, maxPrice float64) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.anchorHighest, d.shortZoneMin, d.shortZoneMax
}

// GetMaxShortPositions 获取最大空仓数量
func (d *CrashDetector) GetMaxShortPositions() int {
	cfg := d.getConfig()
	return cfg.MaxShortPositions
}

// GetCrashRate 获取当前价格与锚点的比例
func (d *CrashDetector) GetCrashRate() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.anchorHighest > 0 {
		return d.currentPrice / d.anchorHighest
	}
	return 0
}

// GetUptrendCandles 兼容旧接口
func (d *CrashDetector) GetUptrendCandles() int {
	return 0
}

// IsEnabled 检查是否启用
func (d *CrashDetector) IsEnabled() bool {
	return d.cfg.Trading.CrashDetection.Enabled
}

// getConfig 获取配置
func (d *CrashDetector) getConfig() ShortGridConfig {
	cfg := d.cfg.Trading.CrashDetection

	result := ShortGridConfig{
		Enabled:           cfg.Enabled,
		KlineInterval:     cfg.KlineInterval,
		KlineCount:        5,   // 固定检查5根K线
		MinMultiplier:     cfg.ShortZoneMinMult,
		MaxMultiplier:     cfg.ShortZoneMaxMult,
		MaxShortPositions: cfg.MaxShortPositions,
	}

	// 设置默认值
	if result.KlineInterval == "" {
		result.KlineInterval = "5m"
	}
	if result.MinMultiplier <= 0 {
		result.MinMultiplier = 1.2
	}
	if result.MaxMultiplier <= 0 {
		result.MaxMultiplier = 3.0
	}
	if result.MaxShortPositions <= 0 {
		result.MaxShortPositions = 10
	}

	return result
}

// loadHistoricalData 加载历史K线数据
func (d *CrashDetector) loadHistoricalData() error {
	cfg := d.getConfig()

	candles, err := d.exchange.GetHistoricalKlines(d.ctx, d.symbol, cfg.KlineInterval, 10)
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.candles = candles
	d.mu.Unlock()

	d.detect()

	logger.Info("✅ [开空检测] 已加载 %d 根历史K线, 锚点:%.6f, 做空区域:[%.6f ~ %.6f]", 
		len(candles), d.anchorHighest, d.shortZoneMin, d.shortZoneMax)
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
		logger.Warn("⚠️ [开空检测] 订阅K线流失败: %v", err)
		if strings.Contains(err.Error(), "K线流已在运行") || strings.Contains(err.Error(), "K线流未启动") {
			logger.Info("🔄 [开空检测] K线流已在运行，尝试注册回调...")
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
				logger.Error("❌ [开空检测] 注册回调失败: %v", err)
				d.fallbackPolling()
			} else {
				logger.Info("✅ [开空检测] 已注册K线回调")
			}
		} else {
			d.fallbackPolling()
		}
	}
}

// fallbackPolling 降级轮询模式
func (d *CrashDetector) fallbackPolling() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			if err := d.loadHistoricalData(); err != nil {
				logger.Warn("⚠️ [开空检测] 轮询更新失败: %v", err)
			}
		}
	}
}

// onCandleUpdate K线更新回调
func (d *CrashDetector) onCandleUpdate(candle *exchange.Candle) {
	d.mu.Lock()

	if candle.IsClosed {
		d.candles = append(d.candles, candle)
		if len(d.candles) > 10 {
			d.candles = d.candles[len(d.candles)-10:]
		}
	} else {
		if len(d.candles) > 0 && !d.candles[len(d.candles)-1].IsClosed {
			d.candles[len(d.candles)-1] = candle
		} else {
			d.candles = append(d.candles, candle)
		}
	}

	d.mu.Unlock()

	d.detect()
}

// detect 执行开空检测
// 逻辑：以最近5根K线最高点为锚点，计算做空区域（1.2倍~3倍）
func (d *CrashDetector) detect() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cfg := d.getConfig()

	// 只保留已关闭的K线
	closedCandles := make([]*exchange.Candle, 0)
	for _, c := range d.candles {
		if c.IsClosed {
			closedCandles = append(closedCandles, c)
		}
	}

	// 至少需要5根K线
	if len(closedCandles) < cfg.KlineCount {
		logger.Debug("🔍 [开空检测] K线数量不足: %d/%d", len(closedCandles), cfg.KlineCount)
		return
	}

	// 获取最近5根K线的最高点作为锚点
	startIdx := len(closedCandles) - cfg.KlineCount
	highest := 0.0
	for i := startIdx; i < len(closedCandles); i++ {
		if closedCandles[i].High > highest {
			highest = closedCandles[i].High
		}
	}

	// 计算做空区域
	d.anchorHighest = highest
	d.shortZoneMin = highest * cfg.MinMultiplier // 1.2倍
	d.shortZoneMax = highest * cfg.MaxMultiplier // 3.0倍

	// 获取当前价格
	d.currentPrice = closedCandles[len(closedCandles)-1].Close

	oldShouldShort := d.shouldShort

	// 判断当前价格是否在做空区域内
	if d.currentPrice >= d.shortZoneMin && d.currentPrice <= d.shortZoneMax {
		d.shouldShort = true
		if d.currentPrice >= highest*2.0 {
			d.currentLevel = CrashSevere // 2倍以上，高位区域
		} else {
			d.currentLevel = CrashMild // 1.2-2倍，开空区域
		}
	} else {
		d.shouldShort = false
		d.currentLevel = CrashNone
	}

	// 调试日志
	logger.Debug("🔍 [开空检测] 锚点:%.6f, 做空区域:[%.6f ~ %.6f], 当前价格:%.6f, 开空:%v",
		d.anchorHighest, d.shortZoneMin, d.shortZoneMax, d.currentPrice, d.shouldShort)

	// 状态变化时输出日志
	if d.shouldShort != oldShouldShort {
		if d.shouldShort {
			ratio := d.currentPrice / d.anchorHighest
			logger.Warn("🔴 [开空检测] 进入做空区域！锚点:%.6f, 当前价格:%.6f (%.1f倍), 区域:[%.6f ~ %.6f]",
				d.anchorHighest, d.currentPrice, ratio, d.shortZoneMin, d.shortZoneMax)
		} else {
			logger.Info("✅ [开空检测] 离开做空区域，当前价格:%.6f", d.currentPrice)
		}
	}
}

// GetStatus 获取检测状态（兼容旧接口）
func (d *CrashDetector) GetStatus() (level CrashLevel, ma20 float64, ma60 float64, uptrendCandles int, crashRate float64) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	level = d.currentLevel
	ma20 = d.shortZoneMin   // 复用：做空区域最小价格
	ma60 = d.shortZoneMax   // 复用：做空区域最大价格
	uptrendCandles = 0
	if d.anchorHighest > 0 {
		crashRate = d.currentPrice / d.anchorHighest
	}

	return
}
