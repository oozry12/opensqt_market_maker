package position

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"opensqt/config"
	"opensqt/logger"
	"opensqt/monitor"
	"opensqt/utils"
)

// OrderUpdate 订单更新事件（避免依赖 websocket 包）
type OrderUpdate struct {
	OrderID       int64
	ClientOrderID string
	Symbol        string
	Status        string
	ExecutedQty   float64
	Price         float64
	AvgPrice      float64
	Side          string
	Type          string
	UpdateTime    int64
}

// OrderExecutorInterface 订单执行器接口（避免循环导入）
type OrderExecutorInterface interface {
	PlaceOrder(req *OrderRequest) (*Order, error)
	BatchPlaceOrders(orders []*OrderRequest) ([]*Order, bool)
	BatchCancelOrders(orderIDs []int64) error
}

// OrderRequest 订单请求（避免循环导入）
type OrderRequest struct {
	Symbol        string
	Side          string
	Price         float64
	Quantity      float64
	PriceDecimals int    // 价格小数位数（用于格式化价格字符串）
	ReduceOnly    bool   // 是否只减仓（平仓单）
	PostOnly      bool   // 是否只做 Maker（Post Only）
	ClientOrderID string // 自定义订单ID
}

// Order 订单信息（避免循环导入）
type Order struct {
	OrderID       int64
	ClientOrderID string
	Symbol        string
	Side          string
	Price         float64
	Quantity      float64
	Status        string
	CreatedAt     time.Time
	ReduceOnly    bool
}

// 订单状态常量
const (
	OrderStatusNotPlaced       = "NOT_PLACED"       // 未下单
	OrderStatusPlaced          = "PLACED"           // 已下单
	OrderStatusConfirmed       = "CONFIRMED"        // 已确认（WebSocket确认）
	OrderStatusPartiallyFilled = "PARTIALLY_FILLED" // 部分成交
	OrderStatusFilled          = "FILLED"           // 全部成交
	OrderStatusCancelRequested = "CANCEL_REQUESTED" // 已申请撤单
	OrderStatusCanceled        = "CANCELED"         // 已撤单
)

// 持仓状态常量
const (
	PositionStatusEmpty  = "EMPTY"  // 空仓
	PositionStatusFilled = "FILLED" // 多仓
)

// 槽位锁定状态
const (
	SlotStatusFree    = "FREE"    // 空闲，可操作
	SlotStatusPending = "PENDING" // 等待下单确认
	SlotStatusLocked  = "LOCKED"  // 已锁定，有活跃订单
)

// InventorySlot 库存槽位（每个价格点一个）
type InventorySlot struct {
	Price float64 // 价格（作为key，支持高精度）

	// 持仓信息
	PositionStatus string  // 持仓状态：空仓/多仓/空仓
	PositionQty    float64 // 持仓数量（正数表示多仓，负数表示空仓）

	// 订单信息 (买卖互斥)
	OrderID        int64     // 订单ID
	ClientOID      string    // 自定义订单ID
	OrderSide      string    // 订单方向 (BUY/SELL)
	OrderStatus    string    // 订单状态
	OrderPrice     float64   // 订单价格
	OrderFilledQty float64   // 成交数量
	OrderCreatedAt time.Time // 创建时间

	// 🔥 新增：槽位锁定状态，防止并发重复操作
	SlotStatus string // FREE/PENDING/LOCKED

	// PostOnly失败计数（连续失败3次后降级为普通单）
	PostOnlyFailCount int

	mu sync.RWMutex // 槽位级别的锁（细粒度锁）
}

// PositionInfo 持仓信息（简化版，避免循环导入）
type PositionInfo struct {
	Symbol string
	Size   float64
}

// IExchange 交易所接口（避免循环导入）
// 注意：这里不能直接使用 exchange.IExchange，否则会循环导入
// 所以定义一个子集接口，只包含对账需要的方法
type IExchange interface {
	GetName() string // 获取交易所名称
	GetPositions(ctx context.Context, symbol string) (interface{}, error)
	GetOpenOrders(ctx context.Context, symbol string) (interface{}, error)
	GetOrder(ctx context.Context, symbol string, orderID int64) (interface{}, error)
	GetBaseAsset() string                                     // 获取基础资产（交易币种）
	CancelAllOrders(ctx context.Context, symbol string) error // 取消所有订单
	GetAvailableBalance(ctx context.Context) (float64, error) // 获取可用保证金
}

// SuperPositionManager 超级仓位管理器
type SuperPositionManager struct {
	config   *config.Config
	executor OrderExecutorInterface
	exchange IExchange

	// 价格锚点（初始化时的市场价格）
	anchorPrice float64
	// 最后市场价格（用于打印状态）
	lastMarketPrice atomic.Value // float64
	// 价格精度（根据锚点价格检测得出的小数位数）
	priceDecimals int
	// 数量精度（从交易所获取）
	quantityDecimals int

	// 库存槽位：价格 -> 槽位
	slots sync.Map // map[float64]*InventorySlot

	// 保证金管理
	insufficientMargin bool
	marginLockTime     time.Time
	marginLockDuration time.Duration
	lowBalanceWarned   bool // 是否已警告过余额不足

	// 动态网格计算器
	dynamicGridCalc *monitor.DynamicGridCalculator
	atrCalculator   *monitor.ATRCalculator

	// 阴跌检测器
	downtrendDetector *monitor.DowntrendDetector

	// 暴跌检测器
	crashDetector *monitor.CrashDetector

	// 统计（注意：以下字段被 safety.Reconciler 和 PrintPositions 使用，不可删除）
	totalBuyQty       atomic.Value // float64 - 累计买入数量
	totalSellQty      atomic.Value // float64 - 累计卖出数量
	reconcileCount    atomic.Int64 // 对账次数
	lastReconcileTime atomic.Value // time.Time - 最后对账时间

	// 初始化标志
	isInitialized atomic.Bool

	mu sync.RWMutex // 全局锁（用于关键操作）
}

// NewSuperPositionManager 创建超级仓位管理器
func NewSuperPositionManager(cfg *config.Config, executor OrderExecutorInterface, exchange IExchange, priceDecimals, quantityDecimals int) *SuperPositionManager {
	marginLockSec := cfg.Trading.MarginLockDurationSec
	if marginLockSec <= 0 {
		marginLockSec = 10 // 默认10秒
	}

	spm := &SuperPositionManager{
		config:             cfg,
		executor:           executor,
		exchange:           exchange,
		insufficientMargin: false,
		marginLockDuration: time.Duration(marginLockSec) * time.Second,
		priceDecimals:      priceDecimals,
		quantityDecimals:   quantityDecimals,
	}
	spm.totalBuyQty.Store(0.0)
	spm.totalSellQty.Store(0.0)
	spm.lastReconcileTime.Store(time.Now())
	spm.lastMarketPrice.Store(0.0)
	return spm
}

// SetDynamicGridCalculator 设置动态网格计算器
func (spm *SuperPositionManager) SetDynamicGridCalculator(calc *monitor.DynamicGridCalculator) {
	spm.dynamicGridCalc = calc
}

// SetATRCalculator 设置ATR计算器
func (spm *SuperPositionManager) SetATRCalculator(atr *monitor.ATRCalculator) {
	spm.atrCalculator = atr
}

// SetDowntrendDetector 设置阴跌检测器
func (spm *SuperPositionManager) SetDowntrendDetector(detector *monitor.DowntrendDetector) {
	spm.downtrendDetector = detector
}

// SetCrashDetector 设置暴跌检测器
func (spm *SuperPositionManager) SetCrashDetector(detector *monitor.CrashDetector) {
	spm.crashDetector = detector
}

func (spm *SuperPositionManager) GetSlots() *sync.Map {
	return &spm.slots
}

// GetCurrentPriceInterval 获取当前有效的价格间距
// 如果启用了动态网格，返回动态计算的间距；否则返回配置的固定间距
func (spm *SuperPositionManager) GetCurrentPriceInterval(currentPrice float64) float64 {
	if spm.dynamicGridCalc != nil && spm.dynamicGridCalc.IsEnabled() {
		return spm.dynamicGridCalc.CalculateDynamicInterval(currentPrice)
	}
	return spm.config.Trading.PriceInterval
}

// Initialize 初始化管理器（设置价格锚点并创建初始槽位）
func (spm *SuperPositionManager) Initialize(initialPrice float64, initialPriceStr string) error {
	spm.mu.Lock()
	defer spm.mu.Unlock()

	if initialPrice <= 0 {
		return fmt.Errorf("初始价格无效: %.2f", initialPrice)
	}

	// 1. 设置价格锚点（精度信息已经在构造函数中设置，从交易所获取）
	spm.anchorPrice = initialPrice
	spm.lastMarketPrice.Store(initialPrice) // 初始化最后市场价格
	logger.Info("✅ 价格锚点已设置: %s, 价格精度:%d, 数量精度:%d",
		formatPrice(initialPrice, spm.priceDecimals), spm.priceDecimals, spm.quantityDecimals)

	// 2. 直接使用锚点价格作为网格价格（不再对齐到整数）
	initialGridPrice := spm.anchorPrice
	logger.Info("✅ 初始网格价格: %s (使用锚点价格)", formatPrice(initialGridPrice, spm.priceDecimals))

	// 4. 使用统一的槽位价格计算方法创建初始槽位
	slotPrices := spm.calculateSlotPrices(initialGridPrice, spm.config.Trading.BuyWindowSize, "down")
	for _, price := range slotPrices {
		spm.getOrCreateSlot(price)
	}
	// 格式化槽位价格用于日志输出
	slotPricesStr := make([]string, len(slotPrices))
	for i, p := range slotPrices {
		slotPricesStr[i] = formatPrice(p, spm.priceDecimals)
	}
	logger.Info("✅ [初始化] 计算出的槽位价格: %v", slotPricesStr)

	// 5. 为初始槽位下买单
	err := spm.placeInitialBuyOrders()
	if err == nil {
		// 标记为已初始化
		spm.isInitialized.Store(true)
		logger.Info("✅ 初始化完成，网格价格: %s", formatPrice(initialGridPrice, spm.priceDecimals))
	}
	return err
}

