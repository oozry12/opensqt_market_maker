package main

import (
	"fmt"
	"math"
	"time"

	"opensqt/config"
	"opensqt/exchange"
	"opensqt/logger"
	"opensqt/monitor"
)

func main() {
	logger.Init("debug", "", false, true)

	cfg := &config.Config{
		Trading: config.TradingConfig{
			CrashDetection: config.CrashDetection{
				Enabled:         true,
				MAWindow:        20,
				LongMAWindow:    60,
				MinUptrendCandles: 5,
				MildCrashRate:   0.05,
				SevereCrashRate: 0.10,
				KlineInterval:   "5m",
			},
		},
	}

	detector := monitor.NewCrashDetector(cfg, nil)

	fmt.Println("========== 场景1：连续上涨K线（单边上涨趋势）==========")
	testScenario1(detector)
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n========== 场景2：单边上涨后暴跌（应该触发做空）==========")
	detector = monitor.NewCrashDetector(cfg, nil)
	testScenario2(detector)
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n========== 场景3：正常波动（不应该触发做空）==========")
	detector = monitor.NewCrashDetector(cfg, nil)
	testScenario3(detector)
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n========== 场景4：严重暴跌（应该触发严重做空）==========")
	detector = monitor.NewCrashDetector(cfg, nil)
	testScenario4(detector)
}

func testScenario1(detector *monitor.CrashDetector) {
	basePrice := 100.0

	for i := 0; i < 70; i++ {
		candle := &exchange.Candle{
			Symbol:    "TESTUSDT",
			Interval:  "5m",
			Open:      basePrice,
			Close:     basePrice * (1 + 0.01), 
			High:      basePrice * (1 + 0.015),
			Low:       basePrice * (1 - 0.005),
			Volume:    1000,
			IsClosed:  true,
			Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute),
		}
		detector.OnCandleUpdate(candle)
		basePrice = candle.Close

		if i >= 59 {
			printDetectorStatus(detector, i+1)
		}
	}
}

func testScenario2(detector *monitor.CrashDetector) {
	basePrice := 100.0

	for i := 0; i < 70; i++ {
		var candle *exchange.Candle
		
		if i < 60 {
			candle = &exchange.Candle{
				Symbol:    "TESTUSDT",
				Interval:  "5m",
				Open:      basePrice,
				Close:     basePrice * (1 + 0.01),
				High:      basePrice * (1 + 0.015),
				Low:       basePrice * (1 - 0.005),
				Volume:    1000,
				IsClosed:  true,
				Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute),
			}
		} else {
			candle = &exchange.Candle{
				Symbol:    "TESTUSDT",
				Interval:  "5m",
				Open:      basePrice,
				Close:     basePrice * (1 - 0.06),
				High:      basePrice * (1 + 0.01),
				Low:       basePrice * (1 - 0.07),
				Volume:    2000,
				IsClosed:  true,
				Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute),
			}
		}
		detector.OnCandleUpdate(candle)
		basePrice = candle.Close

		if i >= 59 {
			printDetectorStatus(detector, i+1)
		}
	}
}

func testScenario3(detector *monitor.CrashDetector) {
	basePrice := 100.0

	for i := 0; i < 70; i++ {
		change := math.Sin(float64(i)*0.2) * 0.02
		candle := &exchange.Candle{
			Symbol:    "TESTUSDT",
			Interval:  "5m",
			Open:      basePrice,
			Close:     basePrice * (1 + change),
			High:      basePrice * (1 + change + 0.01),
			Low:       basePrice * (1 + change - 0.01),
			Volume:    1000,
			IsClosed:  true,
			Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute),
		}
		detector.OnCandleUpdate(candle)
		basePrice = candle.Close

		if i >= 59 {
			printDetectorStatus(detector, i+1)
		}
	}
}

func testScenario4(detector *monitor.CrashDetector) {
	basePrice := 100.0

	for i := 0; i < 70; i++ {
		var candle *exchange.Candle
		
		if i < 60 {
			candle = &exchange.Candle{
				Symbol:    "TESTUSDT",
				Interval:  "5m",
				Open:      basePrice,
				Close:     basePrice * (1 + 0.015),
				High:      basePrice * (1 + 0.02),
				Low:       basePrice * (1 - 0.005),
				Volume:    1000,
				IsClosed:  true,
				Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute),
			}
		} else {
			candle = &exchange.Candle{
				Symbol:    "TESTUSDT",
				Interval:  "5m",
				Open:      basePrice,
				Close:     basePrice * (1 - 0.12),
				High:      basePrice * (1 + 0.005),
				Low:       basePrice * (1 - 0.13),
				Volume:    3000,
				IsClosed:  true,
				Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute),
			}
		}
		detector.OnCandleUpdate(candle)
		basePrice = candle.Close

		if i >= 59 {
			printDetectorStatus(detector, i+1)
		}
	}
}

func printDetectorStatus(detector *monitor.CrashDetector, candleNum int) {
	level, ma20, ma60, uptrendCandles, crashRate := detector.GetStatus()
	shouldOpenShort := detector.ShouldOpenShort()

	fmt.Printf("K线 #%d: MA20=%.4f, MA60=%.4f, 上涨K线=%d, 暴跌=%.2f%%, 级别=%s, 开空=%v\n",
		candleNum, ma20, ma60, uptrendCandles, crashRate*100, level.String(), shouldOpenShort)
}
