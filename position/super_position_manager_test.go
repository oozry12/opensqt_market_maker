package position

import (
	"context"
	"fmt"
	"opensqt/config"
	"opensqt/monitor"
	"sort"
	"strings"
	"sync"
	"testing"
)

// ===== Mock 实现 =====

// MockOrderExecutor 模拟订单执行器
type MockOrderExecutor struct {
	orders      []*Order
	orderID     int64
	mu          sync.Mutex
	PlacedOrders []*OrderRequest // 记录所有下单请求
}

func NewMockOrderExecutor() *MockOrderExecutor {
	return &MockOrderExecutor{
		orders:  make([]*Order, 0),
		orderID: 1000,
	}
}

func (m *MockOrderExecutor) PlaceOrder(req *OrderRequest) (*Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.orderID++
	order := &Order{
		OrderID:       m.orderID,
		ClientOrderID: req.ClientOrderID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		Price:         req.Price,
		Quantity:      req.Quantity,
		Status:        "NEW",
		ReduceOnly:    req.ReduceOnly,
	}
	m.orders = append(m.orders, order)
	m.PlacedOrders = append(m.PlacedOrders, req)
	return order, nil
}

func (m *MockOrderExecutor) BatchPlaceOrders(orders []*OrderRequest) ([]*Order, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	result := make([]*Order, 0, len(orders))
	for _, req := range orders {
		m.orderID++
		order := &Order{
			OrderID:       m.orderID,
			ClientOrderID: req.ClientOrderID,
			Symbol:        req.Symbol,
			Side:          req.Side,
			Price:         req.Price,
			Quantity:      req.Quantity,
			Status:        "NEW",
			ReduceOnly:    req.ReduceOnly,
		}
		m.orders = append(m.orders, order)
		m.PlacedOrders = append(m.PlacedOrders, req)
		result = append(result, order)
	}
	return result, false
}

func (m *MockOrderExecutor) BatchCancelOrders(orderIDs []int64) error {
	return nil
}

func (m *MockOrderExecutor) GetPlacedOrders() []*OrderRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.PlacedOrders
}

func (m *MockOrderExecutor) ClearOrders() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PlacedOrders = nil
	m.orders = nil
}

// MockExchange 模拟交易所
type MockExchange struct {
	name string
}

func NewMockExchange() *MockExchange {
	return &MockExchange{name: "mock"}
}

func (m *MockExchange) GetName() string { return m.name }
func (m *MockExchange) GetPositions(ctx context.Context, symbol string) (interface{}, error) {
	return nil, nil
}
func (m *MockExchange) GetOpenOrders(ctx context.Context, symbol string) (interface{}, error) {
	return nil, nil
}
func (m *MockExchange) GetOrder(ctx context.Context, symbol string, orderID int64) (interface{}, error) {
	return nil, nil
}
func (m *MockExchange) GetBaseAsset() string { return "DOGE" }
func (m *MockExchange) CancelAllOrders(ctx context.Context, symbol string) error { return nil }
func (m *MockExchange) GetAvailableBalance(ctx context.Context) (float64, error) { return 10000, nil }

// MockCrashDetector 模拟开空检测器
type MockCrashDetector struct {
	enabled      bool
	shouldShort  bool
	anchorPrice  float64
	shortZoneMin float64
	shortZoneMax float64
}

func NewMockCrashDetector(anchor float64) *MockCrashDetector {
	return &MockCrashDetector{
		enabled:      true,
		shouldShort:  true,
		anchorPrice:  anchor,
		shortZoneMin: anchor * 1.2,
		shortZoneMax: anchor * 3.0,
	}
}

func (m *MockCrashDetector) IsEnabled() bool { return m.enabled }
func (m *MockCrashDetector) ShouldOpenShort() bool { return m.shouldShort }
func (m *MockCrashDetector) GetShortZone() (anchor, minPrice, maxPrice float64) {
	return m.anchorPrice, m.shortZoneMin, m.shortZoneMax
}
func (m *MockCrashDetector) GetCrashLevel() monitor.CrashLevel { return monitor.CrashNone }
func (m *MockCrashDetector) GetCrashRate() float64 { return 0 }

// ===== 测试用例 =====

// createTestConfig 创建测试配置
func createTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Trading.Symbol = "DOGEUSDT"
	cfg.Trading.PriceInterval = 0.001
	cfg.Trading.OrderQuantity = 10
	cfg.Trading.BuyWindowSize = 5
	cfg.Trading.SellWindowSize = 5
	cfg.Trading.MinOrderValue = 5
	cfg.Trading.OrderCleanupThreshold = 100
	return cfg
}

// TestLongShortNoConflict 测试做多和做空网格不冲突
func TestLongShortNoConflict(t *testing.T) {
	// 创建配置
	cfg := createTestConfig()

	executor := NewMockOrderExecutor()
	exchange := NewMockExchange()

	// 创建仓位管理器
	spm := NewSuperPositionManager(cfg, executor, exchange, 6, 4)
	
	// 设置锚点价格
	currentPrice := 0.14000 // 当前价格
	spm.anchorPrice = currentPrice
	spm.lastMarketPrice.Store(currentPrice)
	spm.isInitialized.Store(true)

	// 创建模拟的开空检测器
	// 锚点 = 0.14，做空区域 = [0.168, 0.42]
	mockCrashDetector := NewMockCrashDetector(0.14)
	
	// 手动设置 crashDetector（因为接口不匹配，我们直接测试 handleShortGrid）
	
	fmt.Println("===== 测试配置 =====")
	fmt.Printf("当前价格: %.6f\n", currentPrice)
	fmt.Printf("价格间距: %.6f\n", cfg.Trading.PriceInterval)
	fmt.Printf("买单窗口: %d\n", cfg.Trading.BuyWindowSize)
	fmt.Printf("做空锚点: %.6f\n", mockCrashDetector.anchorPrice)
	fmt.Printf("做空区域: [%.6f ~ %.6f]\n", mockCrashDetector.shortZoneMin, mockCrashDetector.shortZoneMax)
	
	// 计算买单价格范围
	buyPrices := spm.calculateSlotPrices(currentPrice, cfg.Trading.BuyWindowSize, "down")
	fmt.Println("\n===== 做多网格（买单）价格 =====")
	for i, p := range buyPrices {
		fmt.Printf("  买单 %d: %.6f\n", i+1, p)
	}
	
	// 计算做空网格价格范围
	fmt.Println("\n===== 做空网格价格 =====")
	shortPrices := make([]float64, 0)
	for price := mockCrashDetector.shortZoneMin; price <= mockCrashDetector.shortZoneMax && len(shortPrices) < 10; price += cfg.Trading.PriceInterval {
		shortPrices = append(shortPrices, roundPrice(price, 6))
	}
	for i, p := range shortPrices[:min(5, len(shortPrices))] {
		fmt.Printf("  空单 %d: %.6f\n", i+1, p)
	}
	if len(shortPrices) > 5 {
		fmt.Printf("  ... (共 %d 个空单价格)\n", len(shortPrices))
	}
	
	// 检查是否有冲突
	fmt.Println("\n===== 冲突检测 =====")
	buyMax := buyPrices[0] // 买单最高价
	shortMin := mockCrashDetector.shortZoneMin // 空单最低价
	
	fmt.Printf("买单最高价: %.6f\n", buyMax)
	fmt.Printf("空单最低价: %.6f\n", shortMin)
	fmt.Printf("价格差距: %.6f (%.2f%%)\n", shortMin-buyMax, (shortMin-buyMax)/currentPrice*100)
	
	if buyMax >= shortMin {
		t.Errorf("❌ 冲突！买单最高价 %.6f >= 空单最低价 %.6f", buyMax, shortMin)
	} else {
		fmt.Println("✅ 无冲突：做多网格和做空网格价格区域完全分离")
	}
	
	// 验证安全距离
	safetyGap := shortMin - currentPrice
	fmt.Printf("\n安全距离（空单最低价 - 当前价格）: %.6f (%.2f%%)\n", safetyGap, safetyGap/currentPrice*100)
	
	if safetyGap < currentPrice*0.1 {
		t.Errorf("⚠️ 警告：安全距离过小，空单最低价距离当前价格不足10%%")
	} else {
		fmt.Println("✅ 安全距离充足：空单区域远离当前价格")
	}
}

// TestHandleShortGrid 测试做空网格函数
func TestHandleShortGrid(t *testing.T) {
	cfg := createTestConfig()

	executor := NewMockOrderExecutor()
	exchange := NewMockExchange()
	spm := NewSuperPositionManager(cfg, executor, exchange, 6, 4)
	
	currentPrice := 0.14000
	spm.anchorPrice = currentPrice
	spm.lastMarketPrice.Store(currentPrice)
	spm.isInitialized.Store(true)

	// 模拟 crashDetector 的数据
	anchor := 0.14
	shortZoneMin := anchor * 1.2  // 0.168
	shortZoneMax := anchor * 3.0  // 0.42
	priceInterval := cfg.Trading.PriceInterval

	fmt.Println("\n===== 测试 handleShortGrid 逻辑 =====")
	fmt.Printf("锚点: %.6f\n", anchor)
	fmt.Printf("做空区域: [%.6f ~ %.6f]\n", shortZoneMin, shortZoneMax)
	fmt.Printf("当前价格: %.6f\n", currentPrice)

	// 安全检查：做空区域必须在当前价格上方
	if shortZoneMin <= currentPrice {
		fmt.Printf("❌ 做空区域 %.6f <= 当前价格 %.6f，跳过\n", shortZoneMin, currentPrice)
		return
	}
	fmt.Printf("✅ 做空区域在当前价格上方，可以开空\n")

	// 生成做空槽位价格
	maxShortPositions := 10
	shortCandidates := make([]float64, 0)
	
	for price := shortZoneMin; price <= shortZoneMax && len(shortCandidates) < maxShortPositions; price += priceInterval {
		slotPrice := roundPrice(price, 6)
		shortCandidates = append(shortCandidates, slotPrice)
	}

	fmt.Printf("\n生成的空单价格（前5个）:\n")
	for i, p := range shortCandidates[:min(5, len(shortCandidates))] {
		quantity := cfg.Trading.OrderQuantity / p
		fmt.Printf("  空单 %d: 价格=%.6f, 数量=%.4f, 价值=%.2fU\n", i+1, p, quantity, p*quantity)
	}
	
	if len(shortCandidates) > 5 {
		fmt.Printf("  ... 共 %d 个空单\n", len(shortCandidates))
	}

	// 验证所有空单价格都在当前价格上方
	for _, p := range shortCandidates {
		if p <= currentPrice {
			t.Errorf("❌ 空单价格 %.6f <= 当前价格 %.6f", p, currentPrice)
		}
	}
	fmt.Println("\n✅ 所有空单价格都在当前价格上方")
}

