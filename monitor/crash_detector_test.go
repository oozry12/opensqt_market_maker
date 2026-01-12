package monitor

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"opensqt/config"
	"opensqt/exchange"
)

type MockExchange struct{}

func (m *MockExchange) GetName() string {
	return "Mock"
}

func (m *MockExchange) PlaceOrder(ctx context.Context, req *exchange.OrderRequest) (*exchange.Order, error) {
	return nil, nil
}

func (m *MockExchange) BatchPlaceOrders(ctx context.Context, orders []*exchange.OrderRequest) ([]*exchange.Order, bool) {
	return nil, false
}

func (m *MockExchange) CancelOrder(ctx context.Context, symbol string, orderID int64) error {
	return nil
}

func (m *MockExchange) BatchCancelOrders(ctx context.Context, symbol string, orderIDs []int64) error {
	return nil
}

func (m *MockExchange) CancelAllOrders(ctx context.Context, symbol string) error {
	return nil
}

func (m *MockExchange) GetOrder(ctx context.Context, symbol string, orderID int64) (*exchange.Order, error) {
	return nil, nil
}

func (m *MockExchange) GetOpenOrders(ctx context.Context, symbol string) ([]*exchange.Order, error) {
	return nil, nil
}

func (m *MockExchange) GetAccount(ctx context.Context) (*exchange.Account, error) {
	return nil, nil
}

func (m *MockExchange) GetPositions(ctx context.Context, symbol string) ([]*exchange.Position, error) {
	return nil, nil
}

func (m *MockExchange) GetBalance(ctx context.Context, asset string) (float64, error) {
	return 0, nil
}

func (m *MockExchange) StartOrderStream(ctx context.Context, callback func(interface{})) error {
	return nil
}

func (m *MockExchange) StopOrderStream() error {
	return nil
}

func (m *MockExchange) GetLatestPrice(ctx context.Context, symbol string) (float64, error) {
	return 0, nil
}

func (m *MockExchange) StartPriceStream(ctx context.Context, symbol string, callback func(price float64)) error {
	return nil
}

func (m *MockExchange) StartKlineStream(ctx context.Context, symbols []string, interval string, callback exchange.CandleUpdateCallback) error {
	return nil
}

func (m *MockExchange) StopKlineStream() error {
	return nil
}

func (m *MockExchange) GetHistoricalKlines(ctx context.Context, symbol string, interval string, limit int) ([]*exchange.Candle, error) {
	return nil, nil
}

func (m *MockExchange) RegisterKlineCallback(name string, callback func(interface{})) error {
	return nil
}

func (m *MockExchange) GetPriceDecimals() int {
	return 4
}

func (m *MockExchange) GetQuantityDecimals() int {
	return 2
}

func (m *MockExchange) GetBaseAsset() string {
	return "TEST"
}

func (m *MockExchange) GetQuoteAsset() string {
	return "USDT"
}

func TestCrashDetector(t *testing.T) {
	cfg := &config.Config{}
	cfg.Trading.CrashDetection.Enabled = true
	cfg.Trading.CrashDetection.MAWindow = 20
	cfg.Trading.CrashDetection.LongMAWindow = 60
	cfg.Trading.CrashDetection.MinUptrendCandles = 5
	cfg.Trading.CrashDetection.MildCrashRate = 0.05
	cfg.Trading.CrashDetection.SevereCrashRate = 0.10
	cfg.Trading.CrashDetection.KlineInterval = "5m"

	mockEx := &MockExchange{}
	detector := NewCrashDetector(cfg, mockEx, "TESTUSDT")

	fmt.Println("========== 场景1：连续上涨K线（单边上涨趋势）==========")
	testScenario1(t, detector)
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n========== 场景2：单边上涨后暴跌（应该触发做空）==========")
	detector = NewCrashDetector(cfg, mockEx, "TESTUSDT")
	testScenario2(t, detector)
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n========== 场景3：正常波动（不应该触发做空）==========")
	detector = NewCrashDetector(cfg, mockEx, "TESTUSDT")
	testScenario3(t, detector)
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n========== 场景4：严重暴跌（应该触发严重做空）==========")
	detector = NewCrashDetector(cfg, mockEx, "TESTUSDT")
	testScenario4(t, detector)
}

