package monitor

import (
	"math"
	"opensqt/config"
	"opensqt/logger"
	"sync"
)

// DynamicGridCalculator 动态网格间距计算器
// 根据市场波动率自动调整网格密度
type DynamicGridCalculator struct {
	cfg           *config.Config
	atrCalculator *ATRCalculator

	// 缓存
	lastInterval  float64
	lastATR       float64
	priceDecimals int

	mu sync.RWMutex
}

// NewDynamicGridCalculator 创建动态网格计算器
func NewDynamicGridCalculator(cfg *config.Config, atr *ATRCalculator, priceDecimals int) *DynamicGridCalculator {
	return &DynamicGridCalculator{
		cfg:           cfg,
		atrCalculator: atr,
		priceDecimals: priceDecimals,
	}
}

// CalculateDynamicInterval 计算动态网格间距
// 返回三个值中的最大值：
// 1. 基础间距（配置文件中的固定值）
// 2. 保本间距（确保覆盖手续费并有微利）
// 3. ATR动态间距（根据波动率调整）
func (d *DynamicGridCalculator) CalculateDynamicInterval(currentPrice float64) float64 {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. 基础间距（配置文件中的固定值）
	baseInterval := d.cfg.Trading.PriceInterval

	// 2. 保本间距 = 当前价格 × (买卖手续费 × 2 + 最小利润率)
	// 手续费率从当前交易所配置获取
	feeRate := d.getExchangeFeeRate()
	minProfitRate := d.cfg.Trading.DynamicGrid.MinProfitRate
	if minProfitRate <= 0 {
		minProfitRate = 0.001 // 默认0.1%
	}
	breakEvenInterval := currentPrice * (feeRate*2 + minProfitRate)

	// 3. ATR动态间距 = ATR × 系数
	atrMultiplier := d.cfg.Trading.DynamicGrid.ATRMultiplier
	if atrMultiplier <= 0 {
		atrMultiplier = 0.8 // 默认0.8
	}

	var atrInterval float64
	if d.atrCalculator != nil {
		atr := d.atrCalculator.GetATR()
		if atr > 0 {
			atrInterval = atr * atrMultiplier
			d.lastATR = atr
		}
	}

	// 取三者最大值
	dynamicInterval := math.Max(baseInterval, math.Max(breakEvenInterval, atrInterval))

	// 应用精度
	dynamicInterval = roundToDecimals(dynamicInterval, d.priceDecimals)

	// 确保不低于基础间距
	if dynamicInterval < baseInterval {
		dynamicInterval = baseInterval
	}

	// 记录日志（仅当间距变化时）
	if d.lastInterval != dynamicInterval {
		logger.Info("📐 [动态网格] 间距调整: %.4f -> %.4f (基础:%.4f, 保本:%.4f, ATR:%.4f×%.1f=%.4f)",
			d.lastInterval, dynamicInterval,
			baseInterval, breakEvenInterval,
			d.lastATR, atrMultiplier, atrInterval)
		d.lastInterval = dynamicInterval
	}

	return dynamicInterval
}

// GetCurrentInterval 获取当前网格间距（不重新计算）
func (d *DynamicGridCalculator) GetCurrentInterval() float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.lastInterval > 0 {
		return d.lastInterval
	}
	return d.cfg.Trading.PriceInterval
}

// GetIntervalComponents 获取间距的各个组成部分（用于调试）
func (d *DynamicGridCalculator) GetIntervalComponents(currentPrice float64) (base, breakEven, atrBased, final float64) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	base = d.cfg.Trading.PriceInterval

	feeRate := d.getExchangeFeeRate()
	minProfitRate := d.cfg.Trading.DynamicGrid.MinProfitRate
	if minProfitRate <= 0 {
		minProfitRate = 0.001
	}
	breakEven = currentPrice * (feeRate*2 + minProfitRate)

	atrMultiplier := d.cfg.Trading.DynamicGrid.ATRMultiplier
	if atrMultiplier <= 0 {
		atrMultiplier = 0.8
	}

	if d.atrCalculator != nil {
		atr := d.atrCalculator.GetATR()
		if atr > 0 {
			atrBased = atr * atrMultiplier
		}
	}

	final = math.Max(base, math.Max(breakEven, atrBased))
	final = roundToDecimals(final, d.priceDecimals)

	if final < base {
		final = base
	}

	return
}

// getExchangeFeeRate 获取当前交易所的手续费率
func (d *DynamicGridCalculator) getExchangeFeeRate() float64 {
	exchangeName := d.cfg.App.CurrentExchange
	if exchangeCfg, exists := d.cfg.Exchanges[exchangeName]; exists {
		return exchangeCfg.FeeRate
	}
	return 0.0002 // 默认0.02%
}

// IsEnabled 检查动态网格是否启用
func (d *DynamicGridCalculator) IsEnabled() bool {
	return d.cfg.Trading.DynamicGrid.Enabled
}

// roundToDecimals 四舍五入到指定小数位
func roundToDecimals(value float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Round(value*multiplier) / multiplier
}
