package simulation

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"opensqt/config"
	"opensqt/exchange"
	"opensqt/logger"
	"opensqt/monitor"
	"opensqt/order"
	"opensqt/position"
	"opensqt/safety"
	"sync"
	"time"
)

// MockExchange 模拟交易所
type MockExchange struct {
	symbol      string
	currentPrice float64
	priceHistory []*exchange.Candle
	mu          sync.RWMutex
	callbacks   map[string]func(interface{})
	klineStream map[string]chan *exchange.Candle
}

func NewMockExchange(symbol string, initialPrice float64) *MockExchange {
	return &MockExchange{
		symbol:       symbol,
		currentPrice: initialPrice,
		priceHistory: make([]*exchange.Candle, 0),
		callbacks:    make(map[string]func(interface{})),
		klineStream:  make(map[string]chan *exchange.Candle),
	}
}

func (m *MockExchange) GetName() string {
	return "mock_exchange"
}

func (m *MockExchange) GetPositions(ctx context.Context, symbol string) ([]*exchange.Position, error) {
	// 模拟持仓数据
	return []*exchange.Position{}, nil
}

func (m *MockExchange) GetOpenOrders(ctx context.Context, symbol string) ([]*exchange.Order, error) {
	// 模拟订单数据
	return []*exchange.Order{}, nil
}

func (m *MockExchange) GetOrder(ctx context.Context, symbol string, orderID int64) (*exchange.Order, error) {
	// 模拟订单详情
	return &exchange.Order{
		OrderID:   orderID,
		Symbol:    symbol,
		Side:      exchange.SideBuy,
		Type:      exchange.OrderTypeLimit,
		Price:     0.14,
		Quantity:  100,
		Status:    exchange.OrderStatusFilled,
		CreatedAt: time.Now(),
	}, nil
}

func (m *MockExchange) GetBaseAsset() string {
	return "DOGE"
}

func (m *MockExchange) CancelAllOrders(ctx context.Context, symbol string) error {
	return nil
}

func (m *MockExchange) GetAvailableBalance(ctx context.Context) (float64, error) {
	return 10000, nil // 模拟10000 USDT余额
}

func (m *MockExchange) GetHistoricalKlines(ctx context.Context, symbol, interval string, limit int) ([]*exchange.Candle, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 生成历史K线数据
	candles := make([]*exchange.Candle, 0, limit)
	startTime := time.Now().Add(time.Duration(-limit) * 5 * time.Minute).UnixMilli()

	for i := 0; i < limit; i++ {
		ts := startTime + int64(i)*5*60*1000
		price := m.currentPrice + (rand.Float64()-0.5)*0.01 // 小幅随机波动
		candle := &exchange.Candle{
			Timestamp: ts,
			Open:      price,
			High:      price + rand.Float64()*0.005,
			Low:       price - rand.Float64()*0.005,
			Close:     price + (rand.Float64()-0.5)*0.002,
			Volume:    1000 + rand.Float64()*1000,
			Symbol:    symbol,
			IsClosed:  true,
		}
		candles = append(candles, candle)
	}

	return candles, nil
}

func (m *MockExchange) StartKlineStream(ctx context.Context, symbols []string, interval string, callback exchange.CandleUpdateCallback) error {
	streamKey := fmt.Sprintf("%s_%s", symbols[0], interval)
	streamChan := make(chan *exchange.Candle, 100)
	m.klineStream[streamKey] = streamChan

	// 启动模拟K线推送
	go func() {
		ticker := time.NewTicker(5 * time.Second) // 每5秒推送一次
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.Lock()
				newPrice := m.currentPrice + (rand.Float64()-0.5)*0.001 // 微小波动
				m.currentPrice = newPrice
				m.mu.Unlock()

				candle := &exchange.Candle{
					Timestamp: time.Now().UnixMilli(),
					Open:      newPrice,
					High:      newPrice + rand.Float64()*0.0005,
					Low:       newPrice - rand.Float64()*0.0005,
					Close:     newPrice,
					Volume:    100 + rand.Float64()*200,
					Symbol:    m.symbol,
					IsClosed:  false,
				}

				select {
				case streamChan <- candle:
				default:
					// 如果通道满了就跳过
				}

				// 调用外部回调
				callback(candle)
			}
		}
	}()

	return nil
}

func (m *MockExchange) RegisterKlineCallback(componentName string, callback func(interface{})) error {
	m.callbacks[componentName] = callback
	return nil
}

func (m *MockExchange) ForceReconnectKlineStream() error {
	return nil
}

func (m *MockExchange) GetPriceDecimals() int {
	return 6 // 6 decimal places for DOGE
}

func (m *MockExchange) GetQuantityDecimals() int {
	return 4 // 4 decimal places for quantity
}

func (m *MockExchange) GetQuoteAsset() string {
	return "USDC"
}

func (m *MockExchange) GetLatestPrice(ctx context.Context, symbol string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentPrice, nil
}