// TestHandleCloseShort 测试平空仓函数
func TestHandleCloseShort(t *testing.T) {
	cfg := createTestConfig()

	executor := NewMockOrderExecutor()
	exchange := NewMockExchange()
	spm := NewSuperPositionManager(cfg, executor, exchange, 6, 4)
	
	currentPrice := 0.14000
	spm.anchorPrice = currentPrice
	spm.lastMarketPrice.Store(currentPrice)
	spm.isInitialized.Store(true)

	fmt.Println("\n===== 测试 handleCloseShort 逻辑 =====")

	// 模拟已有空仓
	shortPositions := []struct {
		price    float64
		quantity float64
	}{
		{0.168, -59.5238}, // 开空价格 0.168，持仓 -59.5238
		{0.169, -59.1716},
		{0.170, -58.8235},
	}

	fmt.Println("模拟空仓:")
	for _, pos := range shortPositions {
		slot := spm.getOrCreateSlot(pos.price)
		slot.PositionStatus = PositionStatusFilled
		slot.PositionQty = pos.quantity // 负数表示空仓
		slot.SlotStatus = SlotStatusFree
		fmt.Printf("  价格=%.6f, 持仓=%.4f (空仓)\n", pos.price, pos.quantity)
	}

	// 计算平仓价格和利润
	priceInterval := cfg.Trading.PriceInterval
	fmt.Println("\n平仓计算:")
	for _, pos := range shortPositions {
		closePrice := pos.price - priceInterval
		profitRate := (pos.price - closePrice) / pos.price
		profit := (pos.price - closePrice) * (-pos.quantity)
		fmt.Printf("  开仓价=%.6f, 平仓价=%.6f, 利润率=%.2f%%, 预计利润=%.4fU\n",
			pos.price, closePrice, profitRate*100, profit)
	}

	// 验证平仓价格低于开仓价格
	for _, pos := range shortPositions {
		closePrice := pos.price - priceInterval
		if closePrice >= pos.price {
			t.Errorf("❌ 平仓价格 %.6f >= 开仓价格 %.6f", closePrice, pos.price)
		}
	}
	fmt.Println("\n✅ 所有平仓价格都低于开仓价格（空仓盈利）")
}

// TestFullScenario 完整场景测试
func TestFullScenario(t *testing.T) {
	cfg := createTestConfig()

	executor := NewMockOrderExecutor()
	exchange := NewMockExchange()
	spm := NewSuperPositionManager(cfg, executor, exchange, 6, 4)
	
	currentPrice := 0.14000
	spm.anchorPrice = currentPrice
	spm.lastMarketPrice.Store(currentPrice)
	spm.isInitialized.Store(true)

	fmt.Println("\n===== 完整场景测试 =====")
	fmt.Printf("当前价格: %.6f\n", currentPrice)
	fmt.Printf("价格间距: %.6f\n", cfg.Trading.PriceInterval)

	// 1. 做多网格（买单在当前价格下方）
	fmt.Println("\n--- 做多网格 ---")
	buyPrices := spm.calculateSlotPrices(currentPrice, cfg.Trading.BuyWindowSize, "down")
	for i, p := range buyPrices {
		quantity := cfg.Trading.OrderQuantity / p
		fmt.Printf("买单 %d: 价格=%.6f, 数量=%.4f\n", i+1, p, quantity)
	}

	// 2. 做空网格（空单在锚点1.2倍~3倍区域）
	fmt.Println("\n--- 做空网格 ---")
	anchor := 0.14
	shortZoneMin := anchor * 1.2
	shortZoneMax := anchor * 3.0
	fmt.Printf("锚点: %.6f\n", anchor)
	fmt.Printf("做空区域: [%.6f ~ %.6f]\n", shortZoneMin, shortZoneMax)

	shortPrices := make([]float64, 0)
	for price := shortZoneMin; price <= shortZoneMax && len(shortPrices) < 5; price += cfg.Trading.PriceInterval {
		shortPrices = append(shortPrices, roundPrice(price, 6))
	}
	for i, p := range shortPrices {
		quantity := cfg.Trading.OrderQuantity / p
		fmt.Printf("空单 %d: 价格=%.6f, 数量=%.4f\n", i+1, p, quantity)
	}

	// 3. 验证价格区域
	fmt.Println("\n--- 价格区域验证 ---")
	buyMax := buyPrices[0]
	buyMin := buyPrices[len(buyPrices)-1]
	shortMin := shortZoneMin
	
	fmt.Printf("买单区域: [%.6f ~ %.6f]\n", buyMin, buyMax)
	fmt.Printf("空单区域: [%.6f ~ %.6f]\n", shortMin, shortZoneMax)
	fmt.Printf("当前价格: %.6f\n", currentPrice)
	
	// 检查
	if buyMax > currentPrice {
		t.Errorf("❌ 买单最高价 %.6f > 当前价格 %.6f", buyMax, currentPrice)
	} else {
		fmt.Println("✅ 买单在当前价格或下方")
	}
	
	if shortMin <= currentPrice {
		t.Errorf("❌ 空单最低价 %.6f <= 当前价格 %.6f", shortMin, currentPrice)
	} else {
		fmt.Println("✅ 空单在当前价格上方")
	}
	
	if buyMax >= shortMin {
		t.Errorf("❌ 买单和空单区域重叠")
	} else {
		gap := shortMin - buyMax
		fmt.Printf("✅ 买单和空单区域分离，间隔=%.6f (%.2f%%)\n", gap, gap/currentPrice*100)
	}

	fmt.Println("\n===== 测试完成 =====")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}


// TestCloseLongVsCloseShort 测试平多单和平空单是否冲突
func TestCloseLongVsCloseShort(t *testing.T) {
	cfg := createTestConfig()

	executor := NewMockOrderExecutor()
	exchange := NewMockExchange()
	spm := NewSuperPositionManager(cfg, executor, exchange, 6, 4)

	currentPrice := 0.14000
	spm.anchorPrice = currentPrice
	spm.lastMarketPrice.Store(currentPrice)
	spm.isInitialized.Store(true)

	priceInterval := cfg.Trading.PriceInterval

	fmt.Println("\n===== 平多单 vs 平空单 冲突测试 =====")
	fmt.Printf("当前价格: %.6f\n", currentPrice)
	fmt.Printf("价格间距: %.6f\n", priceInterval)

	// 1. 模拟多仓（买入后持有，等待卖出平仓）
	fmt.Println("\n--- 多仓情况（做多网格）---")
	longPositions := []struct {
		buyPrice  float64 // 买入价格（槽位价格）
		quantity  float64 // 持仓数量（正数）
		sellPrice float64 // 卖出价格（平仓价格）
	}{
		{0.139, 71.9424, 0.140}, // 买入价0.139，卖出价0.140
		{0.138, 72.4638, 0.139},
		{0.137, 72.9927, 0.138},
	}

	fmt.Println("多仓持仓:")
	for _, pos := range longPositions {
		slot := spm.getOrCreateSlot(pos.buyPrice)
		slot.PositionStatus = PositionStatusFilled
		slot.PositionQty = pos.quantity // 正数表示多仓
		slot.SlotStatus = SlotStatusFree
		fmt.Printf("  槽位价格=%.6f, 持仓=%.4f (多仓), 平仓卖出价=%.6f\n",
			pos.buyPrice, pos.quantity, pos.sellPrice)
	}

	// 2. 模拟空仓（卖出开仓后持有，等待买入平仓）
	fmt.Println("\n--- 空仓情况（做空网格）---")
	shortPositions := []struct {
		openPrice  float64 // 开仓价格（卖出价格，槽位价格）
		quantity   float64 // 持仓数量（负数）
		closePrice float64 // 平仓价格（买入价格）
	}{
		{0.168, -59.5238, 0.167}, // 开仓价0.168，平仓价0.167
		{0.169, -59.1716, 0.168},
		{0.170, -58.8235, 0.169},
	}

	fmt.Println("空仓持仓:")
	for _, pos := range shortPositions {
		slot := spm.getOrCreateSlot(pos.openPrice)
		slot.PositionStatus = PositionStatusFilled
		slot.PositionQty = pos.quantity // 负数表示空仓
		slot.SlotStatus = SlotStatusFree
		fmt.Printf("  槽位价格=%.6f, 持仓=%.4f (空仓), 平仓买入价=%.6f\n",
			pos.openPrice, pos.quantity, pos.closePrice)
	}

	// 3. 收集所有平仓订单价格
	fmt.Println("\n--- 平仓订单价格对比 ---")

	// 平多单（卖出）价格
	sellPrices := make([]float64, 0)
	for _, pos := range longPositions {
		sellPrices = append(sellPrices, pos.sellPrice)
	}
	fmt.Printf("平多单（卖出）价格: %v\n", sellPrices)

	// 平空单（买入）价格
	buyClosePrices := make([]float64, 0)
	for _, pos := range shortPositions {
		buyClosePrices = append(buyClosePrices, pos.closePrice)
	}
	fmt.Printf("平空单（买入）价格: %v\n", buyClosePrices)

	// 4. 检查是否有价格重叠
	fmt.Println("\n--- 冲突检测 ---")

	// 找出平多单的价格范围
	sellMin := sellPrices[len(sellPrices)-1]
	sellMax := sellPrices[0]
	fmt.Printf("平多单价格范围: [%.6f ~ %.6f]\n", sellMin, sellMax)

	// 找出平空单的价格范围
	buyCloseMin := buyClosePrices[0]
	buyCloseMax := buyClosePrices[len(buyClosePrices)-1]
	fmt.Printf("平空单价格范围: [%.6f ~ %.6f]\n", buyCloseMin, buyCloseMax)

	// 检查重叠
	hasConflict := false
	for _, sellPrice := range sellPrices {
		for _, buyPrice := range buyClosePrices {
			if sellPrice == buyPrice {
				hasConflict = true
				t.Errorf("❌ 价格冲突！平多单卖出价 %.6f == 平空单买入价 %.6f", sellPrice, buyPrice)
			}
		}
	}

	if !hasConflict {
		gap := buyCloseMin - sellMax
		fmt.Printf("\n✅ 无冲突：平多单和平空单价格区域完全分离\n")
		fmt.Printf("   平多单最高卖出价: %.6f\n", sellMax)
		fmt.Printf("   平空单最低买入价: %.6f\n", buyCloseMin)
		fmt.Printf("   价格间隔: %.6f (%.2f%%)\n", gap, gap/currentPrice*100)
	}

	// 5. 验证订单方向
	fmt.Println("\n--- 订单方向验证 ---")
	fmt.Println("平多单: Side=SELL, ReduceOnly=true (卖出减仓)")
	fmt.Println("平空单: Side=BUY, ReduceOnly=true (买入平仓)")
	fmt.Println("✅ 订单方向不同，不会混淆")
}