// generateClientOrderID 生成自定义订单ID
// 使用新的紧凑格式，最大长度不超过18字符
// 格式: {price_int}_{side}_{timestamp}{seq}
// price_int: price * 10^decimals (转为整数)
// side: B=Buy, S=Sell
func (spm *SuperPositionManager) generateClientOrderID(price float64, side string) string {
	// 使用统一的 utils 包生成紧凑ID
	return utils.GenerateOrderID(price, side, spm.priceDecimals)
}

// parseClientOrderID 解析 ClientOrderID
// 返回: price, side, valid
func (spm *SuperPositionManager) parseClientOrderID(clientOrderID string) (float64, string, bool) {
	// 1. 先移除交易所前缀
	exchangeName := strings.ToLower(spm.exchange.GetName())
	cleanID := utils.RemoveBrokerPrefix(exchangeName, clientOrderID)

	// 2. 使用统一的 utils 包解析
	price, side, _, valid := utils.ParseOrderID(cleanID, spm.priceDecimals)
	if !valid {
		return 0, "", false
	}

	// 🔥 关键修复：不要对从ClientOrderID解析出的价格进行四舍五入！
	// 因为价格本身就是从整数还原的，已经是精确的值
	// 如果再次四舍五入，可能因为浮点数精度问题导致多个不同价格被映射到同一个槽位
	// 例如: 3116.85 和 3114.85 可能都被四舍五入成同一个值

	return price, side, true
}

// placeInitialBuyOrders 设定初始槽位（并恢复持仓槽位）
func (spm *SuperPositionManager) placeInitialBuyOrders() error {
	// 🔥 修改：只恢复持仓槽位，不再主动下单
	// 所有下单操作由 AdjustOrders 统一处理，避免时序问题
	existingPosition := spm.getExistingPosition()
	if existingPosition > 0 {
		logger.Info("🔄 [持仓恢复] 检测到现有持仓: %.4f，开始初始化卖单槽位", existingPosition)
		spm.initializeSellSlotsFromPosition(existingPosition)
	}

	logger.Info("✅ [初始化] 槽位已创建，订单下达将由 AdjustOrders 统一处理")
	return nil
}

