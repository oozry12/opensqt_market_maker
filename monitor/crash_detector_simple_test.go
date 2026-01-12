package monitor

import (
	"fmt"
	"opensqt/config"
	"opensqt/exchange"
	"testing"
	"time"
)

// TestSimplifiedCrashDetection 测试简化后的暴跌检测逻辑
func TestSimplifiedCrashDetection(t *testing.T) {
	cfg := &config.Config{}
	cfg.Trading.CrashDetection.Enabled = true
	cfg.Trading.CrashDetection.MAWindow = 20
	cfg.Trading.CrashDetection.LongMAWindow = 60
	cfg.Trading.CrashDetection.MinUptrendCandles = 2
	cfg.Trading.CrashDetection.MildCrashRate = 0.006  // 0.6%
	cfg.Trading.CrashDetection.SevereCrashRate = 0.012 // 1.2%
	cfg.Trading.CrashDetection.KlineInterval = "15m"

	mockEx := &MockExchange{}
	detector := NewCrashDetector(cfg, mockEx, "DOGEUSDC")

	fmt.Println("========== 测试简化后的暴跌检测逻辑 ==========")
	fmt.Println()

	// 场景1：正常震荡，无暴跌
	fmt.Println("========== 场景1：正常震荡（小幅波动） ==========")
	testNormalVolatility(t, detector)
	time.Sleep(100 * time.Millisecond)

	// 场景2：2根K线平均跌幅 0.7%（应该触发）
	fmt.Println("\n========== 场景2：2根K线平均跌幅 0.7% ==========")
	detector = NewCrashDetector(cfg, mockEx, "DOGEUSDC")
	testMildCrash(t, detector)
	time.Sleep(100 * time.Millisecond)

	// 场景3：2根K线平均跌幅 1.3%（严重暴跌）
	fmt.Println("\n========== 场景3：2根K线平均跌幅 1.3% ==========")
	detector = NewCrashDetector(cfg, mockEx, "DOGEUSDC")
	testSevereCrash(t, detector)
	time.Sleep(100 * time.Millisecond)

	// 场景4：只有1根K线下跌（不应该触发）
	fmt.Println("\n========== 场景4：只有1根K线下跌 ==========")
	detector = NewCrashDetector(cfg, mockEx, "DOGEUSDC")
	testSingleCandleDrop(t, detector)
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n========== 所有测试完成 ==========")
}

// testNormalVolatility 测试正常震荡
func testNormalVolatility(t *testing.T, detector *CrashDetector) {
	basePrice := 0.14000

	// 生成10根正常波动的K线（涨跌幅都很小）
	for i := 0; i < 10; i++ {
		open := basePrice + float64(i%3-1)*0.00005
		close := open + float64((i+1)%3-1)*0.00003
		
		candle := &exchange.Candle{
			Symbol:   "DOGEUSDC",
			Open:     open,
			High:     max(open, close) + 0.00001,
			Low:      min(open, close) - 0.00001,
			Close:    close,
			IsClosed: true,
		}
		
		injectCandle(detector, candle)
		
		if i == 9 {
			level, _, _, _, crashRate := detector.GetStatus()
			shouldOpenShort := detector.ShouldOpenShort()
			
			fmt.Printf("  K线 #%d: 开盘=%.5f, 收盘=%.5f, 最大平均跌幅=%.2f%%, 级别=%s, 开空=%v\n",
				i+1, open, close, crashRate*100, level.String(), shouldOpenShort)
			
			if shouldOpenShort {
				t.Error("场景1不应该触发做空")
			} else {
				fmt.Println("  ✅ 正常震荡，未触发")
			}
		}
	}
}

// testMildCrash 测试轻度暴跌（2根K线平均跌幅 0.7%）
func testMildCrash(t *testing.T, detector *CrashDetector) {
	basePrice := 0.14000

	// 先生成5根正常K线
	for i := 0; i < 5; i++ {
		candle := &exchange.Candle{
			Symbol:   "DOGEUSDC",
			Open:     basePrice,
			High:     basePrice + 0.00005,
			Low:      basePrice - 0.00005,
			Close:    basePrice + 0.00002,
			IsClosed: true,
		}
		injectCandle(detector, candle)
	}

	// K线6: 下跌 0.7%
	open1 := 0.14000
	close1 := 0.13902 // 跌幅 0.7%
	candle1 := &exchange.Candle{
		Symbol:   "DOGEUSDC",
		Open:     open1,
		High:     open1,
		Low:      close1,
		Close:    close1,
		IsClosed: true,
	}
	injectCandle(detector, candle1)
	fmt.Printf("  K线 #6: 开盘=%.5f, 收盘=%.5f, 跌幅=%.2f%%\n", 
		open1, close1, (open1-close1)/open1*100)

	// K线7: 下跌 0.7%
	open2 := 0.13902
	close2 := 0.13805 // 跌幅 0.7%
	candle2 := &exchange.Candle{
		Symbol:   "DOGEUSDC",
		Open:     open2,
		High:     open2,
		Low:      close2,
		Close:    close2,
		IsClosed: true,
	}
	injectCandle(detector, candle2)
	fmt.Printf("  K线 #7: 开盘=%.5f, 收盘=%.5f, 跌幅=%.2f%%\n", 
		open2, close2, (open2-close2)/open2*100)

	// 检查结果
	level, _, _, _, crashRate := detector.GetStatus()
	shouldOpenShort := detector.ShouldOpenShort()
	
	avgDrop := ((open1-close1)/open1 + (open2-close2)/open2) / 2.0
	fmt.Printf("  平均跌幅: %.2f%%\n", avgDrop*100)
	fmt.Printf("  检测到的最大平均跌幅: %.2f%%\n", crashRate*100)
	fmt.Printf("  级别: %s, 开空: %v\n", level.String(), shouldOpenShort)

	if !shouldOpenShort {
		t.Error("场景2应该触发做空（平均跌幅0.7% > 0.6%）")
	} else {
		fmt.Println("  ✅ 轻度暴跌，已触发")
	}
}