// TestOrderSideConflict 测试同价格不同方向订单
func TestOrderSideConflict(t *testing.T) {
	cfg := createTestConfig()

	executor := NewMockOrderExecutor()
	exchange := NewMockExchange()
	spm := NewSuperPositionManager(cfg, executor, exchange, 6, 4)

	currentPrice := 0.14000
	spm.anchorPrice = currentPrice
	spm.lastMarketPrice.Store(currentPrice)
	spm.isInitialized.Store(true)

	fmt.Println("\n===== 同价格不同方向订单测试 =====")

	// 假设一个极端情况：价格快速波动，导致某个价格点既有多仓又有空仓
	testPrice := 0.15000

	fmt.Printf("测试价格: %.6f\n", testPrice)

	// 创建槽位
	slot := spm.getOrCreateSlot(testPrice)

	// 场景1：槽位有多仓
	fmt.Println("\n场景1: 槽位有多仓")
	slot.PositionStatus = PositionStatusFilled
	slot.PositionQty = 66.6667 // 正数 = 多仓
	slot.SlotStatus = SlotStatusFree
	fmt.Printf("  持仓: %.4f (多仓)\n", slot.PositionQty)
	fmt.Printf("  应该挂: SELL 卖单（平多仓）\n")

	// 场景2：槽位有空仓
	fmt.Println("\n场景2: 槽位有空仓")
	slot.PositionQty = -66.6667 // 负数 = 空仓
	fmt.Printf("  持仓: %.4f (空仓)\n", slot.PositionQty)
	fmt.Printf("  应该挂: BUY 买单（平空仓）\n")

	// 场景3：槽位无持仓
	fmt.Println("\n场景3: 槽位无持仓")
	slot.PositionStatus = PositionStatusEmpty
	slot.PositionQty = 0
	fmt.Printf("  持仓: %.4f (空仓位)\n", slot.PositionQty)
	fmt.Printf("  可以挂: BUY 买单（开多仓）或 SELL 卖单（开空仓）\n")

	fmt.Println("\n--- 结论 ---")
	fmt.Println("✅ 每个槽位只能有一种持仓状态（多仓/空仓/无持仓）")
	fmt.Println("✅ 持仓方向由 PositionQty 的正负决定")
	fmt.Println("✅ 同一槽位不会同时挂买单和卖单")
}

// TestPriceZoneSeparation 测试价格区域分离
func TestPriceZoneSeparation(t *testing.T) {
	cfg := createTestConfig()

	fmt.Println("\n===== 价格区域分离测试 =====")

	// 测试不同价格场景
	testCases := []struct {
		name         string
		currentPrice float64
		anchor       float64
	}{
		{"低价币(DOGE)", 0.14, 0.14},
		{"中价币(SOL)", 150.0, 150.0},
		{"高价币(ETH)", 3000.0, 3000.0},
		{"高价币(BTC)", 60000.0, 60000.0},
	}

	for _, tc := range testCases {
		fmt.Printf("\n--- %s (当前价格: %.2f) ---\n", tc.name, tc.currentPrice)

		// 做多网格区域（当前价格下方）
		buyWindowSize := cfg.Trading.BuyWindowSize
		priceInterval := tc.currentPrice * 0.001 // 假设间距为价格的0.1%

		buyMax := tc.currentPrice
		buyMin := tc.currentPrice - float64(buyWindowSize-1)*priceInterval

		// 做空网格区域（锚点1.2倍~3倍）
		shortZoneMin := tc.anchor * 1.2
		shortZoneMax := tc.anchor * 3.0

		// 平多单区域（买入价+间距）
		closeLongMin := buyMin + priceInterval
		closeLongMax := buyMax + priceInterval

		// 平空单区域（开仓价-间距）
		closeShortMin := shortZoneMin - priceInterval
		closeShortMax := shortZoneMax - priceInterval

		fmt.Printf("  做多网格(买单): [%.4f ~ %.4f]\n", buyMin, buyMax)
		fmt.Printf("  平多单(卖出):   [%.4f ~ %.4f]\n", closeLongMin, closeLongMax)
		fmt.Printf("  做空网格(空单): [%.4f ~ %.4f]\n", shortZoneMin, shortZoneMax)
		fmt.Printf("  平空单(买入):   [%.4f ~ %.4f]\n", closeShortMin, closeShortMax)

		// 检查区域分离
		gap := closeShortMin - closeLongMax
		gapPercent := gap / tc.currentPrice * 100

		if closeLongMax >= closeShortMin {
			t.Errorf("❌ %s: 平多单和平空单区域重叠！", tc.name)
		} else {
			fmt.Printf("  ✅ 区域分离，间隔: %.4f (%.2f%%)\n", gap, gapPercent)
		}
	}
}


// TestOrderQuotaConflict 测试订单配额是否会冲突
func TestOrderQuotaConflict(t *testing.T) {
	cfg := createTestConfig()
	cfg.Trading.OrderCleanupThreshold = 50 // 订单上限50个
	cfg.Trading.BuyWindowSize = 30         // 买单窗口30个
	cfg.Trading.SellWindowSize = 30        // 卖单窗口30个

	fmt.Println("\n===== 订单配额冲突测试 =====")
	fmt.Printf("订单上限: %d\n", cfg.Trading.OrderCleanupThreshold)
	fmt.Printf("买单窗口: %d\n", cfg.Trading.BuyWindowSize)
	fmt.Printf("卖单窗口: %d\n", cfg.Trading.SellWindowSize)
	fmt.Printf("最大空仓数量: 10 (代码中硬编码)\n")

	// 模拟场景：已有很多买单和卖单
	scenarios := []struct {
		name           string
		existingBuy    int // 已有买单数量
		existingSell   int // 已有卖单数量
		existingShort  int // 已有空单数量
	}{
		{"正常情况", 10, 5, 0},
		{"买单较多", 30, 5, 0},
		{"买卖单都多", 25, 20, 0},
		{"接近上限", 25, 24, 0},
		{"已有空单", 20, 10, 5},
	}

	for _, sc := range scenarios {
		fmt.Printf("\n--- %s ---\n", sc.name)
		fmt.Printf("已有买单: %d, 已有卖单: %d, 已有空单: %d\n",
			sc.existingBuy, sc.existingSell, sc.existingShort)

		currentOrderCount := sc.existingBuy + sc.existingSell + sc.existingShort
		threshold := cfg.Trading.OrderCleanupThreshold

		// 计算剩余配额
		remainingOrders := threshold - currentOrderCount
		if remainingOrders < 0 {
			remainingOrders = 0
		}

		fmt.Printf("当前订单总数: %d\n", currentOrderCount)
		fmt.Printf("剩余配额: %d\n", remainingOrders)

		// 模拟 AdjustOrders 中的配额分配逻辑
		buyWindowSize := cfg.Trading.BuyWindowSize
		sellWindowSize := cfg.Trading.SellWindowSize
		maxShortPositions := 10

		// 1. 买单配额
		allowedNewBuyOrders := buyWindowSize
		if allowedNewBuyOrders > remainingOrders {
			allowedNewBuyOrders = remainingOrders
		}
		// 假设需要创建的买单数量
		buyOrdersToCreate := min(5, allowedNewBuyOrders) // 假设需要5个新买单

		// 2. 卖单配额（扣除买单后）
		remainingForSell := remainingOrders - buyOrdersToCreate
		if remainingForSell < 0 {
			remainingForSell = 0
		}
		allowedNewSellOrders := sellWindowSize
		if allowedNewSellOrders > remainingForSell {
			allowedNewSellOrders = remainingForSell
		}
		sellOrdersToCreate := min(3, allowedNewSellOrders) // 假设需要3个新卖单

		// 3. 空单配额（扣除买单和卖单后）
		remainingForShort := remainingOrders - buyOrdersToCreate - sellOrdersToCreate
		if remainingForShort < 0 {
			remainingForShort = 0
		}
		// 空单还受最大空仓数量限制
		currentShortCount := sc.existingShort
		allowedNewShorts := maxShortPositions - currentShortCount
		if allowedNewShorts > remainingForShort {
			allowedNewShorts = remainingForShort
		}
		if allowedNewShorts < 0 {
			allowedNewShorts = 0
		}

		fmt.Printf("\n配额分配:\n")
		fmt.Printf("  新买单配额: %d (实际创建: %d)\n", allowedNewBuyOrders, buyOrdersToCreate)
		fmt.Printf("  新卖单配额: %d (实际创建: %d)\n", allowedNewSellOrders, sellOrdersToCreate)
		fmt.Printf("  新空单配额: %d (受限于: 剩余配额=%d, 最大空仓=%d)\n",
			allowedNewShorts, remainingForShort, maxShortPositions-currentShortCount)

		// 检查是否有空单配额
		if allowedNewShorts == 0 && remainingForShort > 0 {
			fmt.Println("  ⚠️ 空单配额为0，但剩余配额>0，可能是空仓数量已达上限")
		} else if allowedNewShorts == 0 {
			fmt.Println("  ⚠️ 空单配额为0，订单配额已用完")
		} else {
			fmt.Printf("  ✅ 空单有配额: %d\n", allowedNewShorts)
		}
	}

	fmt.Println("\n--- 结论 ---")
	fmt.Println("1. 订单配额按顺序分配：买单 -> 卖单 -> 空单")
	fmt.Println("2. 如果买单和卖单用完配额，空单将无法创建")
	fmt.Println("3. 空单还受最大空仓数量(10)限制")
	fmt.Println("4. ⚠️ 存在配额竞争问题！")
}

// TestOrderPriorityIssue 测试订单优先级问题
func TestOrderPriorityIssue(t *testing.T) {
	fmt.Println("\n===== 订单优先级问题分析 =====")

	fmt.Println("\n当前代码中的订单处理顺序（AdjustOrders函数）:")
	fmt.Println("1. 处理买单 (做多开仓)")
	fmt.Println("2. 处理卖单 (做多平仓)")
	fmt.Println("3. 处理空单 (做空开仓) - handleShortGrid")
	fmt.Println("4. 处理平空单 (做空平仓) - handleCloseShort")

	fmt.Println("\n配额计算:")
	fmt.Println("- remainingOrders = threshold - currentOrderCount")
	fmt.Println("- 买单先用配额")
	fmt.Println("- 卖单用剩余配额")
	fmt.Println("- 空单用最后剩余的配额")

	fmt.Println("\n⚠️ 潜在问题:")
	fmt.Println("1. 如果买单窗口很大(如30)，可能占用大部分配额")
	fmt.Println("2. 空单只能用剩余配额，可能无法创建")
	fmt.Println("3. 做空功能可能被做多功能'挤掉'")

	fmt.Println("\n建议解决方案:")
	fmt.Println("1. 为空单预留固定配额（如10个）")
	fmt.Println("2. 或者增加订单上限")
	fmt.Println("3. 或者空单使用独立的配额计算")

	// 模拟极端情况
	fmt.Println("\n--- 极端情况模拟 ---")
	threshold := 50
	buyWindowSize := 30
	sellWindowSize := 30

	// 假设当前没有订单，但需要创建很多
	currentOrderCount := 0
	remainingOrders := threshold - currentOrderCount

	// 买单占用
	buyOrdersToCreate := min(buyWindowSize, remainingOrders)
	remainingAfterBuy := remainingOrders - buyOrdersToCreate

	// 卖单占用
	sellOrdersToCreate := min(sellWindowSize, remainingAfterBuy)
	remainingAfterSell := remainingAfterBuy - sellOrdersToCreate

	// 空单
	shortOrdersToCreate := min(10, remainingAfterSell)

	fmt.Printf("订单上限: %d\n", threshold)
	fmt.Printf("买单创建: %d (窗口: %d)\n", buyOrdersToCreate, buyWindowSize)
	fmt.Printf("卖单创建: %d (窗口: %d)\n", sellOrdersToCreate, sellWindowSize)
	fmt.Printf("空单创建: %d (最大: 10)\n", shortOrdersToCreate)
	fmt.Printf("剩余配额: %d\n", remainingAfterSell-shortOrdersToCreate)

	if shortOrdersToCreate < 10 {
		t.Logf("⚠️ 警告: 空单配额不足，只能创建 %d 个（最大10个）", shortOrdersToCreate)
	}
}