func (m *MockExchange) StartPriceStream(ctx context.Context, symbol string, callback func(price float64)) error {
	// Start a goroutine to periodically push price updates
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.RLock()
				price := m.currentPrice
				m.mu.RUnlock()
				callback(price)
			}
		}
	}()
	return nil
}

func (m *MockExchange) StartOrderStream(ctx context.Context, callback func(interface{})) error {
	return nil
}

func (m *MockExchange) StopOrderStream() error {
	return nil
}

func (m *MockExchange) StopKlineStream() error {
	return nil
}

func (m *MockExchange) PlaceOrder(ctx context.Context, req *exchange.OrderRequest) (*exchange.Order, error) {
	return &exchange.Order{
		OrderID:       int64(rand.Intn(1000000)),
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		Type:          req.Type,
		Price:         req.Price,
		Quantity:      req.Quantity,
		Status:        exchange.OrderStatusNew,
		CreatedAt:     time.Now(),
	}, nil
}

func (m *MockExchange) BatchPlaceOrders(ctx context.Context, orders []*exchange.OrderRequest) ([]*exchange.Order, bool) {
	result := make([]*exchange.Order, 0, len(orders))
	for _, req := range orders {
		order := &exchange.Order{
			OrderID:       int64(rand.Intn(1000000)),
			ClientOrderID: req.ClientOrderID,
			Symbol:        req.Symbol,
			Side:          req.Side,
			Type:          req.Type,
			Price:         req.Price,
			Quantity:      req.Quantity,
			Status:        exchange.OrderStatusNew,
			CreatedAt:     time.Now(),
		}
		result = append(result, order)
	}
	return result, false
}

func (m *MockExchange) CancelOrder(ctx context.Context, symbol string, orderID int64) error {
	return nil
}

func (m *MockExchange) BatchCancelOrders(ctx context.Context, symbol string, orderIDs []int64) error {
	return nil
}

func (m *MockExchange) GetAccount(ctx context.Context) (*exchange.Account, error) {
	return &exchange.Account{
		TotalWalletBalance: 10000,
		TotalMarginBalance: 10000,
		AvailableBalance:   5000,
		Positions:          []*exchange.Position{},
	}, nil
}

func (m *MockExchange) GetBalance(ctx context.Context, asset string) (float64, error) {
	return 10000, nil
}