// AdjustOrders 调整订单（交易入口）
func (spm *SuperPositionManager) AdjustOrders(currentPrice float64) error {
	// 🔥 移除初始化检查：现在完全由 AdjustOrders 控制所有下单
	// 初始化只负责恢复持仓状态，不再下单

	spm.mu.Lock()
	defer spm.mu.Unlock()

	// 验证价格有效性
	if currentPrice <= 0 {
		logger.Warn("⚠️ 收到无效价格: %.2f，跳过订单调整", currentPrice)
		return nil
	}

	// 对当前价格进行精度处理
	currentPrice = roundPrice(currentPrice, spm.priceDecimals)

	// 更新最后市场价格（用于打印状态）
	spm.lastMarketPrice.Store(currentPrice)

	// 检查保证金不足状态
	if spm.insufficientMargin {
		if time.Since(spm.marginLockTime) >= spm.marginLockDuration {
			logger.Info("✅ [保证金恢复] 锁定时间已过，恢复下单功能")
			spm.insufficientMargin = false
		} else {
			remainingTime := spm.marginLockDuration - time.Since(spm.marginLockTime)
			logger.Warn("⏸️ [暂停下单] 保证金不足，暂停下单中... (剩余时间: %.0f秒)", remainingTime.Seconds())
			return nil
		}
	}

	// 计算需要监控的价格范围
	buyWindowSize := spm.config.Trading.BuyWindowSize
	sellWindowSize := spm.config.Trading.SellWindowSize

	// 🔥 阴跌检测：调整买单窗口大小和买入数量
	buyMultiplier := 1.0
	if spm.downtrendDetector != nil && spm.downtrendDetector.IsEnabled() {
		buyMultiplier = spm.downtrendDetector.GetBuyMultiplier()
		windowRatio := spm.downtrendDetector.GetWindowRatio()
		if windowRatio < 1.0 {
			// 严重阴跌时减少买单窗口
			buyWindowSize = int(float64(buyWindowSize) * windowRatio)
			if buyWindowSize < 1 {
				buyWindowSize = 1
			}
		}
	}

	// 🔥 使用动态网格间距（如果启用）
	priceInterval := spm.GetCurrentPriceInterval(currentPrice)

	// 动态计算网格价格（使用动态间距）
	currentGridPrice := spm.findNearestGridPriceWithInterval(currentPrice, priceInterval)
	// logger.Debug("🔄 [实时调整] 当前价格: %s, 网格价格: %s, 买单窗口: %d, 卖单窗口: %d",
	// 	formatPrice(currentPrice, spm.priceDecimals), formatPrice(currentGridPrice, spm.priceDecimals), buyWindowSize, sellWindowSize)

	// 计算当前网格价格下方buy_window_size个价格（使用动态间距）
	slotPrices := spm.calculateSlotPricesWithInterval(currentGridPrice, buyWindowSize, "down", priceInterval)

	var ordersToPlace []*OrderRequest
	var activeBuyOrdersInWindow int

	// 统计当前所有订单数量（分别统计买单和卖单）
	var currentOrderCount int
	var currentBuyOrderCount int
	var currentSellOrderCount int
	spm.slots.Range(func(key, value interface{}) bool {
		slot := value.(*InventorySlot)
		slot.mu.RLock()
		if slot.OrderStatus == OrderStatusPlaced || slot.OrderStatus == OrderStatusConfirmed ||
			slot.OrderStatus == OrderStatusPartiallyFilled {
			currentOrderCount++
			if slot.OrderSide == "BUY" {
				currentBuyOrderCount++
			} else if slot.OrderSide == "SELL" {
				currentSellOrderCount++
			}
		}
		slot.mu.RUnlock()
		return true
	})

	// 计算允许创建的订单数量上限
	threshold := spm.config.Trading.OrderCleanupThreshold
	if threshold <= 0 {
		threshold = 100
	}

	// 🔥 核心改进：不预留空间，允许订单数达到threshold上限
	// 剩余可用订单数 = 阈值 - 当前订单数
	remainingOrders := threshold - currentOrderCount
	if remainingOrders < 0 {
		remainingOrders = 0
	}

	// 买单允许的新增数量
	allowedNewBuyOrders := buyWindowSize
	if allowedNewBuyOrders > remainingOrders {
		allowedNewBuyOrders = remainingOrders
	}

	// 1. 处理买单
	buyOrdersToCreate := 0

	for _, price := range slotPrices {
		slot := spm.getOrCreateSlot(price)
		slot.mu.Lock()

		// 🔥 槽位锁定检查：如果槽位正在被操作，跳过
		if slot.SlotStatus != SlotStatusFree {
			slot.mu.Unlock()
			continue
		}

		// 检查是否已有有效订单
		hasActiveOrder := false
		if slot.OrderStatus == OrderStatusPlaced || slot.OrderStatus == OrderStatusConfirmed ||
			slot.OrderStatus == OrderStatusPartiallyFilled {
			hasActiveOrder = true
			if slot.OrderSide == "BUY" {
				activeBuyOrdersInWindow++
			}
		}

		// 🔥 买单条件：持仓状态=EMPTY + 槽位锁=FREE + 无订单ID + 无ClientOID
		if slot.PositionStatus != PositionStatusEmpty {
			slot.mu.Unlock()
			continue
		}

		// 🔥 新逻辑：只检查槽位锁状态、OrderID和ClientOID，不检查OrderSide
		shouldCreateBuyOrder := !hasActiveOrder &&
			slot.SlotStatus == SlotStatusFree &&
			slot.OrderID == 0 &&
			slot.ClientOID == "" &&
			buyOrdersToCreate < allowedNewBuyOrders

		if shouldCreateBuyOrder {
			// 安全检查：买单价格不应高于当前价格
			safetyBuffer := spm.config.Trading.PriceInterval * 0.1
			if price >= currentPrice-safetyBuffer {
				slot.mu.Unlock()
				continue
			}

			quantity := spm.config.Trading.OrderQuantity / price
			// 🔥 阴跌检测：应用买入数量乘数
			quantity = quantity * buyMultiplier
			// 使用从交易所获取的数量精度
			quantity = roundPrice(quantity, spm.quantityDecimals)

			// 🔥 最小名义价值检查（Binance 要求 >= 5 USDT）
			orderValue := price * quantity
			minValue := spm.config.Trading.MinOrderValue
			if minValue <= 0 {
				minValue = 6.0 // 默认6U，略高于Binance的5U要求
			}
			if orderValue < minValue {
				logger.Debug("⏭️ [跳过买单] 价格 %s 名义价值 %.2f < %.2f，不满足最小订单要求",
					formatPrice(price, spm.priceDecimals), orderValue, minValue)
				slot.SlotStatus = SlotStatusFree // 释放槽位锁
				slot.mu.Unlock()
				continue
			}

			// 生成 ClientOrderID
			clientOID := spm.generateClientOrderID(price, "BUY")

			// 🔥 锁定槽位：标记为PENDING状态，防止并发操作
			slot.SlotStatus = SlotStatusPending

			// 检查PostOnly失败计数，失败3次后不再使用PostOnly
			usePostOnly := slot.PostOnlyFailCount < 3

			ordersToPlace = append(ordersToPlace, &OrderRequest{
				Symbol:        spm.config.Trading.Symbol,
				Side:          "BUY",
				Price:         price,
				Quantity:      quantity,
				PriceDecimals: spm.priceDecimals,
				PostOnly:      usePostOnly,
				ClientOrderID: clientOID,
			})
			buyOrdersToCreate++
		}

		slot.mu.Unlock()
	}

	// 2. 处理卖单
	sellWindowMaxPrice := currentPrice + float64(sellWindowSize)*priceInterval
	sellWindowMaxPrice = roundPrice(sellWindowMaxPrice, spm.priceDecimals)

	type sellCandidate struct {
		SlotPrice     float64 // 槽位价格 (买入价)
		SellPrice     float64 // 目标卖出价
		Quantity      float64
		DistanceToMid float64
	}
	var sellCandidates []sellCandidate

	spm.slots.Range(func(key, value interface{}) bool {
		slotPrice := key.(float64) // 槽位Key = 买入价
		slot := value.(*InventorySlot)
		slot.mu.Lock()
		defer slot.mu.Unlock()

		// 🔥 卖单条件：持仓状态=FILLED + 槽位锁=FREE + 无订单ID + 无ClientOID
		if slot.PositionStatus == PositionStatusFilled &&
			slot.SlotStatus == SlotStatusFree &&
			slot.OrderID == 0 &&
			slot.ClientOID == "" {

			sellPrice := slotPrice + priceInterval
			sellPrice = roundPrice(sellPrice, spm.priceDecimals)

			// 窗口检查
			if slotPrice > sellWindowMaxPrice {
				return true
			}

			// 最小名义价值检查
			orderValue := sellPrice * slot.PositionQty
			minValue := spm.config.Trading.MinOrderValue
			if minValue <= 0 {
				minValue = 6.0
			}

			if orderValue >= minValue {
				distance := math.Abs(slotPrice - currentPrice)
				sellCandidates = append(sellCandidates, sellCandidate{
					SlotPrice:     slotPrice,
					SellPrice:     sellPrice,
					Quantity:      slot.PositionQty,
					DistanceToMid: distance,
				})
			}
		}
		return true
	})

	// 按距离排序
	sort.Slice(sellCandidates, func(i, j int) bool {
		return sellCandidates[i].DistanceToMid < sellCandidates[j].DistanceToMid
	})

	// 🔥 重新计算卖单的剩余配额（扣除新增买单后的剩余空间）
	remainingOrdersForSell := threshold - currentOrderCount - buyOrdersToCreate
	if remainingOrdersForSell < 0 {
		remainingOrdersForSell = 0
	}

	allowedNewSellOrders := sellWindowSize
	if allowedNewSellOrders > remainingOrdersForSell {
		allowedNewSellOrders = remainingOrdersForSell
	}

	// 生成卖单请求
	sellOrdersToCreate := 0
	// 🔥 调试日志: 显示订单配额计算详情（包含买卖单分布）
	logger.Debug("📊 [订单配额] 阈值:%d, 当前订单:%d(买:%d/卖:%d), 剩余:%d, 新增买单:%d, 卖单候选:%d, 允许卖单:%d",
		threshold, currentOrderCount, currentBuyOrderCount, currentSellOrderCount, remainingOrders, buyOrdersToCreate, len(sellCandidates), allowedNewSellOrders)
	if allowedNewSellOrders > 0 {
		for i := 0; i < len(sellCandidates) && sellOrdersToCreate < allowedNewSellOrders; i++ {
			candidate := sellCandidates[i]

			// 🔥 关键修复：最终验证PositionStatus必须为FILLED且有持仓，并且SlotStatus为FREE
			slot := spm.getOrCreateSlot(candidate.SlotPrice)
			slot.mu.Lock()

			// 🔥 双重检查：确保槽位仍然是FREE状态
			if slot.SlotStatus != SlotStatusFree {
				slot.mu.Unlock()
				continue
			}

			currentStatus := slot.PositionStatus
			currentQty := slot.PositionQty

			if currentStatus != PositionStatusFilled || currentQty <= 0 {
				slot.mu.Unlock()
				continue
			}

			// 🔥 立即锁定槽位：标记为PENDING状态，防止并发操作
			slot.SlotStatus = SlotStatusPending
			// 检查PostOnly失败计数，失败3次后不再使用PostOnly
			usePostOnly := slot.PostOnlyFailCount < 3
			slot.mu.Unlock()

			// 生成 ClientOrderID (注意：使用 SlotPrice 即买入价作为标识)
			clientOID := spm.generateClientOrderID(candidate.SlotPrice, "SELL")

			ordersToPlace = append(ordersToPlace, &OrderRequest{
				Symbol:        spm.config.Trading.Symbol,
				Side:          "SELL",
				Price:         candidate.SellPrice,
				Quantity:      candidate.Quantity,
				PriceDecimals: spm.priceDecimals,
				ReduceOnly:    true,
				PostOnly:      usePostOnly,
				ClientOrderID: clientOID, // 🔥
			})
			sellOrdersToCreate++
		}
	}

	// 3. 处理做空网格（在锚点1.2倍~3倍区域挂空单）
	shortOrdersCreated := 0
	if spm.crashDetector != nil && spm.crashDetector.IsEnabled() && spm.crashDetector.ShouldOpenShort() {
		shortOrdersCreated = spm.handleShortGrid(currentPrice, priceInterval, remainingOrders-buyOrdersToCreate-sellOrdersToCreate, &ordersToPlace)
	}

	// 4. 处理平空仓（买入平仓）
	closeShortOrdersCreated := 0
	if spm.crashDetector != nil && spm.crashDetector.IsEnabled() {
		closeShortOrdersCreated = spm.handleCloseShort(currentPrice, priceInterval, remainingOrders-buyOrdersToCreate-sellOrdersToCreate-shortOrdersCreated, &ordersToPlace)
	}

	// 执行下单
	if len(ordersToPlace) > 0 {
		logger.Debug("🔄 [实时调整] 需要新增: %d 个订单 (买:%d, 卖:%d, 开空:%d, 平空:%d)", 
			len(ordersToPlace), buyOrdersToCreate, sellOrdersToCreate, shortOrdersCreated, closeShortOrdersCreated)
		placedOrders, marginError := spm.executor.BatchPlaceOrders(ordersToPlace)

		if marginError {
			logger.Warn("⚠️ [保证金不足] 检测到保证金不足错误，暂停下单 %d 秒", int(spm.marginLockDuration.Seconds()))
			spm.insufficientMargin = true
			spm.marginLockTime = time.Now()
			spm.CancelAllBuyOrders()
		}

		// 🔥 构建成功订单的ClientOrderID集合
		placedClientOIDs := make(map[string]bool)
		for _, ord := range placedOrders {
			placedClientOIDs[ord.ClientOrderID] = true
		}

		// 🔥 释放未成功提交订单的槽位锁
		for _, req := range ordersToPlace {
			if !placedClientOIDs[req.ClientOrderID] {
				// 这个订单没有成功提交，需要释放槽位锁
				price, _, valid := spm.parseClientOrderID(req.ClientOrderID)
				if valid {
					slot := spm.getOrCreateSlot(price)
					slot.mu.Lock()
					if slot.SlotStatus == SlotStatusPending {
						slot.SlotStatus = SlotStatusFree
						logger.Debug("🔓 [释放槽位] 订单提交失败，释放槽位 %s 的锁 (ClientOID: %s)",
							formatPrice(price, spm.priceDecimals), req.ClientOrderID)
					}
					slot.mu.Unlock()
				}
			}
		}

		for _, ord := range placedOrders {
			// 解析 ClientOrderID
			price, side, valid := spm.parseClientOrderID(ord.ClientOrderID)

			if !valid {
				logger.Warn("⚠️ [实时调整] 无法解析 ClientOID: %s", ord.ClientOrderID)
				continue
			}

			// 获取槽位 (注意：无论是买单还是卖单，ID中编码的都是 SlotPrice)
			slot := spm.getOrCreateSlot(price)
			slot.mu.Lock()

			// 🔥 关键修复：检查是否是秒成交场景（买单或卖单都可能）
			// 秒成交的特征:
			// 1. 买单秒成交: PositionStatus=LONG (刚成交) 且 OrderID=0 (已被WebSocket清空) 且 OrderSide=""
			// 2. 卖单秒成交: PositionStatus=EMPTY (已清空) 且 OrderID=0 (已被WebSocket清空) 且 OrderSide=""
			isInstantFill := false
			if side == "BUY" {
				// 买单秒成交: 有持仓但订单ID为0且OrderSide已清空
				isInstantFill = (slot.PositionStatus == PositionStatusFilled && slot.OrderID == 0 && slot.OrderSide == "")
			} else if side == "SELL" {
				// 🔥 卖单秒成交: 持仓已清空且订单ID为0且OrderSide已清空
				isInstantFill = (slot.PositionStatus == PositionStatusEmpty && slot.OrderID == 0 && slot.OrderSide == "" && slot.SlotStatus == SlotStatusFree)
			}

			if !isInstantFill {
				// 正常情况: 更新订单状态
				// 🔥 检查OrderID冲突：只有当ClientOID已设置且不匹配时才是真正的冲突
				// 如果ClientOID为空或匹配，说明是正常的WebSocket先到或批量处理顺序问题
				if slot.OrderID != 0 && slot.OrderID != ord.OrderID {
					if slot.ClientOID != "" && slot.ClientOID != ord.ClientOrderID {
						// 真正的冲突：槽位已被其他订单占用
						logger.Warn("⚠️ [OrderID冲突] 槽位 %.2f: 下单返回OrderID=%d (ClientOID=%s)，但槽位已被OrderID=%d (ClientOID=%s)占用",
							price, ord.OrderID, ord.ClientOrderID, slot.OrderID, slot.ClientOID)
					} else {
						// WebSocket推送先到达，这是正常现象
						logger.Debug("📝 [覆盖OrderID] 槽位 %.2f: WebSocket已设置OrderID=%d，现用下单返回的OrderID=%d (ClientOID: %s)",
							price, slot.OrderID, ord.OrderID, ord.ClientOrderID)
					}
				}

				slot.OrderID = ord.OrderID
				slot.ClientOID = ord.ClientOrderID
				slot.OrderSide = side // "BUY" or "SELL"
				slot.OrderStatus = OrderStatusPlaced
				slot.OrderPrice = ord.Price
				slot.OrderCreatedAt = time.Now()
				// 🔥 订单提交成功，设置为LOCKED状态
				slot.SlotStatus = SlotStatusLocked
				// 注意：不在这里重置PostOnlyFailCount，因为订单可能立即被撤销
				// PostOnly计数只在订单真正成交时重置

				logger.Debug("✅ [实时新增] 槽位价格: %s, %s订单, 订单价格: %s, 订单ID: %d, ClientOID: %s",
					formatPrice(price, spm.priceDecimals), side, formatPrice(ord.Price, spm.priceDecimals), ord.OrderID, ord.ClientOrderID)
			} else {
				// 🔍 秒成交场景：WebSocket已经处理了FILLED,跳过状态更新
				logger.Debug("🔍 [%s单秒成交] 槽位 %s 的订单已被WebSocket处理，跳过状态更新 (持仓: %.4f, SlotStatus: %s)",
					side, formatPrice(price, spm.priceDecimals), slot.PositionQty, slot.SlotStatus)
			}

			slot.mu.Unlock()
		}
	}

	return nil
}