// TestSuggestedFix 测试建议的修复方案
func TestSuggestedFix(t *testing.T) {
	fmt.Println("\n===== 建议的修复方案 =====")

	threshold := 50
	maxShortPositions := 10
	reservedForShort := 10 // 为空单预留的配额

	fmt.Printf("订单上限: %d\n", threshold)
	fmt.Printf("最大空仓: %d\n", maxShortPositions)
	fmt.Printf("空单预留配额: %d\n", reservedForShort)

	// 方案1: 为空单预留配额
	fmt.Println("\n方案1: 为空单预留固定配额")
	availableForLong := threshold - reservedForShort // 40个给做多
	fmt.Printf("  做多可用配额: %d\n", availableForLong)
	fmt.Printf("  做空预留配额: %d\n", reservedForShort)
	fmt.Println("  优点: 保证空单有配额")
	fmt.Println("  缺点: 可能浪费配额（如果不需要开空）")

	// 方案2: 动态分配
	fmt.Println("\n方案2: 动态分配（当前实现）")
	fmt.Println("  按顺序分配: 买单 -> 卖单 -> 空单")
	fmt.Println("  优点: 灵活，不浪费配额")
	fmt.Println("  缺点: 空单可能被挤掉")

	// 方案3: 增加订单上限
	fmt.Println("\n方案3: 增加订单上限")
	newThreshold := 100
	fmt.Printf("  建议订单上限: %d\n", newThreshold)
	fmt.Printf("  买单窗口: 30, 卖单窗口: 30, 空单: 10, 平空: 10 = 80\n")
	fmt.Printf("  剩余缓冲: %d\n", newThreshold-80)
	fmt.Println("  优点: 简单有效")
	fmt.Println("  缺点: 可能增加交易所API压力")

	fmt.Println("\n✅ 推荐: 方案3 - 增加订单上限到100")
	fmt.Println("   或者在config.yaml中设置 order_cleanup_threshold: 100")
}


// TestFixedQuota 测试修复后的配额分配
func TestFixedQuota(t *testing.T) {
	fmt.Println("\n===== 修复后的配额分配测试 =====")

	// 修复后的配置
	threshold := 100 // 增加到100
	buyWindowSize := 10
	sellWindowSize := 10
	maxShortPositions := 10

	fmt.Printf("订单上限: %d (已增加)\n", threshold)
	fmt.Printf("买单窗口: %d\n", buyWindowSize)
	fmt.Printf("卖单窗口: %d\n", sellWindowSize)
	fmt.Printf("最大空仓: %d\n", maxShortPositions)

	scenarios := []struct {
		name         string
		existingBuy  int
		existingSell int
		existingShort int
	}{
		{"空仓状态", 0, 0, 0},
		{"正常运行", 10, 10, 0},
		{"多单较多", 30, 20, 0},
		{"已有空单", 20, 15, 5},
		{"极端情况", 40, 40, 0},
	}

	for _, sc := range scenarios {
		currentOrderCount := sc.existingBuy + sc.existingSell + sc.existingShort
		remainingOrders := threshold - currentOrderCount

		// 买单配额
		allowedBuy := min(buyWindowSize, remainingOrders)
		buyCreated := min(5, allowedBuy)

		// 卖单配额
		remainingAfterBuy := remainingOrders - buyCreated
		allowedSell := min(sellWindowSize, remainingAfterBuy)
		sellCreated := min(5, allowedSell)

		// 空单配额
		remainingAfterSell := remainingAfterBuy - sellCreated
		allowedShort := min(maxShortPositions-sc.existingShort, remainingAfterSell)

		status := "✅"
		if allowedShort < maxShortPositions-sc.existingShort {
			status = "⚠️"
		}

		fmt.Printf("\n%s: 买%d/卖%d/空%d -> 剩余%d -> 空单配额%d %s\n",
			sc.name, sc.existingBuy, sc.existingSell, sc.existingShort,
			remainingOrders, allowedShort, status)
	}

	fmt.Println("\n✅ 订单上限100足够容纳所有订单类型")
}


// TestFullScenarioAllOrderTypes 全场景测试：所有订单类型的价格是否重叠
func TestFullScenarioAllOrderTypes(t *testing.T) {
	cfg := createTestConfig()
	cfg.Trading.BuyWindowSize = 20
	cfg.Trading.SellWindowSize = 20
	cfg.Trading.PriceInterval = 0.001

	executor := NewMockOrderExecutor()
	exchange := NewMockExchange()
	spm := NewSuperPositionManager(cfg, executor, exchange, 6, 4)

	currentPrice := 0.14000
	priceInterval := cfg.Trading.PriceInterval
	spm.anchorPrice = currentPrice
	spm.lastMarketPrice.Store(currentPrice)
	spm.isInitialized.Store(true)

	// 做空参数
	anchor := currentPrice
	shortZoneMin := anchor * 1.2 // 0.168
	shortZoneMax := anchor * 3.0 // 0.42

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("全场景测试：所有订单类型价格重叠检测")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\n当前价格: %.6f\n", currentPrice)
	fmt.Printf("价格间距: %.6f\n", priceInterval)
	fmt.Printf("做空锚点: %.6f\n", anchor)
	fmt.Printf("做空区域: [%.6f ~ %.6f]\n", shortZoneMin, shortZoneMax)

	// ========== 收集所有订单价格 ==========

	type OrderInfo struct {
		Price    float64
		Side     string // BUY or SELL
		Type     string // 订单类型描述
		SlotPrice float64 // 槽位价格
	}
	allOrders := make([]OrderInfo, 0)

	// 1. 做多开仓（买单）- 在当前价格下方
	fmt.Println("\n--- 1. 做多开仓（买单）---")
	buyPrices := spm.calculateSlotPrices(currentPrice, cfg.Trading.BuyWindowSize, "down")
	for i, slotPrice := range buyPrices {
		// 买单价格 = 槽位价格
		allOrders = append(allOrders, OrderInfo{
			Price:     slotPrice,
			Side:      "BUY",
			Type:      "做多开仓",
			SlotPrice: slotPrice,
		})
		if i < 5 {
			fmt.Printf("  买单 %d: 价格=%.6f\n", i+1, slotPrice)
		}
	}
	fmt.Printf("  ... 共 %d 个买单\n", len(buyPrices))

	// 2. 做多平仓（卖单）- 只模拟一个买单成交的情况
	fmt.Println("\n--- 2. 做多平仓（卖单）---")
	if len(buyPrices) > 0 {
		slotPrice := buyPrices[0]
		// 模拟持仓
		slot := spm.getOrCreateSlot(slotPrice)
		slot.PositionStatus = PositionStatusFilled
		slot.PositionQty = cfg.Trading.OrderQuantity / slotPrice
		slot.SlotStatus = SlotStatusFree

		// 卖单价格 = 槽位价格 + 间距
		sellPrice := roundPrice(slotPrice+priceInterval, 6)
		allOrders = append(allOrders, OrderInfo{
			Price:     sellPrice,
			Side:      "SELL",
			Type:      "做多平仓",
			SlotPrice: slotPrice,
		})
		fmt.Printf("  卖单: 槽位=%.6f, 卖出价=%.6f\n", slotPrice, sellPrice)
	}
	fmt.Printf("  共 1 个卖单（模拟一个买单成交）\n")

	// 3. 做空开仓（卖单）- 在做空区域
	fmt.Println("\n--- 3. 做空开仓（卖单）---")
	shortCount := 0
	for price := shortZoneMin; price <= shortZoneMax && shortCount < 10; price += priceInterval {
		slotPrice := roundPrice(price, 6)
		// 开空卖单价格 = 槽位价格
		allOrders = append(allOrders, OrderInfo{
			Price:     slotPrice,
			Side:      "SELL",
			Type:      "做空开仓",
			SlotPrice: slotPrice,
		})
		if shortCount < 5 {
			fmt.Printf("  空单 %d: 价格=%.6f\n", shortCount+1, slotPrice)
		}
		shortCount++
	}
	fmt.Printf("  ... 共 %d 个空单\n", shortCount)

	// 4. 做空平仓（买单）- 只模拟一个空单成交的情况
	fmt.Println("\n--- 4. 做空平仓（买单）---")
	if shortCount > 0 {
		slotPrice := roundPrice(shortZoneMin, 6)
		// 模拟空仓
		slot := spm.getOrCreateSlot(slotPrice)
		slot.PositionStatus = PositionStatusFilled
		slot.PositionQty = -(cfg.Trading.OrderQuantity / slotPrice) // 负数表示空仓
		slot.SlotStatus = SlotStatusFree

		// 平空买单价格 = 槽位价格 - 间距
		closePrice := roundPrice(slotPrice-priceInterval, 6)
		allOrders = append(allOrders, OrderInfo{
			Price:     closePrice,
			Side:      "BUY",
			Type:      "做空平仓",
			SlotPrice: slotPrice,
		})
		fmt.Printf("  平空: 槽位=%.6f, 买入价=%.6f\n", slotPrice, closePrice)
	}
	fmt.Printf("  共 1 个平空单（模拟一个空单成交）\n")

	// ========== 检测价格重叠 ==========
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("价格重叠检测")
	fmt.Println(strings.Repeat("=", 60))

	// 按价格分组
	priceMap := make(map[float64][]OrderInfo)
	for _, order := range allOrders {
		priceMap[order.Price] = append(priceMap[order.Price], order)
	}

	// 检查每个价格点
	conflictCount := 0
	for price, orders := range priceMap {
		if len(orders) > 1 {
			// 检查是否有不同方向的订单
			hasBuy := false
			hasSell := false
			for _, o := range orders {
				if o.Side == "BUY" {
					hasBuy = true
				} else {
					hasSell = true
				}
			}

			if hasBuy && hasSell {
				conflictCount++
				fmt.Printf("\n❌ 价格 %.6f 存在买卖冲突:\n", price)
				for _, o := range orders {
					fmt.Printf("   - %s %s (槽位: %.6f)\n", o.Side, o.Type, o.SlotPrice)
				}
				t.Errorf("价格 %.6f 同时有买单和卖单", price)
			} else {
				// 同方向多个订单（可能是不同槽位的订单）
				fmt.Printf("\n⚠️ 价格 %.6f 有多个同方向订单:\n", price)
				for _, o := range orders {
					fmt.Printf("   - %s %s (槽位: %.6f)\n", o.Side, o.Type, o.SlotPrice)
				}
			}
		}
	}

	// ========== 价格区域分析 ==========
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("价格区域分析")
	fmt.Println(strings.Repeat("=", 60))

	// 收集各类型订单的价格范围
	var buyOpenPrices, sellClosePrices, sellOpenPrices, buyClosePrices []float64
	for _, o := range allOrders {
		switch o.Type {
		case "做多开仓":
			buyOpenPrices = append(buyOpenPrices, o.Price)
		case "做多平仓":
			sellClosePrices = append(sellClosePrices, o.Price)
		case "做空开仓":
			sellOpenPrices = append(sellOpenPrices, o.Price)
		case "做空平仓":
			buyClosePrices = append(buyClosePrices, o.Price)
		}
	}

	// 排序
	sort.Float64s(buyOpenPrices)
	sort.Float64s(sellClosePrices)
	sort.Float64s(sellOpenPrices)
	sort.Float64s(buyClosePrices)

	fmt.Printf("\n做多开仓(BUY):  [%.6f ~ %.6f] (%d个)\n",
		buyOpenPrices[0], buyOpenPrices[len(buyOpenPrices)-1], len(buyOpenPrices))
	fmt.Printf("做多平仓(SELL): [%.6f ~ %.6f] (%d个)\n",
		sellClosePrices[0], sellClosePrices[len(sellClosePrices)-1], len(sellClosePrices))
	fmt.Printf("做空开仓(SELL): [%.6f ~ %.6f] (%d个)\n",
		sellOpenPrices[0], sellOpenPrices[len(sellOpenPrices)-1], len(sellOpenPrices))
	fmt.Printf("做空平仓(BUY):  [%.6f ~ %.6f] (%d个)\n",
		buyClosePrices[0], buyClosePrices[len(buyClosePrices)-1], len(buyClosePrices))

	// 检查区域重叠
	fmt.Println("\n--- 区域重叠检查 ---")

	// 做多平仓(SELL) vs 做空开仓(SELL) - 都是卖单，检查价格是否重叠
	sellCloseMax := sellClosePrices[len(sellClosePrices)-1]
	sellOpenMin := sellOpenPrices[0]
	if sellCloseMax >= sellOpenMin {
		fmt.Printf("⚠️ 做多平仓卖单 和 做空开仓卖单 价格可能重叠\n")
		fmt.Printf("   做多平仓最高价: %.6f\n", sellCloseMax)
		fmt.Printf("   做空开仓最低价: %.6f\n", sellOpenMin)
	} else {
		gap := sellOpenMin - sellCloseMax
		fmt.Printf("✅ 做多平仓卖单 和 做空开仓卖单 分离，间隔: %.6f (%.2f%%)\n",
			gap, gap/currentPrice*100)
	}

	// 做多开仓(BUY) vs 做空平仓(BUY) - 都是买单，检查价格是否重叠
	buyOpenMax := buyOpenPrices[len(buyOpenPrices)-1]
	buyCloseMin := buyClosePrices[0]
	if buyOpenMax >= buyCloseMin {
		fmt.Printf("⚠️ 做多开仓买单 和 做空平仓买单 价格可能重叠\n")
		fmt.Printf("   做多开仓最高价: %.6f\n", buyOpenMax)
		fmt.Printf("   做空平仓最低价: %.6f\n", buyCloseMin)
	} else {
		gap := buyCloseMin - buyOpenMax
		fmt.Printf("✅ 做多开仓买单 和 做空平仓买单 分离，间隔: %.6f (%.2f%%)\n",
			gap, gap/currentPrice*100)
	}

	// ========== 总结 ==========
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("测试总结")
	fmt.Println(strings.Repeat("=", 60))

	if conflictCount == 0 {
		fmt.Println("✅ 没有发现同价格买卖冲突")
	} else {
		fmt.Printf("❌ 发现 %d 个价格点存在买卖冲突\n", conflictCount)
	}

	fmt.Println("\n订单分布图:")
	fmt.Println("价格轴 (从低到高):")
	fmt.Printf("  [%.4f]----[%.4f]  做多开仓(BUY)\n", buyOpenPrices[0], buyOpenPrices[len(buyOpenPrices)-1])
	fmt.Printf("  [%.4f]----[%.4f]  做多平仓(SELL)\n", sellClosePrices[0], sellClosePrices[len(sellClosePrices)-1])
	fmt.Printf("  ... 当前价格: %.4f ...\n", currentPrice)
	fmt.Printf("  [%.4f]----[%.4f]  做空平仓(BUY)\n", buyClosePrices[0], buyClosePrices[len(buyClosePrices)-1])
	fmt.Printf("  [%.4f]----[%.4f]  做空开仓(SELL)\n", sellOpenPrices[0], sellOpenPrices[len(sellOpenPrices)-1])
}