// Simulator 仿真器
type Simulator struct {
	config     *config.Config
	exchange   *MockExchange
	manager    *position.SuperPositionManager
	executor   *order.ExchangeOrderExecutor
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewSimulator 创建新的仿真器
func NewSimulator(cfg *config.Config) *Simulator {
	// 创建模拟交易所
	mockEx := NewMockExchange(cfg.Trading.Symbol, 0.14) // 使用DOGEUSDT的典型价格

	// 创建模拟订单执行器
	executor := &order.ExchangeOrderExecutor{}

	// 创建仓位管理器
	manager := position.NewSuperPositionManager(
		cfg,
		&exchangeExecutorAdapter{executor: executor},
		&positionExchangeAdapter{exchange: mockEx},
		6, // 价格精度
		4, // 数量精度
	)

	// 初始化动态网格计算器（如果启用）
	if cfg.Trading.DynamicGrid.Enabled {
		atrCalculator := monitor.NewATRCalculator(mockEx, cfg.Trading.Symbol, cfg.Trading.DynamicGrid.ATRInterval, cfg.Trading.DynamicGrid.ATRPeriod)
		dynamicGridCalc := monitor.NewDynamicGridCalculator(cfg, atrCalculator, 6)
		manager.SetATRCalculator(atrCalculator)
		manager.SetDynamicGridCalculator(dynamicGridCalc)
	}

	// 初始化阴跌检测器（如果启用）
	if cfg.Trading.DowntrendDetection.Enabled {
		detector := monitor.NewDowntrendDetector(cfg, mockEx, cfg.Trading.Symbol)
		manager.SetDowntrendDetector(detector)
	}

	// 初始化暴跌检测器（如果启用）
	if cfg.Trading.CrashDetection.Enabled {
		crashDetector := monitor.NewCrashDetector(cfg, mockEx, cfg.Trading.Symbol)
		manager.SetCrashDetector(crashDetector)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Simulator{
		config:     cfg,
		exchange:   mockEx,
		manager:    manager,
		executor:   executor,
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// exchangeExecutorAdapter 适配器
type exchangeExecutorAdapter struct {
	executor *order.ExchangeOrderExecutor
}

func (a *exchangeExecutorAdapter) PlaceOrder(req *position.OrderRequest) (*position.Order, error) {
	// 模拟下单
	return &position.Order{
		OrderID:       int64(rand.Intn(100000)),
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		Price:         req.Price,
		Quantity:      req.Quantity,
		Status:        "FILLED",
		ReduceOnly:    req.ReduceOnly,
	}, nil
}

func (a *exchangeExecutorAdapter) BatchPlaceOrders(orders []*position.OrderRequest) ([]*position.Order, bool) {
	result := make([]*position.Order, 0, len(orders))
	for _, req := range orders {
		order := &position.Order{
			OrderID:       int64(rand.Intn(100000)),
			ClientOrderID: req.ClientOrderID,
			Symbol:        req.Symbol,
			Side:          req.Side,
			Price:         req.Price,
			Quantity:      req.Quantity,
			Status:        "FILLED",
			ReduceOnly:    req.ReduceOnly,
		}
		result = append(result, order)
	}
	return result, false
}

func (a *exchangeExecutorAdapter) BatchCancelOrders(orderIDs []int64) error {
	return nil
}

// positionExchangeAdapter 适配器
type positionExchangeAdapter struct {
	exchange *MockExchange
}

func (a *positionExchangeAdapter) GetAvailableBalance(ctx context.Context) (float64, error) {
	return a.exchange.GetAvailableBalance(ctx)
}

func (a *positionExchangeAdapter) GetPositions(ctx context.Context, symbol string) (interface{}, error) {
	return a.exchange.GetPositions(ctx, symbol)
}

func (a *positionExchangeAdapter) CancelAllOrders(ctx context.Context, symbol string) error {
	return nil
}

func (a *positionExchangeAdapter) GetBaseAsset() string {
	return "DOGE"
}

func (a *positionExchangeAdapter) GetName() string {
	return "mock"
}

func (a *positionExchangeAdapter) GetOpenOrders(ctx context.Context, symbol string) (interface{}, error) {
	return []*exchange.Position{}, nil
}

func (a *positionExchangeAdapter) GetOrder(ctx context.Context, symbol string, orderID int64) (interface{}, error) {
	return map[string]interface{}{
		"orderId": orderID,
		"status":  "FILLED",
	}, nil
}

// Run 运行仿真
func (s *Simulator) Run(duration time.Duration) error {
	logger.Info("🚀 开始运行模拟交易系统...")

	// 执行安全检查
	currentPrice := s.exchange.currentPrice
	feeRate := 0.0002 // 模拟手续费率
	requiredPositions := int(math.Ceil(100.0 / currentPrice)) // 模拟所需持仓数

	if err := safety.CheckAccountSafety(
		s.exchange,
		s.config.Trading.Symbol,
		currentPrice,
		s.config.Trading.OrderQuantity,
		s.config.Trading.PriceInterval,
		feeRate,
		requiredPositions,
		6, // 价格精度
	); err != nil {
		logger.Warn("⚠️ 安全检查警告: %v", err)
	} else {
		logger.Info("✅ 安全检查通过")
	}

	// 启动阴跌检测器（如果启用）
	if s.config.Trading.DowntrendDetection.Enabled {
		if detector := s.manager.GetDowntrendDetector(); detector != nil {
			if err := detector.Start(s.ctx); err != nil {
				logger.Error("❌ 阴跌检测器启动失败: %v", err)
			} else {
				logger.Info("✅ 阴跌检测器已启动")
			}
		}
	}

	// 启动ATR计算器（如果启用动态网格）
	if s.config.Trading.DynamicGrid.Enabled {
		if atr := s.manager.GetATRCalculator(); atr != nil {
			if err := atr.Start(s.ctx); err != nil {
				logger.Error("❌ ATR计算器启动失败: %v", err)
			} else {
				logger.Info("✅ ATR计算器已启动")
			}
		}
	}

	// 启动主要的交易循环
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	endTime := time.Now().Add(duration)

	logger.Info("📊 模拟开始，持续时间: %v", duration)
	logger.Info("💡 当前价格: %.6f", currentPrice)

	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-ticker.C:
			if time.Now().After(endTime) {
				logger.Info("🏁 模拟结束")
				return nil
			}

			// 更新价格
			s.exchange.mu.Lock()
			newPrice := s.exchange.currentPrice + (rand.Float64()-0.5)*0.0005
			s.exchange.currentPrice = newPrice
			s.exchange.mu.Unlock()

			// 更新仓位管理器的市场价格
			s.manager.UpdateCurrentPrice(newPrice)

			// 执行一次交易逻辑
			if err := s.manager.HandleTradingLogic(newPrice); err != nil {
				logger.Error("❌ 交易逻辑错误: %v", err)
			}

			// 每10秒打印一次状态
			if time.Now().Second()%10 == 0 {
				logger.Info("📈 模拟价格: %.6f", newPrice)
				s.manager.PrintPositions()
			}
		}
	}
}

// Stop 停止仿真
func (s *Simulator) Stop() {
	logger.Info("🛑 停止模拟交易系统...")
	s.cancelFunc()
}

// GetManager 返回仓位管理器
func (s *Simulator) GetManager() *position.SuperPositionManager {
	return s.manager
}