func testScenario1(t *testing.T, detector *CrashDetector) {
	basePrice := 100.0

	for i := 0; i < 70; i++ {
		candle := &exchange.Candle{
			Symbol:    "TESTUSDT",
			Open:      basePrice,
			Close:     basePrice * (1 + 0.01),
			High:      basePrice * (1 + 0.015),
			Low:       basePrice * (1 - 0.005),
			Volume:    1000,
			IsClosed:  true,
			Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute).UnixMilli(),
		}
		injectCandle(detector, candle)
		basePrice = candle.Close

		if i >= 59 {
			printDetectorStatus(detector, i+1)
		}
	}

	level, _, _, uptrendCandles, crashRate := detector.GetStatus()
	shouldOpenShort := detector.ShouldOpenShort()

	if level != CrashNone {
		t.Errorf("场景1期望无暴跌，实际: %s", level.String())
	}
	if shouldOpenShort {
		t.Error("场景1不应该触发做空")
	}
	if uptrendCandles < 5 {
		t.Errorf("场景1期望至少5根上涨K线，实际: %d", uptrendCandles)
	}
	if crashRate > 0.01 {
		t.Errorf("场景1期望暴跌幅度很小，实际: %.2f%%", crashRate*100)
	}
}

func testScenario2(t *testing.T, detector *CrashDetector) {
	basePrice := 100.0

	var crashTriggered bool
	var crashLevel CrashLevel
	var crashUptrendCandles int
	var crashRateAtTrigger float64

	for i := 0; i < 70; i++ {
		var candle *exchange.Candle
		
		if i < 60 {
			candle = &exchange.Candle{
				Symbol:    "TESTUSDT",
				Open:      basePrice,
				Close:     basePrice * (1 + 0.01),
				High:      basePrice * (1 + 0.015),
				Low:       basePrice * (1 - 0.005),
				Volume:    1000,
				IsClosed:  true,
				Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute).UnixMilli(),
			}
		} else {
			candle = &exchange.Candle{
				Symbol:    "TESTUSDT",
				Open:      basePrice,
				Close:     basePrice * (1 - 0.06),
				High:      basePrice * (1 + 0.01),
				Low:       basePrice * (1 - 0.07),
				Volume:    2000,
				IsClosed:  true,
				Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute).UnixMilli(),
			}
		}
		injectCandle(detector, candle)
		basePrice = candle.Close

		if i >= 59 {
			printDetectorStatus(detector, i+1)
		}

		if i == 60 {
			level, ma20, ma60, uptrendCandles, crashRate := detector.GetStatus()
			shouldOpenShort := detector.ShouldOpenShort()
			crashTriggered = shouldOpenShort
			crashLevel = level
			crashUptrendCandles = uptrendCandles
			crashRateAtTrigger = crashRate
			fmt.Printf("验证: MA20(%.4f) > MA60(%.4f) = %v, 上涨K线=%d >= 5 = %v, 暴跌=%.2f%% >= 5%% = %v\n",
				ma20, ma60, ma20 > ma60, uptrendCandles, uptrendCandles >= 5, crashRate*100, crashRate >= 0.05)
		}
	}

	if !crashTriggered {
		t.Error("场景2应该触发做空")
	}
	if crashLevel != CrashMild {
		t.Errorf("场景2期望轻度暴跌，实际: %s", crashLevel.String())
	}
	if crashUptrendCandles < 5 {
		t.Errorf("场景2期望至少5根上涨K线，实际: %d", crashUptrendCandles)
	}
	if crashRateAtTrigger < 0.05 {
		t.Errorf("场景2期望暴跌幅度>=5%%，实际: %.2f%%", crashRateAtTrigger*100)
	}
}