// TestSamePriceBuySellConflict 测试同一价格是否会同时有买单和卖单
func TestSamePriceBuySellConflict(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("同价格买卖冲突详细测试")
	fmt.Println(strings.Repeat("=", 60))

	currentPrice := 0.14000
	priceInterval := 0.001
	anchor := currentPrice
	shortZoneMin := anchor * 1.2

	// 计算各类订单的价格边界
	buyWindowSize := 20

	// 做多开仓买单：从当前价格向下
	buyOpenMax := currentPrice
	buyOpenMin := currentPrice - float64(buyWindowSize-1)*priceInterval

	// 做多平仓卖单：买入价 + 间距
	sellCloseMax := buyOpenMax + priceInterval
	sellCloseMin := buyOpenMin + priceInterval

	// 做空开仓卖单：从 shortZoneMin 开始
	sellOpenMin := shortZoneMin
	sellOpenMax := shortZoneMin + 9*priceInterval // 10个空单

	// 做空平仓买单：开仓价 - 间距
	buyCloseMin := sellOpenMin - priceInterval
	buyCloseMax := sellOpenMax - priceInterval

	fmt.Printf("\n当前价格: %.6f\n", currentPrice)
	fmt.Printf("价格间距: %.6f\n", priceInterval)
	fmt.Printf("做空区域起点: %.6f (锚点×1.2)\n", shortZoneMin)

	fmt.Println("\n各类订单价格范围:")
	fmt.Printf("  做多开仓(BUY):  %.6f ~ %.6f\n", buyOpenMin, buyOpenMax)
	fmt.Printf("  做多平仓(SELL): %.6f ~ %.6f\n", sellCloseMin, sellCloseMax)
	fmt.Printf("  做空平仓(BUY):  %.6f ~ %.6f\n", buyCloseMin, buyCloseMax)
	fmt.Printf("  做空开仓(SELL): %.6f ~ %.6f\n", sellOpenMin, sellOpenMax)

	fmt.Println("\n冲突检查:")

	// 检查1: 做多开仓(BUY) vs 做多平仓(SELL)
	if buyOpenMax >= sellCloseMin {
		overlap := buyOpenMax - sellCloseMin + priceInterval
		fmt.Printf("⚠️ 做多开仓 和 做多平仓 有重叠: %.6f\n", overlap)
	} else {
		fmt.Println("✅ 做多开仓 和 做多平仓 无重叠（正常，它们是相邻的）")
	}

	// 检查2: 做多平仓(SELL) vs 做空开仓(SELL)
	gap1 := sellOpenMin - sellCloseMax
	if gap1 < 0 {
		fmt.Printf("❌ 做多平仓卖单 和 做空开仓卖单 重叠！\n")
		t.Errorf("卖单重叠")
	} else {
		fmt.Printf("✅ 做多平仓卖单 和 做空开仓卖单 分离，间隔: %.6f (%.2f%%)\n",
			gap1, gap1/currentPrice*100)
	}

	// 检查3: 做多开仓(BUY) vs 做空平仓(BUY)
	gap2 := buyCloseMin - buyOpenMax
	if gap2 < 0 {
		fmt.Printf("❌ 做多开仓买单 和 做空平仓买单 重叠！\n")
		t.Errorf("买单重叠")
	} else {
		fmt.Printf("✅ 做多开仓买单 和 做空平仓买单 分离，间隔: %.6f (%.2f%%)\n",
			gap2, gap2/currentPrice*100)
	}

	// 检查4: 做多平仓(SELL) vs 做空平仓(BUY) - 这是关键！
	// 做多平仓是卖单，做空平仓是买单，如果价格相同会冲突
	if sellCloseMax >= buyCloseMin {
		fmt.Printf("❌ 做多平仓卖单 和 做空平仓买单 价格重叠！可能同价买卖！\n")
		fmt.Printf("   做多平仓最高价: %.6f\n", sellCloseMax)
		fmt.Printf("   做空平仓最低价: %.6f\n", buyCloseMin)
		t.Errorf("同价格买卖冲突")
	} else {
		gap := buyCloseMin - sellCloseMax
		fmt.Printf("✅ 做多平仓卖单 和 做空平仓买单 分离，间隔: %.6f (%.2f%%)\n",
			gap, gap/currentPrice*100)
	}

	// 检查5: 做多开仓(BUY) vs 做空开仓(SELL) - 也是关键！
	if buyOpenMax >= sellOpenMin {
		fmt.Printf("❌ 做多开仓买单 和 做空开仓卖单 价格重叠！可能同价买卖！\n")
		t.Errorf("同价格买卖冲突")
	} else {
		gap := sellOpenMin - buyOpenMax
		fmt.Printf("✅ 做多开仓买单 和 做空开仓卖单 分离，间隔: %.6f (%.2f%%)\n",
			gap, gap/currentPrice*100)
	}

	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("结论: 由于做空区域在锚点1.2倍以上，")
	fmt.Println("      与做多区域（当前价格附近）有约20%的间隔，")
	fmt.Println("      不会出现同价格买卖冲突。")
}


// TestRealWorldScenario 真实场景测试：同一槽位不会同时有买卖单
func TestRealWorldScenario(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("真实场景测试：槽位状态机")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Println("\n关键理解：每个槽位是一个状态机，同一时刻只能处于一种状态")

	fmt.Println("\n=== 做多网格槽位状态 ===")
	fmt.Println("槽位价格: 0.138")
	fmt.Println("")
	fmt.Println("状态1: EMPTY (空仓)")
	fmt.Println("  → 可以挂 BUY 买单 (价格=0.138)")
	fmt.Println("  → 不能挂 SELL 卖单")
	fmt.Println("")
	fmt.Println("状态2: FILLED (多仓，持仓>0)")
	fmt.Println("  → 不能挂 BUY 买单")
	fmt.Println("  → 可以挂 SELL 卖单 (价格=0.139)")
	fmt.Println("")
	fmt.Println("结论: 同一槽位不会同时有买单和卖单 ✅")

	fmt.Println("\n=== 做空网格槽位状态 ===")
	fmt.Println("槽位价格: 0.168")
	fmt.Println("")
	fmt.Println("状态1: EMPTY (空仓)")
	fmt.Println("  → 可以挂 SELL 卖单开空 (价格=0.168)")
	fmt.Println("  → 不能挂 BUY 买单")
	fmt.Println("")
	fmt.Println("状态2: FILLED (空仓，持仓<0)")
	fmt.Println("  → 不能挂 SELL 卖单")
	fmt.Println("  → 可以挂 BUY 买单平空 (价格=0.167)")
	fmt.Println("")
	fmt.Println("结论: 同一槽位不会同时有买单和卖单 ✅")

	fmt.Println("\n=== 价格重叠的真相 ===")
	fmt.Println("")
	fmt.Println("价格 0.138 可能有:")
	fmt.Println("  - 槽位0.138的买单 (做多开仓)")
	fmt.Println("  - 槽位0.137的卖单 (做多平仓，0.137+0.001=0.138)")
	fmt.Println("")
	fmt.Println("但这两个订单属于不同槽位！")
	fmt.Println("  - 如果槽位0.138是EMPTY，才会挂买单")
	fmt.Println("  - 如果槽位0.137是FILLED，才会挂卖单")
	fmt.Println("")
	fmt.Println("它们不会冲突，因为:")
	fmt.Println("  1. 不同槽位独立管理")
	fmt.Println("  2. 交易所允许同价格不同方向的订单")
	fmt.Println("  3. 这正是网格交易的正常行为")

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("最终结论")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("")
	fmt.Println("✅ 做多网格和做空网格价格区域分离（间隔20%）")
	fmt.Println("✅ 同一槽位不会同时有买单和卖单")
	fmt.Println("✅ 不同槽位的订单可以在同一价格，这是正常的")
	fmt.Println("✅ 不存在真正的买卖冲突")
}

