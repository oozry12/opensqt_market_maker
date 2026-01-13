package main

import (
	"flag"
	"fmt"
	"os"
	"opensqt/config"
	"opensqt/logger"
	"opensqt/simulation"
	"time"
)

func main() {
	// 命令行参数
	duration := flag.Duration("duration", 5*time.Minute, "模拟运行时长")
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	verbose := flag.Bool("verbose", false, "详细日志输出")

	flag.Parse()

	// 初始化日志
	logLevel := logger.INFO
	if *verbose {
		logLevel = logger.DEBUG
	}
	logger.SetLevel(logLevel)

	fmt.Println("🤖 OpenSQT 市场制造者 - 模拟测试")
	fmt.Printf("📋 使用配置文件: %s\n", *configPath)
	fmt.Printf("⏱️  模拟时长: %v\n", *duration)
	fmt.Println("🚀 启动模拟系统...")

	// 创建模拟专用配置
	cfg := &config.Config{
		App: struct {
			CurrentExchange string `yaml:"current_exchange"`
		}{
			CurrentExchange: "mock",
		},
		Trading: struct {
			Symbol                string  `yaml:"symbol"`
			PriceInterval         float64 `yaml:"price_interval"`
			OrderQuantity         float64 `yaml:"order_quantity"`
			MinOrderValue         float64 `yaml:"min_order_value"`
			BuyWindowSize         int     `yaml:"buy_window_size"`
			SellWindowSize        int     `yaml:"sell_window_size"`
			ReconcileInterval     int     `yaml:"reconcile_interval"`
			OrderCleanupThreshold int     `yaml:"order_cleanup_threshold"`
			CleanupBatchSize      int     `yaml:"cleanup_batch_size"`
			MarginLockDurationSec int     `yaml:"margin_lock_duration_seconds"`
			PositionSafetyCheck   int     `yaml:"position_safety_check"`
			MinMarginBalance      float64 `yaml:"min_margin_balance"`
			DynamicGrid           struct {
				Enabled       bool    `yaml:"enabled"`
				ATRPeriod     int     `yaml:"atr_period"`
				ATRInterval   string  `yaml:"atr_interval"`
				ATRMultiplier float64 `yaml:"atr_multiplier"`
				MinProfitRate float64 `yaml:"min_profit_rate"`
			} `yaml:"dynamic_grid"`
			DowntrendDetection struct {
				Enabled              bool    `yaml:"enabled"`
				MAWindow             int     `yaml:"ma_window"`
				MildThreshold        float64 `yaml:"mild_threshold"`
				SevereThreshold      float64 `yaml:"severe_threshold"`
				ConsecutiveDownCount int     `yaml:"consecutive_down_count"`
				MildMultiplier       float64 `yaml:"mild_multiplier"`
				SevereMultiplier     float64 `yaml:"severe_multiplier"`
				SevereWindowRatio    float64 `yaml:"severe_window_ratio"`
				KlineInterval        string  `yaml:"kline_interval"`
			} `yaml:"downtrend_detection"`
			CrashDetection struct {
				Enabled           bool    `yaml:"enabled"`
				KlineInterval     string  `yaml:"kline_interval"`
				ShortZoneMinMult  float64 `yaml:"short_zone_min_mult"`
				ShortZoneMaxMult  float64 `yaml:"short_zone_max_mult"`
				MaxShortPositions int     `yaml:"max_short_positions"`
				MAWindow          int     `yaml:"ma_window"`
				LongMAWindow      int     `yaml:"long_ma_window"`
				MinUptrendCandles int     `yaml:"min_uptrend_candles"`
				MildCrashRate     float64 `yaml:"mild_crash_rate"`
				SevereCrashRate   float64 `yaml:"severe_crash_rate"`
			} `yaml:"crash_detection"`
		}{
			Symbol:                "DOGEUSDT",
			PriceInterval:         0.0001,
			OrderQuantity:         10,
			MinOrderValue:         5,
			BuyWindowSize:         10,
			SellWindowSize:        10,
			ReconcileInterval:     60,
			OrderCleanupThreshold: 100,
			CleanupBatchSize:      10,
			MarginLockDurationSec: 10,
			PositionSafetyCheck:   100,
			MinMarginBalance:      5,
			DynamicGrid: struct {
				Enabled       bool    `yaml:"enabled"`
				ATRPeriod     int     `yaml:"atr_period"`
				ATRInterval   string  `yaml:"atr_interval"`
				ATRMultiplier float64 `yaml:"atr_multiplier"`
				MinProfitRate float64 `yaml:"min_profit_rate"`
			}{
				Enabled:       true,
				ATRPeriod:     14,
				ATRInterval:   "5m",
				ATRMultiplier: 0.8,
				MinProfitRate: 0.001,
			},
			DowntrendDetection: struct {
				Enabled              bool    `yaml:"enabled"`
				MAWindow             int     `yaml:"ma_window"`
				MildThreshold        float64 `yaml:"mild_threshold"`
				SevereThreshold      float64 `yaml:"severe_threshold"`
				ConsecutiveDownCount int     `yaml:"consecutive_down_count"`
				MildMultiplier       float64 `yaml:"mild_multiplier"`
				SevereMultiplier     float64 `yaml:"severe_multiplier"`
				SevereWindowRatio    float64 `yaml:"severe_window_ratio"`
				KlineInterval        string  `yaml:"kline_interval"`
			}{
				Enabled:              true,
				MAWindow:             20,
				MildThreshold:        0.98,
				SevereThreshold:      0.985,
				ConsecutiveDownCount: 6,
				MildMultiplier:       0.8,
				SevereMultiplier:     0.6,
				SevereWindowRatio:    0.3,
				KlineInterval:        "5m",
			},
			CrashDetection: struct {
				Enabled           bool    `yaml:"enabled"`
				KlineInterval     string  `yaml:"kline_interval"`
				ShortZoneMinMult  float64 `yaml:"short_zone_min_mult"`
				ShortZoneMaxMult  float64 `yaml:"short_zone_max_mult"`
				MaxShortPositions int     `yaml:"max_short_positions"`
				MAWindow          int     `yaml:"ma_window"`
				LongMAWindow      int     `yaml:"long_ma_window"`
				MinUptrendCandles int     `yaml:"min_uptrend_candles"`
				MildCrashRate     float64 `yaml:"mild_crash_rate"`
				SevereCrashRate   float64 `yaml:"severe_crash_rate"`
			}{
				Enabled:           false,
				KlineInterval:     "5m",
				ShortZoneMinMult:  1.2,
				ShortZoneMaxMult:  3.0,
				MaxShortPositions: 10,
				MAWindow:          20,
				LongMAWindow:      50,
				MinUptrendCandles: 5,
				MildCrashRate:     0.02,
				SevereCrashRate:   0.05,
			},
		},
		System: struct {
			LogLevel     string `yaml:"log_level"`
			CancelOnExit bool   `yaml:"cancel_on_exit"`
		}{
			LogLevel:     "INFO",
			CancelOnExit: true,
		},
		RiskControl: struct {
			Enabled           bool     `yaml:"enabled"`
			MonitorSymbols    []string `yaml:"monitor_symbols"`
			Interval          string   `yaml:"interval"`
			VolumeMultiplier  float64  `yaml:"volume_multiplier"`
			AverageWindow     int      `yaml:"average_window"`
			TriggerThreshold  int      `yaml:"trigger_threshold"`
			RecoveryThreshold int      `yaml:"recovery_threshold"`
		}{
			Enabled:           true,
			MonitorSymbols:    []string{"DOGEUSDT"},
			Interval:          "1m",
			VolumeMultiplier:  3.0,
			AverageWindow:     20,
			TriggerThreshold:  1,
			RecoveryThreshold: 1,
		},
		Timing: struct {
			WebSocketReconnectDelay    int `yaml:"websocket_reconnect_delay"`
			WebSocketWriteWait         int `yaml:"websocket_write_wait"`
			WebSocketPongWait          int `yaml:"websocket_pong_wait"`
			WebSocketPingInterval      int `yaml:"websocket_ping_interval"`
			ListenKeyKeepAliveInterval int `yaml:"listen_key_keepalive_interval"`
			PriceSendInterval          int `yaml:"price_send_interval"`
			RateLimitRetryDelay        int `yaml:"rate_limit_retry_delay"`
			OrderRetryDelay            int `yaml:"order_retry_delay"`
			PricePollInterval          int `yaml:"price_poll_interval"`
			StatusPrintInterval        int `yaml:"status_print_interval"`
			OrderCleanupInterval       int `yaml:"order_cleanup_interval"`
		}{
			WebSocketReconnectDelay:    5,
			WebSocketWriteWait:         10,
			WebSocketPongWait:          60,
			WebSocketPingInterval:      20,
			ListenKeyKeepAliveInterval: 30,
			PriceSendInterval:          50,
			RateLimitRetryDelay:        1,
			OrderRetryDelay:            500,
			PricePollInterval:          500,
			StatusPrintInterval:        1,
			OrderCleanupInterval:       60,
		},
	}

	// 创建模拟器
	simulator := simulation.NewSimulator(cfg)

	// 运行模拟
	if err := simulator.Run(*duration); err != nil {
		fmt.Printf("❌ 模拟运行失败: %v\n", err)
		os.Exit(1)
	}

	// 停止模拟
	simulator.Stop()

	fmt.Println("✅ 模拟完成！")
}