func testScenario3(t *testing.T, detector *CrashDetector) {
	basePrice := 100.0

	for i := 0; i < 70; i++ {
		change := math.Sin(float64(i)*0.2) * 0.02
		candle := &exchange.Candle{
			Symbol:    "TESTUSDT",
			Open:      basePrice,
			Close:     basePrice * (1 + change),
			High:      basePrice * (1 + change + 0.01),
			Low:       basePrice * (1 + change - 0.01),
			Volume:    1000,
			IsClosed:  true,
			Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute).UnixMilli(),
		}
		injectCandle(detector, candle)
		basePrice = candle.Close

		if i >= 59 {
			printDetectorStatus(detector, i+1)
		}
	}

	level, _, _, _, crashRate := detector.GetStatus()
	shouldOpenShort := detector.ShouldOpenShort()

	if level != CrashNone {
		t.Errorf("场景3期望无暴跌，实际: %s", level.String())
	}
	if shouldOpenShort {
		t.Error("场景3不应该触发做空")
	}
	if crashRate > 0.05 {
		t.Errorf("场景3期望暴跌幅度<5%%，实际: %.2f%%", crashRate*100)
	}
}

func testScenario4(t *testing.T, detector *CrashDetector) {
	basePrice := 100.0

	var crashTriggered bool
	var crashLevel CrashLevel
	var crashUptrendCandles int
	var crashRateAtTrigger float64

	for i := 0; i < 70; i++ {
		var candle *exchange.Candle
		
		if i < 60 {
			candle = &exchange.Candle{
				Symbol:    "TESTUSDT",
				Open:      basePrice,
				Close:     basePrice * (1 + 0.015),
				High:      basePrice * (1 + 0.02),
				Low:       basePrice * (1 - 0.005),
				Volume:    1000,
				IsClosed:  true,
				Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute).UnixMilli(),
			}
		} else {
			candle = &exchange.Candle{
				Symbol:    "TESTUSDT",
				Open:      basePrice,
				Close:     basePrice * (1 - 0.12),
				High:      basePrice * (1 + 0.005),
				Low:       basePrice * (1 - 0.13),
				Volume:    3000,
				IsClosed:  true,
				Timestamp: time.Now().Add(time.Duration(i) * 5 * time.Minute).UnixMilli(),
			}
		}
		injectCandle(detector, candle)
		basePrice = candle.Close

		if i >= 59 {
			printDetectorStatus(detector, i+1)
		}

		if i == 60 {
			level, ma20, ma60, uptrendCandles, crashRate := detector.GetStatus()
			shouldOpenShort := detector.ShouldOpenShort()
			crashTriggered = shouldOpenShort
			crashLevel = level
			crashUptrendCandles = uptrendCandles
			crashRateAtTrigger = crashRate
			fmt.Printf("验证: MA20(%.4f) > MA60(%.4f) = %v, 上涨K线=%d >= 5 = %v, 暴跌=%.2f%% >= 10%% = %v\n",
				ma20, ma60, ma20 > ma60, uptrendCandles, uptrendCandles >= 5, crashRate*100, crashRate >= 0.10)
		}
	}

	if !crashTriggered {
		t.Error("场景4应该触发做空")
	}
	if crashLevel != CrashSevere {
		t.Errorf("场景4期望严重暴跌，实际: %s", crashLevel.String())
	}
	if crashUptrendCandles < 5 {
		t.Errorf("场景4期望至少5根上涨K线，实际: %d", crashUptrendCandles)
	}
	if crashRateAtTrigger < 0.10 {
		t.Errorf("场景4期望暴跌幅度>=10%%，实际: %.2f%%", crashRateAtTrigger*100)
	}
}

func injectCandle(detector *CrashDetector, candle *exchange.Candle) {
	detector.mu.Lock()

	cfg := detector.getConfigLocked()
	maxCandles := cfg.LongMAWindow + cfg.MinUptrendCandles + 10

	if candle.IsClosed {
		detector.candles = append(detector.candles, candle)
		if len(detector.candles) > maxCandles {
			detector.candles = detector.candles[len(detector.candles)-maxCandles:]
		}
	}

	detector.mu.Unlock()

	detector.detect()
}

func printDetectorStatus(detector *CrashDetector, candleNum int) {
	level, ma20, ma60, uptrendCandles, crashRate := detector.GetStatus()
	shouldOpenShort := detector.ShouldOpenShort()

	fmt.Printf("K线 #%d: MA20=%.4f, MA60=%.4f, 上涨K线=%d, 暴跌=%.2f%%, 级别=%s, 开空=%v\n",
		candleNum, ma20, ma60, uptrendCandles, crashRate*100, level.String(), shouldOpenShort)
}