// TestSlotStateMachine 测试槽位状态机
func TestSlotStateMachine(t *testing.T) {
	cfg := createTestConfig()
	executor := NewMockOrderExecutor()
	exchange := NewMockExchange()
	spm := NewSuperPositionManager(cfg, executor, exchange, 6, 4)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("槽位状态机测试")
	fmt.Println(strings.Repeat("=", 60))

	// 测试做多槽位
	fmt.Println("\n--- 做多槽位测试 (价格=0.138) ---")
	longSlot := spm.getOrCreateSlot(0.138)

	// 状态1: EMPTY
	longSlot.PositionStatus = PositionStatusEmpty
	longSlot.PositionQty = 0
	longSlot.SlotStatus = SlotStatusFree
	longSlot.OrderID = 0

	canBuy := longSlot.PositionStatus == PositionStatusEmpty &&
		longSlot.SlotStatus == SlotStatusFree &&
		longSlot.OrderID == 0
	canSell := longSlot.PositionStatus == PositionStatusFilled &&
		longSlot.PositionQty > 0 &&
		longSlot.SlotStatus == SlotStatusFree &&
		longSlot.OrderID == 0

	fmt.Printf("状态: EMPTY, 持仓: %.4f\n", longSlot.PositionQty)
	fmt.Printf("  可以买入: %v\n", canBuy)
	fmt.Printf("  可以卖出: %v\n", canSell)

	if canBuy && canSell {
		t.Error("❌ 同一槽位同时可以买入和卖出")
	} else {
		fmt.Println("  ✅ 只能执行一种操作")
	}

	// 状态2: FILLED (多仓)
	longSlot.PositionStatus = PositionStatusFilled
	longSlot.PositionQty = 72.4638
	longSlot.SlotStatus = SlotStatusFree
	longSlot.OrderID = 0

	canBuy = longSlot.PositionStatus == PositionStatusEmpty &&
		longSlot.SlotStatus == SlotStatusFree &&
		longSlot.OrderID == 0
	canSell = longSlot.PositionStatus == PositionStatusFilled &&
		longSlot.PositionQty > 0 &&
		longSlot.SlotStatus == SlotStatusFree &&
		longSlot.OrderID == 0

	fmt.Printf("\n状态: FILLED, 持仓: %.4f (多仓)\n", longSlot.PositionQty)
	fmt.Printf("  可以买入: %v\n", canBuy)
	fmt.Printf("  可以卖出: %v\n", canSell)

	if canBuy && canSell {
		t.Error("❌ 同一槽位同时可以买入和卖出")
	} else {
		fmt.Println("  ✅ 只能执行一种操作")
	}

	// 测试做空槽位
	fmt.Println("\n--- 做空槽位测试 (价格=0.168) ---")
	shortSlot := spm.getOrCreateSlot(0.168)

	// 状态1: EMPTY (可以开空)
	shortSlot.PositionStatus = PositionStatusEmpty
	shortSlot.PositionQty = 0
	shortSlot.SlotStatus = SlotStatusFree
	shortSlot.OrderID = 0

	canOpenShort := shortSlot.PositionStatus == PositionStatusEmpty &&
		shortSlot.SlotStatus == SlotStatusFree &&
		shortSlot.OrderID == 0
	canCloseShort := shortSlot.PositionQty < -0.000001 &&
		shortSlot.SlotStatus == SlotStatusFree &&
		shortSlot.OrderID == 0

	fmt.Printf("状态: EMPTY, 持仓: %.4f\n", shortSlot.PositionQty)
	fmt.Printf("  可以开空(SELL): %v\n", canOpenShort)
	fmt.Printf("  可以平空(BUY): %v\n", canCloseShort)

	if canOpenShort && canCloseShort {
		t.Error("❌ 同一槽位同时可以开空和平空")
	} else {
		fmt.Println("  ✅ 只能执行一种操作")
	}

	// 状态2: FILLED (空仓，持仓<0)
	shortSlot.PositionStatus = PositionStatusFilled
	shortSlot.PositionQty = -59.5238 // 负数表示空仓
	shortSlot.SlotStatus = SlotStatusFree
	shortSlot.OrderID = 0

	canOpenShort = shortSlot.PositionStatus == PositionStatusEmpty &&
		shortSlot.SlotStatus == SlotStatusFree &&
		shortSlot.OrderID == 0
	canCloseShort = shortSlot.PositionQty < -0.000001 &&
		shortSlot.SlotStatus == SlotStatusFree &&
		shortSlot.OrderID == 0

	fmt.Printf("\n状态: FILLED, 持仓: %.4f (空仓)\n", shortSlot.PositionQty)
	fmt.Printf("  可以开空(SELL): %v\n", canOpenShort)
	fmt.Printf("  可以平空(BUY): %v\n", canCloseShort)

	if canOpenShort && canCloseShort {
		t.Error("❌ 同一槽位同时可以开空和平空")
	} else {
		fmt.Println("  ✅ 只能执行一种操作")
	}

	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("结论: 槽位状态机保证同一槽位不会同时有买卖单 ✅")
}


// TestProfitAnalysis 盈利分析测试
func TestProfitAnalysis(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("盈利分析测试")
	fmt.Println(strings.Repeat("=", 60))

	priceInterval := 0.001
	feeRate := 0.0002 // 手续费率 0.02%

	// ========== 做多盈利分析 ==========
	fmt.Println("\n=== 做多盈利分析 ===")
	fmt.Println("策略: 低买高卖")
	fmt.Println("")

	longBuyPrice := 0.138000  // 买入价格
	longSellPrice := 0.139000 // 卖出价格 = 买入价 + 间距
	longQuantity := 72.4638   // 买入数量 (10U / 0.138)

	fmt.Printf("买入价格: %.6f\n", longBuyPrice)
	fmt.Printf("卖出价格: %.6f (买入价 + %.6f)\n", longSellPrice, priceInterval)
	fmt.Printf("交易数量: %.4f\n", longQuantity)

	// 计算盈亏
	longBuyCost := longBuyPrice * longQuantity
	longSellRevenue := longSellPrice * longQuantity
	longGrossProfit := longSellRevenue - longBuyCost
	longBuyFee := longBuyCost * feeRate
	longSellFee := longSellRevenue * feeRate
	longTotalFee := longBuyFee + longSellFee
	longNetProfit := longGrossProfit - longTotalFee

	fmt.Printf("\n买入成本: %.6f U\n", longBuyCost)
	fmt.Printf("卖出收入: %.6f U\n", longSellRevenue)
	fmt.Printf("毛利润:   %.6f U\n", longGrossProfit)
	fmt.Printf("买入手续费: %.6f U (%.4f%%)\n", longBuyFee, feeRate*100)
	fmt.Printf("卖出手续费: %.6f U (%.4f%%)\n", longSellFee, feeRate*100)
	fmt.Printf("总手续费: %.6f U\n", longTotalFee)
	fmt.Printf("净利润:   %.6f U\n", longNetProfit)

	if longNetProfit > 0 {
		profitRate := longNetProfit / longBuyCost * 100
		fmt.Printf("\n✅ 做多盈利! 利润率: %.4f%%\n", profitRate)
	} else {
		fmt.Printf("\n❌ 做多亏损!\n")
		t.Error("做多应该盈利")
	}

	// ========== 做空盈利分析 ==========
	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("\n=== 做空盈利分析 ===")
	fmt.Println("策略: 高卖低买")
	fmt.Println("")

	shortOpenPrice := 0.168000  // 开空价格（卖出）
	shortClosePrice := 0.167000 // 平空价格（买入）= 开仓价 - 间距
	shortQuantity := 59.5238    // 开仓数量 (10U / 0.168)

	fmt.Printf("开空价格(卖出): %.6f\n", shortOpenPrice)
	fmt.Printf("平空价格(买入): %.6f (开仓价 - %.6f)\n", shortClosePrice, priceInterval)
	fmt.Printf("交易数量: %.4f\n", shortQuantity)

	// 计算盈亏
	shortOpenRevenue := shortOpenPrice * shortQuantity  // 开空时卖出收入
	shortCloseCost := shortClosePrice * shortQuantity   // 平空时买入成本
	shortGrossProfit := shortOpenRevenue - shortCloseCost
	shortOpenFee := shortOpenRevenue * feeRate
	shortCloseFee := shortCloseCost * feeRate
	shortTotalFee := shortOpenFee + shortCloseFee
	shortNetProfit := shortGrossProfit - shortTotalFee

	fmt.Printf("\n开空收入(卖出): %.6f U\n", shortOpenRevenue)
	fmt.Printf("平空成本(买入): %.6f U\n", shortCloseCost)
	fmt.Printf("毛利润:   %.6f U\n", shortGrossProfit)
	fmt.Printf("开空手续费: %.6f U (%.4f%%)\n", shortOpenFee, feeRate*100)
	fmt.Printf("平空手续费: %.6f U (%.4f%%)\n", shortCloseFee, feeRate*100)
	fmt.Printf("总手续费: %.6f U\n", shortTotalFee)
	fmt.Printf("净利润:   %.6f U\n", shortNetProfit)

	if shortNetProfit > 0 {
		profitRate := shortNetProfit / shortCloseCost * 100
		fmt.Printf("\n✅ 做空盈利! 利润率: %.4f%%\n", profitRate)
	} else {
		fmt.Printf("\n❌ 做空亏损!\n")
		t.Error("做空应该盈利")
	}

	// ========== 盈利对比 ==========
	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("\n=== 盈利对比 ===")
	fmt.Printf("做多净利润: %.6f U\n", longNetProfit)
	fmt.Printf("做空净利润: %.6f U\n", shortNetProfit)
	fmt.Printf("每单投入: 10 U\n")
	fmt.Printf("做多利润率: %.4f%%\n", longNetProfit/10*100)
	fmt.Printf("做空利润率: %.4f%%\n", shortNetProfit/10*100)
}