// testSevereCrash 测试严重暴跌（2根K线平均跌幅 1.3%）
func testSevereCrash(t *testing.T, detector *CrashDetector) {
	basePrice := 0.14000

	// 先生成5根正常K线
	for i := 0; i < 5; i++ {
		candle := &exchange.Candle{
			Symbol:   "DOGEUSDC",
			Open:     basePrice,
			High:     basePrice + 0.00005,
			Low:      basePrice - 0.00005,
			Close:    basePrice + 0.00002,
			IsClosed: true,
		}
		injectCandle(detector, candle)
	}

	// K线6: 下跌 1.3%
	open1 := 0.14000
	close1 := 0.13818 // 跌幅 1.3%
	candle1 := &exchange.Candle{
		Symbol:   "DOGEUSDC",
		Open:     open1,
		High:     open1,
		Low:      close1,
		Close:    close1,
		IsClosed: true,
	}
	injectCandle(detector, candle1)
	fmt.Printf("  K线 #6: 开盘=%.5f, 收盘=%.5f, 跌幅=%.2f%%\n", 
		open1, close1, (open1-close1)/open1*100)

	// K线7: 下跌 1.3%
	open2 := 0.13818
	close2 := 0.13638 // 跌幅 1.3%
	candle2 := &exchange.Candle{
		Symbol:   "DOGEUSDC",
		Open:     open2,
		High:     open2,
		Low:      close2,
		Close:    close2,
		IsClosed: true,
	}
	injectCandle(detector, candle2)
	fmt.Printf("  K线 #7: 开盘=%.5f, 收盘=%.5f, 跌幅=%.2f%%\n", 
		open2, close2, (open2-close2)/open2*100)

	// 检查结果
	level, _, _, _, crashRate := detector.GetStatus()
	shouldOpenShort := detector.ShouldOpenShort()
	
	avgDrop := ((open1-close1)/open1 + (open2-close2)/open2) / 2.0
	fmt.Printf("  平均跌幅: %.2f%%\n", avgDrop*100)
	fmt.Printf("  检测到的最大平均跌幅: %.2f%%\n", crashRate*100)
	fmt.Printf("  级别: %s, 开空: %v\n", level.String(), shouldOpenShort)

	if level != CrashSevere {
		t.Errorf("场景3应该触发严重暴跌，实际: %s", level.String())
	} else {
		fmt.Println("  ✅ 严重暴跌，已触发")
	}
}

// testSingleCandleDrop 测试只有1根K线下跌
func testSingleCandleDrop(t *testing.T, detector *CrashDetector) {
	basePrice := 0.14000

	// 先生成5根正常K线
	for i := 0; i < 5; i++ {
		candle := &exchange.Candle{
			Symbol:   "DOGEUSDC",
			Open:     basePrice,
			High:     basePrice + 0.00005,
			Low:      basePrice - 0.00005,
			Close:    basePrice + 0.00002,
			IsClosed: true,
		}
		injectCandle(detector, candle)
	}

	// K线6: 下跌 1.0%
	open1 := 0.14000
	close1 := 0.13860 // 跌幅 1.0%
	candle1 := &exchange.Candle{
		Symbol:   "DOGEUSDC",
		Open:     open1,
		High:     open1,
		Low:      close1,
		Close:    close1,
		IsClosed: true,
	}
	injectCandle(detector, candle1)
	fmt.Printf("  K线 #6: 开盘=%.5f, 收盘=%.5f, 跌幅=%.2f%%\n", 
		open1, close1, (open1-close1)/open1*100)

	// K线7: 上涨（不是下跌）
	open2 := 0.13860
	close2 := 0.13900 // 上涨
	candle2 := &exchange.Candle{
		Symbol:   "DOGEUSDC",
		Open:     open2,
		High:     close2,
		Low:      open2,
		Close:    close2,
		IsClosed: true,
	}
	injectCandle(detector, candle2)
	fmt.Printf("  K线 #7: 开盘=%.5f, 收盘=%.5f, 涨幅=%.2f%%\n", 
		open2, close2, (close2-open2)/open2*100)

	// 检查结果
	level, _, _, _, crashRate := detector.GetStatus()
	shouldOpenShort := detector.ShouldOpenShort()
	
	fmt.Printf("  检测到的最大平均跌幅: %.2f%%\n", crashRate*100)
	fmt.Printf("  级别: %s, 开空: %v\n", level.String(), shouldOpenShort)

	if shouldOpenShort {
		t.Error("场景4不应该触发做空（只有1根K线下跌）")
	} else {
		fmt.Println("  ✅ 只有1根下跌K线，未触发")
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