// OnOrderUpdate 订单更新回调（异步订单同步流）
func (spm *SuperPositionManager) OnOrderUpdate(update OrderUpdate) {
	// 🔥 重构：完全依赖 ClientOrderID 解析
	price, side, valid := spm.parseClientOrderID(update.ClientOrderID)

	if !valid {
		logger.Debug("⏳ [忽略] 无法识别的订单更新: ID=%d, ClientOID=%s", update.OrderID, update.ClientOrderID)
		return
	}

	slot := spm.getOrCreateSlot(price)
	slot.mu.Lock()
	defer slot.mu.Unlock()

	// 校验：确保这个更新属于当前的订单 (防止旧订单的延迟推送干扰新订单)
	// 优先使用 ClientOrderID 匹配 (某些交易所如 Gate.io 的 OrderID 可能略有差异)
	if slot.ClientOID != "" && slot.ClientOID != update.ClientOrderID {
		// ClientOrderID 不匹配，忽略此更新
		logger.Info("⚠️ [订单更新被忽略] 槽位 %.2f: ClientOID不匹配 (槽位: %s, 推送: %s, OrderID: %d)",
			price, slot.ClientOID, update.ClientOrderID, update.OrderID)
		return
	}

	// 更新订单ID (如果是首个推送)
	if slot.OrderID == 0 {
		logger.Debug("📝 [首次设置OrderID] 槽位 %.2f: OrderID=%d, ClientOID=%s", price, update.OrderID, update.ClientOrderID)
		slot.OrderID = update.OrderID
		slot.ClientOID = update.ClientOrderID
		slot.OrderSide = side
	} else if slot.OrderID != update.OrderID {
		// OrderID 不一致但 ClientOrderID 匹配，更新 OrderID (Gate.io 批量下单可能出现此情况)
		logger.Debug("📝 [更新OrderID] 槽位 %.2f: %d -> %d (ClientOID: %s)", price, slot.OrderID, update.OrderID, update.ClientOrderID)
		slot.OrderID = update.OrderID
	}

	// 处理状态转换
	switch update.Status {
	case "NEW":
		if slot.OrderStatus == OrderStatusPlaced {
			slot.OrderStatus = OrderStatusConfirmed
		}

	case "PARTIALLY_FILLED", "FILLED":
		// 计算增量
		deltaQty := update.ExecutedQty - slot.OrderFilledQty
		if deltaQty < 0 {
			deltaQty = 0
		}

		slot.OrderFilledQty = update.ExecutedQty

		// 根据方向更新持仓
		if side == "BUY" {
			if deltaQty > 0 {
				slot.PositionQty += deltaQty
				// 累加统计
				oldTotal := spm.totalBuyQty.Load().(float64)
				spm.totalBuyQty.Store(oldTotal + deltaQty)
			}

			if update.Status == "FILLED" {
				slot.OrderStatus = OrderStatusNotPlaced // 重置订单状态
				slot.OrderID = 0
				slot.ClientOID = ""
				slot.OrderSide = "" // 🔥 清除订单方向，避免误判
				slot.OrderFilledQty = 0

				// 🔥 修复：简化持仓状态判断
				if slot.PositionQty > 0.000001 {
					// 正数持仓 = 多仓
					slot.PositionStatus = PositionStatusFilled
					logger.Info("✅ [买单成交] 价格: %s, 持仓: %.4f (多仓)",
						formatPrice(price, spm.priceDecimals), slot.PositionQty)
				} else {
					// 持仓为0或负数 = 空仓位
					slot.PositionStatus = PositionStatusEmpty
					logger.Info("✅ [平仓完成] 价格: %s, 持仓已清空",
						formatPrice(price, spm.priceDecimals))
				}
				// 🔥 释放槽位锁：买单成交，允许后续挂卖单
				slot.SlotStatus = SlotStatusFree
				// 🔥 买单成交，重置PostOnly失败计数
				slot.PostOnlyFailCount = 0
				logger.Debug("🔍 [买单成交后] 等待下次AdjustOrders调用时挂出卖单...")
			} else {
				slot.OrderStatus = OrderStatusPartiallyFilled
			}

		} else { // SELL
			if deltaQty > 0 {
				slot.PositionQty -= deltaQty
				if slot.PositionQty < 0 {
					slot.PositionQty = 0
				}
				// 累加统计
				oldTotal := spm.totalSellQty.Load().(float64)
				spm.totalSellQty.Store(oldTotal + deltaQty)
			}

			if update.Status == "FILLED" {
				slot.OrderStatus = OrderStatusNotPlaced // 重置订单状态
				slot.OrderID = 0
				slot.ClientOID = ""
				slot.OrderSide = "" // 🔥 清除订单方向，避免误判
				slot.OrderFilledQty = 0

				// 🔥 修复：简化持仓状态判断
				if slot.PositionQty > 0.000001 {
					// 正数持仓 = 多仓（卖出减仓后剩余）
					slot.PositionStatus = PositionStatusFilled
					logger.Info("✅ [卖单成交] 价格: %s, 剩余持仓: %.4f (多仓)",
						formatPrice(price, spm.priceDecimals), slot.PositionQty)
				} else {
					// 持仓为0或负数 = 空仓位（平仓完成）
					slot.PositionStatus = PositionStatusEmpty
					logger.Info("✅ [平仓完成] 价格: %s, 持仓已清空",
						formatPrice(price, spm.priceDecimals))
				}

				// 🔥 释放槽位锁：卖单成交，允许后续操作
				slot.SlotStatus = SlotStatusFree
				// 🔥 卖单成交，重置PostOnly失败计数
				slot.PostOnlyFailCount = 0
			} else {
				slot.OrderStatus = OrderStatusPartiallyFilled
			}
		}

	case "CANCELED", "EXPIRED", "REJECTED":
		logger.Info("⚠️ [订单%s] 价格: %s, 方向: %s, 原因: %s, 已成交: %.4f",
			update.Status, formatPrice(price, spm.priceDecimals), side, update.Status, slot.OrderFilledQty)

		// 🔥 核心修复：根据订单方向和成交情况处理槽位状态
		if side == "BUY" {
			// 买单被取消/拒绝
			if slot.PositionQty > 0 || slot.OrderFilledQty > 0 {
				// 部分成交后被取消：保留持仓，允许后续挂卖单
				logger.Info("💡 [买单部分成交后取消] 价格: %s, 持仓: %.4f, 转为多仓状态",
					formatPrice(price, spm.priceDecimals), slot.PositionQty)
				slot.PositionStatus = PositionStatusFilled
				slot.SlotStatus = SlotStatusFree // 允许挂卖单
			} else {
				// 完全未成交被取消：重置为空槽位
				logger.Info("🔄 [买单未成交取消] 价格: %s, 重置槽位为空闲",
					formatPrice(price, spm.priceDecimals))
				slot.PositionStatus = PositionStatusEmpty
				slot.SlotStatus = SlotStatusFree // 允许重新挂买单
			}
		} else if side == "SELL" {
			// 卖单被取消/拒绝：应该还持有币，保持持仓状态
			if slot.PositionQty > 0 {
				// 增加PostOnly失败计数（订单被交易所撤销通常是PostOnly失败）
				slot.PostOnlyFailCount++
				logger.Info("🔄 [卖单取消] 价格: %s, 保持持仓状态: %.4f, 等待重挂, PostOnly失败计数: %d",
					formatPrice(price, spm.priceDecimals), slot.PositionQty, slot.PostOnlyFailCount)
				slot.PositionStatus = PositionStatusFilled
				slot.SlotStatus = SlotStatusFree // 允许重新挂卖单
			} else {
				// 异常情况：卖单取消但没有持仓，重置为空
				logger.Warn("⚠️ [异常] 卖单取消但无持仓，价格: %s, 重置为空",
					formatPrice(price, spm.priceDecimals))
				slot.PositionStatus = PositionStatusEmpty
				slot.SlotStatus = SlotStatusFree
			}
		}

		// 清空订单信息
		slot.OrderStatus = OrderStatusCanceled
		slot.OrderID = 0
		slot.ClientOID = ""
		slot.OrderFilledQty = 0
		// 保留 OrderSide 用于日志调试
	}
}