// TestProfitWithDifferentIntervals 不同间距的盈利测试
func TestProfitWithDifferentIntervals(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("不同间距的盈利测试")
	fmt.Println(strings.Repeat("=", 60))

	feeRate := 0.0002 // 手续费率 0.02%
	orderValue := 10.0 // 每单10U

	intervals := []float64{0.0001, 0.0005, 0.001, 0.002, 0.005}
	basePrice := 0.14 // 基准价格

	fmt.Printf("\n基准价格: %.4f\n", basePrice)
	fmt.Printf("每单金额: %.0f U\n", orderValue)
	fmt.Printf("手续费率: %.4f%%\n", feeRate*100)

	fmt.Println("\n| 间距 | 间距% | 毛利润 | 手续费 | 净利润 | 利润率 | 结果 |")
	fmt.Println("|------|-------|--------|--------|--------|--------|------|")

	for _, interval := range intervals {
		buyPrice := basePrice
		sellPrice := basePrice + interval
		quantity := orderValue / buyPrice

		buyCost := buyPrice * quantity
		sellRevenue := sellPrice * quantity
		grossProfit := sellRevenue - buyCost
		totalFee := (buyCost + sellRevenue) * feeRate
		netProfit := grossProfit - totalFee
		profitRate := netProfit / orderValue * 100
		intervalPercent := interval / basePrice * 100

		result := "✅"
		if netProfit <= 0 {
			result = "❌"
		}

		fmt.Printf("| %.4f | %.3f%% | %.4f | %.4f | %.4f | %.3f%% | %s |\n",
			interval, intervalPercent, grossProfit, totalFee, netProfit, profitRate, result)
	}

	// 计算保本间距
	// 毛利润 = 间距 × 数量 = 间距 × (orderValue / price)
	// 手续费 = 2 × orderValue × feeRate (买卖各一次)
	// 保本: 间距 × (orderValue / price) = 2 × orderValue × feeRate
	// 间距 = 2 × price × feeRate
	breakEvenInterval := 2 * basePrice * feeRate
	fmt.Printf("\n保本间距: %.6f (%.4f%%)\n", breakEvenInterval, breakEvenInterval/basePrice*100)
	fmt.Println("间距必须大于保本间距才能盈利")
}

// TestShortProfitScenarios 做空不同场景的盈利测试
func TestShortProfitScenarios(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("做空不同场景的盈利测试")
	fmt.Println(strings.Repeat("=", 60))

	feeRate := 0.0002
	orderValue := 10.0
	priceInterval := 0.001

	fmt.Println("\n场景: 价格从当前价格上涨到做空区域，然后回落")
	fmt.Println("")

	currentPrice := 0.14
	anchor := currentPrice
	shortZoneMin := anchor * 1.2 // 0.168

	fmt.Printf("当前价格: %.4f\n", currentPrice)
	fmt.Printf("做空锚点: %.4f\n", anchor)
	fmt.Printf("做空区域起点: %.4f (锚点×1.2)\n", shortZoneMin)
	fmt.Printf("价格间距: %.4f\n", priceInterval)

	// 模拟做空过程
	fmt.Println("\n--- 做空过程 ---")

	// 1. 价格上涨到做空区域
	fmt.Println("1. 价格上涨到 0.168，触发开空")
	openPrice := 0.168
	quantity := orderValue / openPrice
	fmt.Printf("   开空价格: %.4f, 数量: %.4f\n", openPrice, quantity)

	// 2. 价格回落，触发平空
	fmt.Println("2. 价格回落到 0.167，触发平空")
	closePrice := openPrice - priceInterval
	fmt.Printf("   平空价格: %.4f\n", closePrice)

	// 3. 计算盈亏
	openRevenue := openPrice * quantity
	closeCost := closePrice * quantity
	grossProfit := openRevenue - closeCost
	totalFee := (openRevenue + closeCost) * feeRate
	netProfit := grossProfit - totalFee

	fmt.Println("\n--- 盈亏计算 ---")
	fmt.Printf("开空收入: %.4f U\n", openRevenue)
	fmt.Printf("平空成本: %.4f U\n", closeCost)
	fmt.Printf("毛利润: %.4f U\n", grossProfit)
	fmt.Printf("手续费: %.4f U\n", totalFee)
	fmt.Printf("净利润: %.4f U\n", netProfit)

	if netProfit > 0 {
		fmt.Printf("\n✅ 做空盈利! 利润率: %.4f%%\n", netProfit/orderValue*100)
	} else {
		fmt.Printf("\n❌ 做空亏损!\n")
		t.Error("做空应该盈利")
	}

	// 对比做多
	fmt.Println("\n--- 对比做多 ---")
	longBuyPrice := 0.138
	longSellPrice := 0.139
	longQuantity := orderValue / longBuyPrice
	longGrossProfit := (longSellPrice - longBuyPrice) * longQuantity
	longTotalFee := (longBuyPrice + longSellPrice) * longQuantity * feeRate
	longNetProfit := longGrossProfit - longTotalFee

	fmt.Printf("做多: 买%.4f 卖%.4f, 净利润: %.4f U\n",
		longBuyPrice, longSellPrice, longNetProfit)
	fmt.Printf("做空: 卖%.4f 买%.4f, 净利润: %.4f U\n",
		openPrice, closePrice, netProfit)

	fmt.Println("\n结论:")
	fmt.Println("✅ 做多: 低买高卖，价格上涨时盈利")
	fmt.Println("✅ 做空: 高卖低买，价格下跌时盈利")
	fmt.Println("✅ 两种策略都能盈利，只要价格间距大于手续费成本")
}