// getOrCreateSlot 获取或创建槽位
func (spm *SuperPositionManager) getOrCreateSlot(price float64) *InventorySlot {
	if slot, exists := spm.slots.Load(price); exists {
		return slot.(*InventorySlot)
	}

	// 创建新槽位
	slot := &InventorySlot{
		Price:          price,
		PositionStatus: PositionStatusEmpty,
		PositionQty:    0,
		OrderStatus:    OrderStatusNotPlaced,
		SlotStatus:     SlotStatusFree, // 🔥 初始化为FREE状态
	}
	spm.slots.Store(price, slot)
	return slot
}

// findNearestGridPrice 找到最近的网格价格
// 根据当前价格动态计算最近的网格对齐价格
func (spm *SuperPositionManager) findNearestGridPrice(currentPrice float64) float64 {
	return spm.findNearestGridPriceWithInterval(currentPrice, 0)
}

// findNearestGridPriceWithInterval 找到最近的网格价格（支持自定义间距）
func (spm *SuperPositionManager) findNearestGridPriceWithInterval(currentPrice float64, customInterval float64) float64 {
	// 使用自定义间距或配置的固定间距
	priceInterval := customInterval
	if priceInterval <= 0 {
		priceInterval = spm.config.Trading.PriceInterval
	}

	// 计算当前价格相对于锚点的偏移量
	offset := currentPrice - spm.anchorPrice
	// 计算离当前价格最近的网格间隔数（四舍五入）
	intervals := math.Round(offset / priceInterval)
	// 计算最近的网格价格
	gridPrice := spm.anchorPrice + intervals*priceInterval
	// 使用检测到的价格精度进行舍入
	return roundPrice(gridPrice, spm.priceDecimals)
}

// calculateSlotPrices 计算槽位价格列表（统一的网格计算方法）
// 这个方法确保初始化和实时调整计算出完全相同的槽位价格
// 参数：
//   - gridPrice: 网格价格（使用锚点价格）
//   - count: 需要计算的槽位数量
//   - direction: 方向，"down"表示向下（买单），"up"表示向上（卖单）
//
// 返回：槽位价格列表，从网格价格开始，按价格间隔递减或递增，使用检测到的价格精度
func (spm *SuperPositionManager) calculateSlotPrices(gridPrice float64, count int, direction string) []float64 {
	return spm.calculateSlotPricesWithInterval(gridPrice, count, direction, 0)
}

// calculateSlotPricesWithInterval 计算槽位价格列表（支持自定义间距）
// 如果 customInterval <= 0，则使用配置的固定间距
func (spm *SuperPositionManager) calculateSlotPricesWithInterval(gridPrice float64, count int, direction string, customInterval float64) []float64 {
	var prices []float64

	// 使用自定义间距或配置的固定间距
	priceInterval := customInterval
	if priceInterval <= 0 {
		priceInterval = spm.config.Trading.PriceInterval
	}

	for i := 0; i < count; i++ {
		var price float64
		if direction == "down" {
			// 向下：网格价格 - i * 间隔
			price = gridPrice - float64(i)*priceInterval
		} else {
			// 向上：网格价格 + i * 间隔
			price = gridPrice + float64(i)*priceInterval
		}
		// 使用检测到的价格精度进行舍入
		price = roundPrice(price, spm.priceDecimals)
		prices = append(prices, price)
	}

	return prices
}

// ===== IPositionManager 接口实现（供 safety.Reconciler 使用）=====
// 注意：以下方法是 safety/reconciler.go 中 IPositionManager 接口的实现，
// 被 Reconciler 对账器调用，不可删除或修改签名

// SlotData 槽位数据结构（用于传递给外部）
type SlotData struct {
	Price          float64
	PositionStatus string
	PositionQty    float64
	OrderID        int64
	OrderSide      string
	OrderStatus    string
	OrderCreatedAt time.Time
}

// IterateSlots 遍历所有槽位（封装 sync.Map.Range）
// 注意：为了避免类型冲突，这里使用 interface{} 返回槽位数据
// 调用者需要将其转换为具体的槽位信息
func (spm *SuperPositionManager) IterateSlots(fn func(price float64, slot interface{}) bool) {
	spm.slots.Range(func(key, value interface{}) bool {
		price := key.(float64)
		slot := value.(*InventorySlot)
		slot.mu.RLock()
		defer slot.mu.RUnlock()

		// 构造槽位数据
		data := SlotData{
			Price:          price,
			PositionStatus: slot.PositionStatus,
			PositionQty:    slot.PositionQty,
			OrderID:        slot.OrderID,
			OrderSide:      slot.OrderSide,
			OrderStatus:    slot.OrderStatus,
			OrderCreatedAt: slot.OrderCreatedAt,
		}

		// 返回槽位数据
		return fn(price, data)
	})
}

// GetTotalBuyQty 获取累计买入数量（IPositionManager 接口方法，供 Reconciler 使用）
func (spm *SuperPositionManager) GetTotalBuyQty() float64 {
	return spm.totalBuyQty.Load().(float64)
}

// GetTotalSellQty 获取累计卖出数量（IPositionManager 接口方法，供 Reconciler 使用）
func (spm *SuperPositionManager) GetTotalSellQty() float64 {
	return spm.totalSellQty.Load().(float64)
}

// GetReconcileCount 获取对账次数（IPositionManager 接口方法，供 Reconciler 使用）
func (spm *SuperPositionManager) GetReconcileCount() int64 {
	return spm.reconcileCount.Load()
}

// IncrementReconcileCount 增加对账次数（IPositionManager 接口方法，供 Reconciler 使用）
func (spm *SuperPositionManager) IncrementReconcileCount() {
	spm.reconcileCount.Add(1)
}

// UpdateLastReconcileTime 更新最后对账时间（IPositionManager 接口方法，供 Reconciler 使用）
func (spm *SuperPositionManager) UpdateLastReconcileTime(t time.Time) {
	spm.lastReconcileTime.Store(t)
}

// GetSymbol 获取交易符号
func (spm *SuperPositionManager) GetSymbol() string {
	return spm.config.Trading.Symbol
}

// GetPriceInterval 获取价格间隔
func (spm *SuperPositionManager) GetPriceInterval() float64 {
	return spm.config.Trading.PriceInterval
}

// ===== 订单清理功能已迁移到 safety.OrderCleaner =====
// StartOrderCleanup 和 cleanupOrders 方法已移至 safety/order_cleaner.go

// UpdateSlotOrderStatus 更新槽位订单状态（供 OrderCleaner 使用）
func (spm *SuperPositionManager) UpdateSlotOrderStatus(price float64, status string) {
	slot := spm.getOrCreateSlot(price)
	slot.mu.Lock()
	slot.OrderStatus = status
	slot.mu.Unlock()
}

// CancelAllBuyOrders 撤销所有买单（风控触发时使用）
func (spm *SuperPositionManager) CancelAllBuyOrders() {
	var buyOrderIDs []int64
	var buyPrices []float64

	// 🔥 修复：收集所有OrderID>0且OrderSide=BUY的订单，不管OrderStatus
	spm.slots.Range(func(key, value interface{}) bool {
		price := key.(float64)
		slot := value.(*InventorySlot)

		slot.mu.RLock()
		if slot.OrderSide == "BUY" && slot.OrderID > 0 {
			buyOrderIDs = append(buyOrderIDs, slot.OrderID)
			buyPrices = append(buyPrices, price)
		}
		slot.mu.RUnlock()
		return true
	})

	if len(buyOrderIDs) == 0 {
		return
	}

	logger.Info("🔄 [撤销买单] 准备撤销 %d 个买单以释放保证金", len(buyOrderIDs))

	// 🔥 重复尝试3次，确保撤单干净
	for attempt := 1; attempt <= 3; attempt++ {
		if len(buyOrderIDs) == 0 {
			break
		}

		logger.Info("🔄 [撤销买单] 第 %d 次尝试，剩余 %d 个订单", attempt, len(buyOrderIDs))

		if err := spm.executor.BatchCancelOrders(buyOrderIDs); err != nil {
			logger.Error("❌ [撤销买单] 批量撤单失败: %v", err)
		}

		// 更新槽位状态
		for _, price := range buyPrices {
			slot := spm.getOrCreateSlot(price)
			slot.mu.Lock()
			slot.OrderStatus = OrderStatusCancelRequested
			slot.mu.Unlock()
		}

		// 等待2秒让撤单生效（WebSocket推送通知）
		time.Sleep(2 * time.Second)

		// 🔥 二次检查：重新扫描本地槽位状态
		if attempt < 3 {
			buyOrderIDs = nil
			buyPrices = nil

			spm.slots.Range(func(key, value interface{}) bool {
				price := key.(float64)
				slot := value.(*InventorySlot)

				slot.mu.RLock()
				// 如果OrderStatus不是CANCELED且OrderID>0，说明可能还有残留
				if slot.OrderSide == "BUY" && slot.OrderID > 0 &&
					slot.OrderStatus != OrderStatusCanceled {
					buyOrderIDs = append(buyOrderIDs, slot.OrderID)
					buyPrices = append(buyPrices, price)
				}
				slot.mu.RUnlock()
				return true
			})

			if len(buyOrderIDs) > 0 {
				logger.Warn("⚠️ [撤销买单] 检测到 %d 个残留买单，继续清理", len(buyOrderIDs))
			} else {
				logger.Info("✅ [撤销买单] 所有买单已清理完成")
				break
			}
		}
	}

	logger.Info("✅ [撤销买单] 清理完成")
}

// ===== 对账功能已迁移到 safety.Reconciler =====
// StartReconciliation 和 Reconcile 方法已移至 safety/reconciler.go
// SetPauseChecker 也已移至 Reconciler

// CancelAllOrders 撤销所有订单（退出时使用）
// 委托给交易所适配器实现具体逻辑
func (spm *SuperPositionManager) CancelAllOrders() {
	ctx := context.Background()
	if err := spm.exchange.CancelAllOrders(ctx, spm.config.Trading.Symbol); err != nil {
		logger.Error("❌ [%s] 撤销所有订单失败: %v", spm.exchange.GetName(), err)
	} else {
		logger.Info("✅ [%s] 撤销所有订单完成", spm.exchange.GetName())
	}
}

// getExistingPosition 获取当前持仓数量（容错处理）
func (spm *SuperPositionManager) getExistingPosition() float64 {
	ctx := context.Background()
	positionsInterface, err := spm.exchange.GetPositions(ctx, spm.config.Trading.Symbol)
	if err != nil || positionsInterface == nil {
		logger.Debug("🔍 [持仓恢复] 无法获取持仓信息: %v", err)
		return 0
	}

	// 尝试类型断言 - 假设返回的是包含 Size 字段的结构体切片
	// 我们使用反射来安全地提取持仓数量
	switch positions := positionsInterface.(type) {
	case []*PositionInfo:
		// PositionInfo 切片（简化版）
		for _, pos := range positions {
			if pos != nil && pos.Symbol == spm.config.Trading.Symbol {
				logger.Debug("🔍 [持仓恢复] 找到持仓 (PositionInfo): %.4f", pos.Size)
				return pos.Size
			}
		}
	case []interface{}:
		// 通用接口数组 - 尝试解析为持仓结构
		for _, pos := range positions {
			// 尝试直接类型断言为 PositionInfo
			if posInfo, ok := pos.(*PositionInfo); ok {
				if posInfo.Symbol == spm.config.Trading.Symbol {
					logger.Debug("🔍 [持仓恢复] 找到持仓 (interface->PositionInfo): %.4f", posInfo.Size)
					return posInfo.Size
				}
			}
			// 尝试解析为 map
			if posMap, ok := pos.(map[string]interface{}); ok {
				if symbol, ok := posMap["Symbol"].(string); ok && symbol == spm.config.Trading.Symbol {
					if size, ok := posMap["Size"].(float64); ok {
						logger.Debug("🔍 [持仓恢复] 找到持仓 (map): %.4f", size)
						return size
					}
				}
			}
		}
	default:
		// 其他情况：使用反射尝试提取 Size 字段
		logger.Debug("🔍 [持仓恢复] 持仓类型: %T，尝试使用反射提取", positionsInterface)
		// 尝试使用反射处理未知类型
		// 注意：实际上 exchange 返回的是 []*exchange.Position，但因为接口返回 interface{}，所以需要特殊处理
		return 0
	}

	logger.Debug("🔍 [持仓恢复] 未找到匹配的持仓")
	return 0
}

// initializeSellSlotsFromPosition 从现有持仓初始化卖单槽位（用于程序重启后恢复状态）
func (spm *SuperPositionManager) initializeSellSlotsFromPosition(totalPosition float64) {
	if totalPosition <= 0 {
		return
	}

	// 1. 计算每单的理论数量（基于当前价格）
	// 使用锚点价格作为参考价格，使用从交易所获取的数量精度

	// 每单的理论数量 = 目标金额 / 锚点价格
	theoryQtyPerSlot := spm.config.Trading.OrderQuantity / spm.anchorPrice
	theoryQtyPerSlot = roundPrice(theoryQtyPerSlot, spm.quantityDecimals)

	// 2. 计算需要创建的总槽位数
	totalSlotsNeeded := int(math.Ceil(totalPosition / theoryQtyPerSlot))
	logger.Info("🔄 [持仓恢复] 总持仓: %.4f，每单理论数量: %.4f，需要创建 %d 个槽位",
		totalPosition, theoryQtyPerSlot, totalSlotsNeeded)

	// 3. 确定窗口大小（前N个槽位可以立即挂卖单）
	sellWindowSize := spm.config.Trading.SellWindowSize
	if sellWindowSize <= 0 {
		sellWindowSize = spm.config.Trading.BuyWindowSize // 默认与买单窗口相同
	}

	// 4. 计算卖单槽位价格（从锚点价格 + 价格间隔开始）
	// 卖单最低价 = 锚点价格 + 价格间隔（避免与买单最高价冲突）
	// 注意：这里使用 calculateSlotPrices 的 "up" 方向，第一个价格就是 anchorPrice + interval
	sellStartPrice := spm.anchorPrice + spm.config.Trading.PriceInterval
	sellPrices := spm.calculateSlotPrices(sellStartPrice, totalSlotsNeeded, "up")

	logger.Info("🔄 [持仓恢复] 从价格 %s 向上创建 %d 个槽位（前 %d 个将挂卖单）",
		formatPrice(sellStartPrice, spm.priceDecimals), totalSlotsNeeded, sellWindowSize)

	// 5. 先计算所有槽位的理论数量总和（固定金额模式）
	var totalTheoryQty float64
	theoryQtys := make([]float64, len(sellPrices))
	for i, price := range sellPrices {
		theoryQty := spm.config.Trading.OrderQuantity / price
		theoryQty = roundPrice(theoryQty, spm.quantityDecimals)
		theoryQtys[i] = theoryQty
		totalTheoryQty += theoryQty
	}

	logger.Debug("🔍 [持仓恢复] 理论总数量: %.4f, 实际持仓: %.4f, 比例: %.4f",
		totalTheoryQty, totalPosition, totalPosition/totalTheoryQty)

	// 6. 按比例分配实际持仓到各个槽位
	var allocatedQty float64

	for i, price := range sellPrices {
		// 计算这个槽位应该分配的数量
		var slotQty float64
		if i == len(sellPrices)-1 {
			// 最后一个槽位：分配剩余的所有持仓（避免舍入误差）
			slotQty = totalPosition - allocatedQty
		} else {
			// 按比例分配：实际数量 = 理论数量 × (总持仓 / 理论总数量)
			slotQty = theoryQtys[i] * (totalPosition / totalTheoryQty)
			slotQty = roundPrice(slotQty, spm.quantityDecimals)

			// 确保不超过剩余持仓
			remaining := totalPosition - allocatedQty
			if slotQty > remaining {
				slotQty = remaining
			}
		}

		if slotQty <= 0 {
			logger.Warn("⚠️ [持仓恢复] 槽位 %s 分配数量过小 %.4f，跳过（已分配: %.4f / 总计: %.4f）",
				formatPrice(price, spm.priceDecimals), slotQty, allocatedQty, totalPosition)
			continue
		}

		// 7. 创建或更新槽位
		slot := spm.getOrCreateSlot(price)
		slot.mu.Lock()

		// 设置为多仓状态
		slot.PositionStatus = PositionStatusFilled
		slot.PositionQty = slotQty

		// 清空订单信息，但设置方向为SELL（因为这是恢复的持仓，将来要挂卖单）
		slot.OrderID = 0
		slot.OrderStatus = OrderStatusNotPlaced
		slot.OrderSide = "SELL" // 恢复持仓时标记为卖单方向
		slot.ClientOID = ""
		slot.OrderFilledQty = 0

		slot.mu.Unlock()

		allocatedQty += slotQty

		// 日志标记：是否在窗口内（只打印前10个和最后10个）
		if i < 10 || i >= len(sellPrices)-10 {
			inWindow := ""
			if i < sellWindowSize {
				inWindow = " [可挂单]"
			} else {
				inWindow = " [暂不挂单]"
			}
			logger.Info("✅ [持仓恢复] 槽位 %s: 分配持仓 %.4f (理论: %.4f)%s",
				formatPrice(price, spm.priceDecimals), slotQty, theoryQtys[i], inWindow)
		} else if i == 10 {
			logger.Info("... （省略中间 %d 个槽位）", len(sellPrices)-20)
		}
	}

	logger.Info("✅ [持仓恢复] 完成持仓恢复，总持仓: %.4f，已分配: %.4f，差异: %.4f",
		totalPosition, allocatedQty, totalPosition-allocatedQty)

	// 8. 提示用户后续会自动下卖单
	logger.Info("💡 [持仓恢复] 前 %d 个槽位的卖单将在价格调整时自动创建", sellWindowSize)
	logger.Info("💡 [持仓恢复] 其余 %d 个槽位保持有仓状态，价格接近时自动挂单", totalSlotsNeeded-sellWindowSize)
}