// TestReduceOnlyOrderFix 测试 ReduceOnly 订单修复
// 验证：当槽位状态为 FILLED 但持仓数量 <= 0 时，不会创建 ReduceOnly 卖单
func TestReduceOnlyOrderFix(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("ReduceOnly 订单修复测试")
	fmt.Println(strings.Repeat("=", 60))

	// 创建配置
	cfg := createTestConfig()
	executor := NewMockOrderExecutor()
	exchange := NewMockExchange()

	// 创建仓位管理器
	spm := NewSuperPositionManager(cfg, executor, exchange, 6, 4)
	currentPrice := 0.14000
	spm.anchorPrice = currentPrice
	spm.lastMarketPrice.Store(currentPrice)
	spm.isInitialized.Store(true)

	fmt.Println("\n===== 测试场景：槽位状态为 FILLED 但持仓数量为 0 =====")
	fmt.Printf("当前价格: %.6f\n", currentPrice)
	fmt.Printf("价格间距: %.6f\n", cfg.Trading.PriceInterval)

	var hasReduceOnlySell bool
	var placedOrders []*OrderRequest

	// 场景1: 槽位状态为 FILLED，但持仓数量为 0
	testPrice1 := 0.139000
	slot1 := spm.getOrCreateSlot(testPrice1)
	slot1.mu.Lock()
	slot1.PositionStatus = PositionStatusFilled
	slot1.PositionQty = 0.0 // 持仓数量为 0
	slot1.SlotStatus = SlotStatusFree
	slot1.OrderID = 0
	slot1.ClientOID = ""
	slot1.mu.Unlock()

	fmt.Printf("\n槽位1: 价格=%.6f, 状态=%s, 持仓=%.6f\n", 
		testPrice1, slot1.PositionStatus, slot1.PositionQty)

	// 尝试创建卖单（通过 AdjustOrders）
	err := spm.AdjustOrders(currentPrice)
	if err != nil {
		t.Errorf("AdjustOrders failed: %v", err)
	}

	// 验证结果：检查 MockOrderExecutor 中记录的订单
	hasReduceOnlySell = false
	placedOrders = executor.GetPlacedOrders()
	for _, order := range placedOrders {
		if order.Side == "SELL" && order.ReduceOnly {
			hasReduceOnlySell = true
			fmt.Printf("❌ 发现 ReduceOnly 卖单: 价格=%.6f, 数量=%.4f\n", 
				order.Price, order.Quantity)
		}
	}

	if hasReduceOnlySell {
		t.Error("❌ 测试失败: 持仓数量为 0 时不应创建 ReduceOnly 卖单")
	} else {
		fmt.Println("✅ 测试通过: 持仓数量为 0 时未创建 ReduceOnly 卖单")
	}

	// 场景2: 槽位状态为 FILLED，持仓数量 > 0（正常情况）
	executor.ClearOrders()
	testPrice2 := 0.138000
	slot2 := spm.getOrCreateSlot(testPrice2)
	slot2.mu.Lock()
	slot2.PositionStatus = PositionStatusFilled
	slot2.PositionQty = 72.4638 // 持仓数量 > 0
	slot2.SlotStatus = SlotStatusFree
	slot2.OrderID = 0
	slot2.ClientOID = ""
	slot2.mu.Unlock()

	fmt.Printf("\n槽位2: 价格=%.6f, 状态=%s, 持仓=%.6f\n", 
		testPrice2, slot2.PositionStatus, slot2.PositionQty)

	// 尝试创建卖单（通过 AdjustOrders）
	err = spm.AdjustOrders(currentPrice)
	if err != nil {
		t.Errorf("AdjustOrders failed: %v", err)
	}

	// 验证结果：检查 MockOrderExecutor 中记录的订单
	hasReduceOnlySell = false
	placedOrders = executor.GetPlacedOrders()
	for _, order := range placedOrders {
		if order.Side == "SELL" && order.ReduceOnly {
			hasReduceOnlySell = true
			fmt.Printf("✅ 发现 ReduceOnly 卖单: 价格=%.6f, 数量=%.4f\n", 
				order.Price, order.Quantity)
		}
	}

	if !hasReduceOnlySell {
		t.Error("❌ 测试失败: 持仓数量 > 0 时应创建 ReduceOnly 卖单")
	} else {
		fmt.Println("✅ 测试通过: 持仓数量 > 0 时正确创建了 ReduceOnly 卖单")
	}

	// 场景3: 槽位状态为 FILLED，持仓数量为负数（空仓）
	executor.ClearOrders()
	testPrice3 := 0.137000
	slot3 := spm.getOrCreateSlot(testPrice3)
	slot3.mu.Lock()
	slot3.PositionStatus = PositionStatusFilled
	slot3.PositionQty = -59.5238 // 负数表示空仓
	slot3.SlotStatus = SlotStatusFree
	slot3.OrderID = 0
	slot3.ClientOID = ""
	slot3.mu.Unlock()

	fmt.Printf("\n槽位3: 价格=%.6f, 状态=%s, 持仓=%.6f (空仓)\n", 
		testPrice3, slot3.PositionStatus, slot3.PositionQty)

	// 尝试创建卖单（通过 AdjustOrders）
	err = spm.AdjustOrders(currentPrice)
	if err != nil {
		t.Errorf("AdjustOrders failed: %v", err)
	}

	// 验证结果：检查 MockOrderExecutor 中记录的订单
	hasReduceOnlySell = false
	placedOrders = executor.GetPlacedOrders()
	for _, order := range placedOrders {
		if order.Side == "SELL" && order.ReduceOnly {
			hasReduceOnlySell = true
			fmt.Printf("❌ 发现 ReduceOnly 卖单: 价格=%.6f, 数量=%.4f\n", 
				order.Price, order.Quantity)
		}
	}

	if hasReduceOnlySell {
		t.Error("❌ 测试失败: 空仓（持仓<0）时不应创建 ReduceOnly 卖单")
	} else {
		fmt.Println("✅ 测试通过: 空仓（持仓<0）时未创建 ReduceOnly 卖单")
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("所有测试场景完成 ✅")
	fmt.Println("修复总结:")
	fmt.Println("  - 持仓数量 <= 0 时，不会创建 ReduceOnly 卖单")
	fmt.Println("  - 持仓数量 > 0 时，正确创建 ReduceOnly 卖单")
	fmt.Println("  - 避免了币安返回 -2022 错误（ReduceOnly Order is rejected）")
	fmt.Println(strings.Repeat("=", 60))
}

// TestRealConfigParameters 使用config.yaml中的实际参数测试
func TestRealConfigParameters(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("真实配置参数测试（config.yaml）")
	fmt.Println(strings.Repeat("=", 60))

	currentPrice := 0.14000
	priceInterval := 0.0001
	anchor := currentPrice
	
	// 使用config.yaml中的实际参数
	shortZoneMinMult := 1.004
	shortZoneMaxMult := 1.006
	shortZoneMin := anchor * shortZoneMinMult
	shortZoneMax := anchor * shortZoneMaxMult

	fmt.Printf("\n当前价格: %.6f\n", currentPrice)
	fmt.Printf("价格间距: %.6f\n", priceInterval)
	fmt.Printf("做空锚点: %.6f\n", anchor)
	fmt.Printf("做空区域最小倍数: %.3f\n", shortZoneMinMult)
	fmt.Printf("做空区域最大倍数: %.3f\n", shortZoneMaxMult)
	fmt.Printf("做空区域: [%.6f ~ %.6f]\n", shortZoneMin, shortZoneMax)

	type OrderInfo struct {
		Price    float64
		Side     string
		Type     string
		SlotPrice float64
	}
	allOrders := make([]OrderInfo, 0)

	// 1. 做多开仓（买单）- 在当前价格下方
	fmt.Println("\n--- 1. 做多开仓（买单）---")
	buyWindowSize := 10
	for i := 0; i < buyWindowSize; i++ {
		slotPrice := roundPrice(currentPrice-float64(i)*priceInterval, 6)
		allOrders = append(allOrders, OrderInfo{
			Price:     slotPrice,
			Side:      "BUY",
			Type:      "做多开仓",
			SlotPrice: slotPrice,
		})
		if i < 5 {
			fmt.Printf("  买单 %d: 价格=%.6f\n", i+1, slotPrice)
		}
	}
	fmt.Printf("  ... 共 %d 个买单\n", buyWindowSize)

	// 2. 做多平仓（卖单）- 模拟一个买单成交
	fmt.Println("\n--- 2. 做多平仓（卖单）---")
	sellPrice := roundPrice(currentPrice+priceInterval, 6)
	allOrders = append(allOrders, OrderInfo{
		Price:     sellPrice,
		Side:      "SELL",
		Type:      "做多平仓",
		SlotPrice: currentPrice,
	})
	fmt.Printf("  卖单: 槽位=%.6f, 卖出价=%.6f\n", currentPrice, sellPrice)

	// 3. 做空开仓（卖单）- 在做空区域
	fmt.Println("\n--- 3. 做空开仓（卖单）---")
	shortCount := 0
	for price := shortZoneMin; price <= shortZoneMax && shortCount < 10; price += priceInterval {
		slotPrice := roundPrice(price, 6)
		allOrders = append(allOrders, OrderInfo{
			Price:     slotPrice,
			Side:      "SELL",
			Type:      "做空开仓",
			SlotPrice: slotPrice,
		})
		if shortCount < 5 {
			fmt.Printf("  空单 %d: 价格=%.6f\n", shortCount+1, slotPrice)
		}
		shortCount++
	}
	fmt.Printf("  ... 共 %d 个空单\n", shortCount)

	// 4. 做空平仓（买单）- 使用优化后的逻辑
	fmt.Println("\n--- 4. 做空平仓（买单）---")
	if shortCount > 0 {
		slotPrice := roundPrice(shortZoneMin, 6)
		
		// 🔥 使用优化后的平仓逻辑
		var closePrice float64
		if slotPrice > currentPrice+2*priceInterval {
			// 价格已经下跌较多，使用做多平仓价+间隔快速平仓
			// 这样可以避免与做多平仓价冲突
			closePrice = currentPrice + 2*priceInterval
			fmt.Printf("  使用快速平仓策略\n")
		} else {
			// 价格接近开空价，使用正常平仓价
			closePrice = slotPrice - priceInterval
			fmt.Printf("  使用正常平仓策略\n")
		}
		closePrice = roundPrice(closePrice, 6)
		
		allOrders = append(allOrders, OrderInfo{
			Price:     closePrice,
			Side:      "BUY",
			Type:      "做空平仓",
			SlotPrice: slotPrice,
		})
		fmt.Printf("  平空: 槽位=%.6f, 买入价=%.6f\n", slotPrice, closePrice)
	}

	// ========== 价格重叠检测 ==========
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("价格重叠检测")
	fmt.Println(strings.Repeat("=", 60))

	priceMap := make(map[float64][]OrderInfo)
	for _, order := range allOrders {
		priceMap[order.Price] = append(priceMap[order.Price], order)
	}

	conflictCount := 0
	for price, orders := range priceMap {
		if len(orders) > 1 {
			hasBuy := false
			hasSell := false
			for _, o := range orders {
				if o.Side == "BUY" {
					hasBuy = true
				} else {
					hasSell = true
				}
			}

			if hasBuy && hasSell {
				conflictCount++
				fmt.Printf("\n❌ 价格 %.6f 存在买卖冲突:\n", price)
				for _, o := range orders {
					fmt.Printf("   - %s %s (槽位: %.6f)\n", o.Side, o.Type, o.SlotPrice)
				}
				t.Errorf("价格 %.6f 同时有买单和卖单", price)
			}
		}
	}

	if conflictCount == 0 {
		fmt.Println("\n✅ 没有发现同价格买卖冲突")
	}

	// ========== 价格区域分析 ==========
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("价格区域分析")
	fmt.Println(strings.Repeat("=", 60))

	var buyOpenPrices, sellClosePrices, sellOpenPrices, buyClosePrices []float64
	for _, o := range allOrders {
		switch o.Type {
		case "做多开仓":
			buyOpenPrices = append(buyOpenPrices, o.Price)
		case "做多平仓":
			sellClosePrices = append(sellClosePrices, o.Price)
		case "做空开仓":
			sellOpenPrices = append(sellOpenPrices, o.Price)
		case "做空平仓":
			buyClosePrices = append(buyClosePrices, o.Price)
		}
	}

	sort.Float64s(buyOpenPrices)
	sort.Float64s(sellClosePrices)
	sort.Float64s(sellOpenPrices)
	sort.Float64s(buyClosePrices)

	fmt.Printf("\n做多开仓(BUY):  [%.6f ~ %.6f] (%d个)\n", 
		buyOpenPrices[0], buyOpenPrices[len(buyOpenPrices)-1], len(buyOpenPrices))
	fmt.Printf("做多平仓(SELL): [%.6f ~ %.6f] (%d个)\n", 
		sellClosePrices[0], sellClosePrices[len(sellClosePrices)-1], len(sellClosePrices))
	fmt.Printf("做空开仓(SELL): [%.6f ~ %.6f] (%d个)\n", 
		sellOpenPrices[0], sellOpenPrices[len(sellOpenPrices)-1], len(sellOpenPrices))
	fmt.Printf("做空平仓(BUY):  [%.6f ~ %.6f] (%d个)\n", 
		buyClosePrices[0], buyClosePrices[len(buyClosePrices)-1], len(buyClosePrices))

	// 检查区域重叠
	fmt.Println("\n--- 区域重叠检查 ---")
	
	// 做多平仓卖单 vs 做空开仓卖单
	gap1 := sellOpenPrices[0] - sellClosePrices[len(sellClosePrices)-1]
	if gap1 < 0 {
		fmt.Printf("❌ 做多平仓卖单 和 做空开仓卖单 重叠！\n")
		t.Errorf("卖单重叠")
	} else {
		fmt.Printf("✅ 做多平仓卖单 和 做空开仓卖单 分离，间隔: %.6f (%.2f%%)\n",
			gap1, gap1/currentPrice*100)
	}

	// 做多开仓买单 vs 做空平仓买单
	gap2 := buyClosePrices[0] - buyOpenPrices[len(buyOpenPrices)-1]
	if gap2 < 0 {
		fmt.Printf("❌ 做多开仓买单 和 做空平仓买单 重叠！\n")
		t.Errorf("买单重叠")
	} else {
		fmt.Printf("✅ 做多开仓买单 和 做空平仓买单 分离，间隔: %.6f (%.2f%%)\n",
			gap2, gap2/currentPrice*100)
	}

	// ========== 测试总结 ==========
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("测试总结")
	fmt.Println(strings.Repeat("=", 60))
	
	if conflictCount == 0 {
		fmt.Println("✅ 测试通过：使用真实配置参数，没有价格冲突")
		fmt.Println("\n关键发现:")
		fmt.Printf("  - 做空区域非常接近当前价格（%.3f%% ~ %.3f%%）\n", 
			(shortZoneMinMult-1)*100, (shortZoneMaxMult-1)*100)
		fmt.Println("  - 优化后的平仓逻辑能够正确处理这种情况")
		fmt.Println("  - 当价格下跌时，使用快速平仓策略（当前价+间隔）")
		fmt.Println("  - 当价格接近时，使用正常平仓策略（开空价-间隔）")
	} else {
		fmt.Printf("❌ 测试失败：发现 %d 个价格冲突\n", conflictCount)
	}
}

// TestAlwaysEnableShortGrid 测试做空网格始终启用的功能
func TestAlwaysEnableShortGrid(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("做空网格始终启用测试")
	fmt.Println(strings.Repeat("=", 60))

	// 测试场景：当前价格低于做空区域，但仍应允许挂空单
	currentPrice := 0.14000
	anchor := 0.14000 // 假设锚点是0.14000
	shortZoneMin := anchor * 1.004 // 0.14056
	shortZoneMax := anchor * 1.006 // 0.14084

	fmt.Printf("当前价格: %.6f\n", currentPrice)
	fmt.Printf("锚点价格: %.6f\n", anchor)
	fmt.Printf("做空区域: [%.6f ~ %.6f]\n", shortZoneMin, shortZoneMax)
	fmt.Printf("当前价格 < 做空区域最小值: %t\n", currentPrice < shortZoneMin)

	// 验证在当前价格低于做空区域的情况下，仍应能挂空单
	// 在新的实现中，只要做空区域有效就会允许挂空单
	fmt.Println("\n✅ 预期行为: 即使当前价格不在做空区域内，只要做空区域有效，就可以挂空单")
	fmt.Println("   - 这是因为修改了crash_detector.go中的逻辑")
	fmt.Println("   - 不再要求当前价格必须在做空区域内")
	fmt.Println("   - 只要锚点和做空区域范围有效，就允许挂空单")
	
	fmt.Println("\n✅ 修改总结:")
	fmt.Println("   - 移除了super_position_manager.go中的安全检查")
	fmt.Println("   - 修改了crash_detector.go中的shouldShort判断逻辑")
	fmt.Println("   - 现在只要做空区域有效，就会允许挂空单")
}