// ===== 状态打印功能 =====

// PrintPositions 打印持仓状态（由 main.go 定期调用和退出时调用）
// 注意：该方法内部使用 totalBuyQty 和 totalSellQty 统计数据
func (spm *SuperPositionManager) PrintPositions() {
	logger.Info("📊 ===== 当前持仓 =====")
	total := 0.0
	count := 0

	// 收集所有持仓数据
	type positionInfo struct {
		Price       float64
		Qty         float64
		OrderStatus string
		OrderSide   string
		OrderID     int64
		SlotStatus  string
	}
	var positions []positionInfo

	spm.slots.Range(func(key, value interface{}) bool {
		price := key.(float64)
		slot := value.(*InventorySlot)
		slot.mu.RLock()
		// 只显示多仓
		if slot.PositionStatus == PositionStatusFilled && slot.PositionQty > 0.001 {
			positions = append(positions, positionInfo{
				Price:       price,
				Qty:         slot.PositionQty,
				OrderStatus: slot.OrderStatus,
				OrderSide:   slot.OrderSide,
				OrderID:     slot.OrderID,
				SlotStatus:  slot.SlotStatus,
			})
			total += slot.PositionQty
			count++
		}
		slot.mu.RUnlock()
		return true
	})

	// 按价格从高到低排序
	sort.Slice(positions, func(i, j int) bool {
		return positions[i].Price > positions[j].Price
	})

	// 从交易所接口获取基础币种（支持U本位和币本位合约）
	baseCurrency := spm.exchange.GetBaseAsset()

	// 打印持仓（从高到低）
	for _, pos := range positions {
		// 🔥 中性网格：区分多仓和空仓图标
		statusIcon := "🟢" // 默认多仓
		if pos.Qty < 0 {
			statusIcon = "🔴" // 空仓
		}
		priceStr := formatPrice(pos.Price, spm.priceDecimals)
		positionDesc := fmt.Sprintf("持仓: %.4f %s", pos.Qty, baseCurrency)

		orderInfo := ""
		if pos.OrderStatus != OrderStatusNotPlaced && pos.OrderStatus != "" {
			orderInfo = fmt.Sprintf(", 订单: %s/%s (ID:%d)", pos.OrderSide, pos.OrderStatus, pos.OrderID)
		}

		// 🔥 总是显示槽位状态,便于调试
		slotStatusInfo := ""
		if pos.SlotStatus != "" {
			slotStatusInfo = fmt.Sprintf(" [槽位:%s]", pos.SlotStatus)
		} else {
			slotStatusInfo = " [槽位:空]"
		}

		logger.Info("  %s %s: %s%s%s",
			statusIcon, priceStr, positionDesc, orderInfo, slotStatusInfo)
	}

	logger.Info("持仓统计: %.4f %s (%d 个槽位)", total, baseCurrency, count)
	totalBuyQty := spm.totalBuyQty.Load().(float64)
	totalSellQty := spm.totalSellQty.Load().(float64)

	// 获取最后的市场价格用于计算动态间距
	lastPrice, ok := spm.lastMarketPrice.Load().(float64)
	if !ok || lastPrice <= 0 {
		lastPrice = spm.anchorPrice
	}

	// 获取当前有效的价格间距
	currentInterval := spm.GetCurrentPriceInterval(lastPrice)

	// 预计盈利 = 累计卖出数量 × 价格间距（每笔盈利 = 价格间距 × 数量）
	estimatedProfit := totalSellQty * currentInterval
	logger.Info("累计买入: %.2f, 累计卖出: %.2f, 预计盈利: %.2f U",
		totalBuyQty, totalSellQty, estimatedProfit)

	// 打印动态网格信息（如果启用）
	if spm.dynamicGridCalc != nil && spm.dynamicGridCalc.IsEnabled() {
		base, breakEven, atrBased, final := spm.dynamicGridCalc.GetIntervalComponents(lastPrice)
		logger.Info("📐 [动态网格] 当前间距: %.4f (基础:%.4f, 保本:%.4f, ATR:%.4f)",
			final, base, breakEven, atrBased)
	}

	// 打印阴跌检测状态（如果启用）
	if spm.downtrendDetector != nil && spm.downtrendDetector.IsEnabled() {
		level, ma20, consecutiveDowns, multiplier, windowRatio := spm.downtrendDetector.GetStatus()
		levelIcon := "✅"
		if level == monitor.DowntrendMild {
			levelIcon = "🔻"
		} else if level == monitor.DowntrendSevere {
			levelIcon = "🔻🔻"
		}
		logger.Info("%s [阴跌检测] %s | MA20: %.4f | 连续收阴: %d根 | 买入×%.1f | 窗口×%.1f",
			levelIcon, level.String(), ma20, consecutiveDowns, multiplier, windowRatio)
	}

	// === 新增：打印买单窗口详细信息 ===
	logger.Info("🔍 ===== 买单窗口状态 =====")

	logger.Info("当前市场价格: %s", formatPrice(lastPrice, spm.priceDecimals))

	// 收集所有槽位信息（包括买单和空槽位）
	type slotInfo struct {
		Price          float64
		PositionStatus string
		PositionQty    float64
		OrderSide      string
		OrderStatus    string
		OrderID        int64
		ClientOID      string
		SlotStatus     string
	}
	var allSlots []slotInfo

	spm.slots.Range(func(key, value interface{}) bool {
		price := key.(float64)
		slot := value.(*InventorySlot)
		slot.mu.RLock()
		allSlots = append(allSlots, slotInfo{
			Price:          price,
			PositionStatus: slot.PositionStatus,
			PositionQty:    slot.PositionQty,
			OrderSide:      slot.OrderSide,
			OrderStatus:    slot.OrderStatus,
			OrderID:        slot.OrderID,
			ClientOID:      slot.ClientOID,
			SlotStatus:     slot.SlotStatus,
		})
		slot.mu.RUnlock()
		return true
	})

	// 按价格从高到低排序
	sort.Slice(allSlots, func(i, j int) bool {
		return allSlots[i].Price > allSlots[j].Price
	})

	// 找到最接近当前价格的网格价格
	currentGridPrice := spm.findNearestGridPrice(lastPrice)
	logger.Info("当前网格价格: %s", formatPrice(currentGridPrice, spm.priceDecimals))

	// 计算买单窗口范围（当前网格价格下方的买单窗口）
	buyWindowSize := spm.config.Trading.BuyWindowSize
	buyWindowPrices := spm.calculateSlotPrices(currentGridPrice, buyWindowSize, "down")

	// 创建价格查找表
	buyWindowPriceMap := make(map[string]bool)
	for _, p := range buyWindowPrices {
		buyWindowPriceMap[formatPrice(p, spm.priceDecimals)] = true
	}

	// 打印买单窗口内的所有槽位
	logger.Info("买单窗口大小: %d 个槽位 (当前网格价格下方)", buyWindowSize)
	buyOrderCount := 0
	emptySlotCount := 0
	longSlotCount := 0

	for _, slot := range allSlots {
		priceStr := formatPrice(slot.Price, spm.priceDecimals)
		// 只打印买单窗口内的槽位
		if buyWindowPriceMap[priceStr] {
			statusIcon := "⚪" // 空槽位
			statusDesc := ""

			// 区分多仓和空仓
			if slot.PositionStatus == PositionStatusFilled {
				statusIcon = "🟢" // 多仓
				statusDesc = fmt.Sprintf("多仓: %.4f %s", slot.PositionQty, baseCurrency)
				longSlotCount++
			} else {
				statusDesc = "无持仓"
				emptySlotCount++
			}

			orderInfo := ""
			if slot.OrderStatus != OrderStatusNotPlaced && slot.OrderStatus != "" {
				orderInfo = fmt.Sprintf(", 订单: %s/%s (ID:%d)", slot.OrderSide, slot.OrderStatus, slot.OrderID)
				if slot.OrderSide == "BUY" && (slot.OrderStatus == OrderStatusPlaced ||
					slot.OrderStatus == OrderStatusConfirmed ||
					slot.OrderStatus == OrderStatusPartiallyFilled) {
					buyOrderCount++
				}
			}

			// 🔥 总是显示槽位状态,便于调试
			slotStatusInfo := ""
			if slot.SlotStatus != "" {
				slotStatusInfo = fmt.Sprintf(" [槽位:%s]", slot.SlotStatus)
			} else {
				slotStatusInfo = " [槽位:空]"
			}

			logger.Info("  %s %s: %s%s%s",
				statusIcon, priceStr, statusDesc, orderInfo, slotStatusInfo)
		}
	}

	logger.Info("窗口统计: %d 个买单活跃, %d 个多仓, %d 个空槽位",
		buyOrderCount, longSlotCount, emptySlotCount)
	logger.Info("==========================")
}

// 辅助函数
// roundPrice 价格四舍五入
func roundPrice(price float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Round(price*multiplier) / multiplier
}

// formatPrice 格式化价格字符串，使用指定的小数位数
func formatPrice(price float64, decimals int) string {
	return fmt.Sprintf("%.*f", decimals, price)
}

// ==================== 做空网格逻辑 ====================

// handleShortGrid 处理做空网格（在锚点1.2倍~3倍区域挂空单）
// 返回创建的空单数量
func (spm *SuperPositionManager) handleShortGrid(currentPrice float64, priceInterval float64, remainingOrders int, ordersToPlace *[]*OrderRequest) int {
	if remainingOrders <= 0 {
		return 0
	}

	// 获取做空区域
	anchor, shortZoneMin, shortZoneMax := spm.crashDetector.GetShortZone()
	if anchor <= 0 || shortZoneMin <= 0 {
		return 0
	}

	// 🔥 安全检查：做空区域必须在当前价格上方，避免与做多网格冲突
	if shortZoneMin <= currentPrice {
		logger.Debug("🔍 [做空网格] 做空区域 %.6f <= 当前价格 %.6f，跳过", shortZoneMin, currentPrice)
		return 0
	}

	logger.Debug("🔍 [做空网格] 锚点:%.6f, 做空区域:[%.6f ~ %.6f], 当前价格:%.6f",
		anchor, shortZoneMin, shortZoneMax, currentPrice)

	maxShortPositions := spm.crashDetector.GetMaxShortPositions()

	// 统计当前空仓数量
	currentShortCount := 0
	spm.slots.Range(func(key, value interface{}) bool {
		slot := value.(*InventorySlot)
		slot.mu.RLock()
		if slot.PositionQty < -0.000001 {
			currentShortCount++
		}
		slot.mu.RUnlock()
		return true
	})

	if currentShortCount >= maxShortPositions {
		logger.Debug("🔍 [做空网格] 已达最大空仓数量 %d/%d", currentShortCount, maxShortPositions)
		return 0
	}

	allowedNewShorts := maxShortPositions - currentShortCount
	if allowedNewShorts > remainingOrders {
		allowedNewShorts = remainingOrders
	}

	type shortCandidate struct {
		SlotPrice float64
		Quantity  float64
	}
	var candidates []shortCandidate

	// 生成做空槽位价格
	for price := shortZoneMin; price <= shortZoneMax && len(candidates) < allowedNewShorts; price += priceInterval {
		slotPrice := roundPrice(price, spm.priceDecimals)

		slot := spm.getOrCreateSlot(slotPrice)
		slot.mu.Lock()

		if slot.PositionStatus == PositionStatusEmpty &&
			slot.SlotStatus == SlotStatusFree &&
			slot.OrderID == 0 &&
			slot.ClientOID == "" {

			quantity := spm.config.Trading.OrderQuantity / slotPrice
			quantity = roundPrice(quantity, spm.quantityDecimals)

			orderValue := slotPrice * quantity
			minValue := spm.config.Trading.MinOrderValue
			if minValue <= 0 {
				minValue = 6.0
			}

			if orderValue >= minValue {
				candidates = append(candidates, shortCandidate{
					SlotPrice: slotPrice,
					Quantity:  quantity,
				})
			}
		}
		slot.mu.Unlock()
	}

	// 生成开空仓订单
	shortOrdersCreated := 0
	for _, candidate := range candidates {
		slot := spm.getOrCreateSlot(candidate.SlotPrice)
		slot.mu.Lock()

		if slot.SlotStatus != SlotStatusFree {
			slot.mu.Unlock()
			continue
		}

		slot.SlotStatus = SlotStatusPending
		usePostOnly := slot.PostOnlyFailCount < 3
		slot.mu.Unlock()

		clientOID := spm.generateClientOrderID(candidate.SlotPrice, "SELL")
		*ordersToPlace = append(*ordersToPlace, &OrderRequest{
			Symbol:        spm.config.Trading.Symbol,
			Side:          "SELL",
			Price:         candidate.SlotPrice,
			Quantity:      candidate.Quantity,
			PriceDecimals: spm.priceDecimals,
			ReduceOnly:    false,
			PostOnly:      usePostOnly,
			ClientOrderID: clientOID,
		})

		shortOrdersCreated++
		logger.Debug("📉 [开空单] 价格: %s, 数量: %.4f",
			formatPrice(candidate.SlotPrice, spm.priceDecimals), candidate.Quantity)
	}

	if shortOrdersCreated > 0 {
		logger.Info("🔴 [做空网格] 创建 %d 个开空单，区域:[%.6f ~ %.6f]",
			shortOrdersCreated, shortZoneMin, shortZoneMax)
	}

	return shortOrdersCreated
}

// handleCloseShort 处理平空仓（买入平仓）
// 返回创建的平仓单数量
func (spm *SuperPositionManager) handleCloseShort(currentPrice float64, priceInterval float64, remainingOrders int, ordersToPlace *[]*OrderRequest) int {
	if remainingOrders <= 0 {
		return 0
	}

	type closeCandidate struct {
		SlotPrice  float64
		ClosePrice float64
		Quantity   float64
		ProfitRate float64
	}
	var candidates []closeCandidate

	spm.slots.Range(func(key, value interface{}) bool {
		slotPrice := key.(float64)
		slot := value.(*InventorySlot)
		slot.mu.Lock()
		defer slot.mu.Unlock()

		// 空仓条件：持仓为负数 + 槽位空闲 + 无活跃订单
		if slot.PositionQty < -0.000001 &&
			slot.SlotStatus == SlotStatusFree &&
			slot.OrderID == 0 &&
			slot.ClientOID == "" {

			closePrice := slotPrice - priceInterval
			closePrice = roundPrice(closePrice, spm.priceDecimals)

			profitRate := (slotPrice - closePrice) / slotPrice
			if profitRate >= 0.001 { // 最小0.1%利润率
				quantity := math.Abs(slot.PositionQty)
				candidates = append(candidates, closeCandidate{
					SlotPrice:  slotPrice,
					ClosePrice: closePrice,
					Quantity:   quantity,
					ProfitRate: profitRate,
				})
			}
		}
		return true
	})

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ProfitRate > candidates[j].ProfitRate
	})

	closeOrdersCreated := 0
	for _, candidate := range candidates {
		if closeOrdersCreated >= remainingOrders {
			break
		}

		orderValue := candidate.ClosePrice * candidate.Quantity
		minValue := spm.config.Trading.MinOrderValue
		if minValue <= 0 {
			minValue = 6.0
		}
		if orderValue < minValue {
			continue
		}

		slot := spm.getOrCreateSlot(candidate.SlotPrice)
		slot.mu.Lock()
		if slot.SlotStatus != SlotStatusFree {
			slot.mu.Unlock()
			continue
		}
		slot.SlotStatus = SlotStatusPending
		usePostOnly := slot.PostOnlyFailCount < 3
		slot.mu.Unlock()

		clientOID := spm.generateClientOrderID(candidate.SlotPrice, "BUY")
		*ordersToPlace = append(*ordersToPlace, &OrderRequest{
			Symbol:        spm.config.Trading.Symbol,
			Side:          "BUY",
			Price:         candidate.ClosePrice,
			Quantity:      candidate.Quantity,
			PriceDecimals: spm.priceDecimals,
			ReduceOnly:    true,
			PostOnly:      usePostOnly,
			ClientOrderID: clientOID,
		})

		closeOrdersCreated++
		logger.Debug("📈 [平空单] 开仓价: %s, 平仓价: %s, 数量: %.4f, 利润率: %.2f%%",
			formatPrice(candidate.SlotPrice, spm.priceDecimals),
			formatPrice(candidate.ClosePrice, spm.priceDecimals),
			candidate.Quantity, candidate.ProfitRate*100)
	}

	if closeOrdersCreated > 0 {
		logger.Info("🟢 [平空仓] 创建 %d 个平仓单", closeOrdersCreated)
	}

	return closeOrdersCreated
}
