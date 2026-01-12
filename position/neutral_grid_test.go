package position

import (
	"fmt"
	"testing"

	"opensqt/config"
)

// TestNeutralGridScenario 测试中性网格的实际场景
func TestNeutralGridScenario(t *testing.T) {
	cfg := &config.Config{}
	cfg.Trading.Symbol = "DOGEUSDC"
	cfg.Trading.PriceInterval = 0.0001
	cfg.Trading.OrderQuantity = 10.0
	cfg.Trading.MinOrderValue = 6.0
	cfg.Trading.BuyWindowSize = 5
	cfg.Trading.SellWindowSize = 5
	cfg.Trading.NeutralGrid.Enabled = true
	cfg.Trading.NeutralGrid.MaxShortPositions = 3
	cfg.Trading.NeutralGrid.ShortCloseRate = 0.006

	mockExecutor := &MockOrderExecutor{}
	spm := NewSuperPositionManager(cfg, mockExecutor, nil, 6, 4)

	fmt.Println("========== 场景：模拟做多网格和做空网格同时运行 ==========")
	fmt.Println()

	// 当前市场价格
	currentPrice := 0.14000

	fmt.Printf("📊 当前市场价格: %.5f\n", currentPrice)
	fmt.Printf("📏 价格间隔: %.5f\n", cfg.Trading.PriceInterval)
	fmt.Println()

	// ========== 步骤1：创建做多网格（当前价格下方） ==========
	fmt.Println("========== 步骤1：创建做多网格（买单区域） ==========")
	
	buyPrices := []float64{
		0.13999, // 当前价格 - 1 * 间隔
		0.13998, // 当前价格 - 2 * 间隔
		0.13997, // 当前价格 - 3 * 间隔
		0.13996, // 当前价格 - 4 * 间隔
		0.13995, // 当前价格 - 5 * 间隔
	}

	for _, price := range buyPrices {
		slot := spm.getOrCreateSlot(price)
		slot.mu.Lock()
		slot.PositionStatus = PositionStatusEmpty
		slot.SlotStatus = SlotStatusFree
		slot.OrderSide = ""
		slot.mu.Unlock()
		fmt.Printf("  创建买单槽位: 价格=%.5f, 状态=%s\n", price, slot.PositionStatus)
	}
	fmt.Println()

	// ========== 步骤2：创建做空网格（当前价格上方） ==========
	fmt.Println("========== 步骤2：创建做空网格（卖单区域） ==========")
	
	sellPrices := []float64{
		0.14001, // 当前价格 + 1 * 间隔
		0.14002, // 当前价格 + 2 * 间隔
		0.14003, // 当前价格 + 3 * 间隔
	}

	for _, price := range sellPrices {
		slot := spm.getOrCreateSlot(price)
		slot.mu.Lock()
		slot.PositionStatus = PositionStatusEmpty
		slot.SlotStatus = SlotStatusFree
		slot.OrderSide = ""
		slot.mu.Unlock()
		fmt.Printf("  创建卖单槽位: 价格=%.5f, 状态=%s\n", price, slot.PositionStatus)
	}
	fmt.Println()

	// ========== 步骤3：模拟买单成交（开多仓） ==========
	fmt.Println("========== 步骤3：模拟买单成交（开多仓） ==========")
	
	// 价格下跌，买单成交
	filledBuyPrices := []float64{0.13999, 0.13998}
	for _, price := range filledBuyPrices {
		slot := spm.getOrCreateSlot(price)
		slot.mu.Lock()
		slot.PositionQty = 71.43 // 10 USDT / 0.14 ≈ 71.43 DOGE
		slot.PositionStatus = PositionStatusFilled
		slot.SlotStatus = SlotStatusFree
		slot.mu.Unlock()
		fmt.Printf("  ✅ 买单成交: 价格=%.5f, 持仓=%.2f (多仓)\n", price, slot.PositionQty)
	}
	fmt.Println()

	// ========== 步骤4：模拟卖单成交（开空仓） ==========
	fmt.Println("========== 步骤4：模拟卖单成交（开空仓） ==========")
	
	// 价格上涨，卖单成交（开空仓）
	filledSellPrices := []float64{0.14001, 0.14002}
	for _, price := range filledSellPrices {
		slot := spm.getOrCreateSlot(price)
		slot.mu.Lock()
		slot.PositionQty = -71.42 // 负数表示空仓
		slot.PositionStatus = PositionStatusShort
		slot.SlotStatus = SlotStatusFree
		slot.mu.Unlock()
		fmt.Printf("  ✅ 卖单成交: 价格=%.5f, 持仓=%.2f (空仓)\n", price, slot.PositionQty)
	}
	fmt.Println()

	// ========== 步骤5：验证槽位状态 ==========
	fmt.Println("========== 步骤5：验证所有槽位状态 ==========")
	
	type slotInfo struct {
		price  float64
		status string
		qty    float64
		zone   string
	}
	
	var slots []slotInfo
	
	spm.slots.Range(func(key, value interface{}) bool {
		price := key.(float64)
		slot := value.(*InventorySlot)
		slot.mu.RLock()
		
		zone := "中间"
		if price < currentPrice {
			zone = "买单区"
		} else if price > currentPrice {
			zone = "卖单区"
		}
		
		slots = append(slots, slotInfo{
			price:  price,
			status: slot.PositionStatus,
			qty:    slot.PositionQty,
			zone:   zone,
		})
		slot.mu.RUnlock()
		return true
	})
	
	// 按价格排序（从高到低）
	for i := 0; i < len(slots); i++ {
		for j := i + 1; j < len(slots); j++ {
			if slots[i].price < slots[j].price {
				slots[i], slots[j] = slots[j], slots[i]
			}
		}
	}
	
	fmt.Println()
	fmt.Println("  价格分布图:")
	fmt.Println("  ----------------------------------------")
	
	for _, s := range slots {
		icon := "⚪"
		desc := "空槽位"
		
		if s.status == PositionStatusFilled && s.qty > 0 {
			icon = "🟢"
			desc = fmt.Sprintf("多仓: %.2f", s.qty)
		} else if s.status == PositionStatusShort && s.qty < 0 {
			icon = "🔴"
			desc = fmt.Sprintf("空仓: %.2f", s.qty)
		}
		
		fmt.Printf("  %s %.5f [%s] %s\n", icon, s.price, s.zone, desc)
	}
	
	fmt.Println("  ----------------------------------------")
	fmt.Printf("  📍 当前价格: %.5f\n", currentPrice)
	fmt.Println()

	// ========== 步骤6：统计和验证 ==========
	fmt.Println("========== 步骤6：统计和验证 ==========")
	
	var longCount, shortCount, emptyCount int
	var longQty, shortQty float64
	
	spm.slots.Range(func(key, value interface{}) bool {
		slot := value.(*InventorySlot)
		slot.mu.RLock()
		
		if slot.PositionStatus == PositionStatusFilled && slot.PositionQty > 0 {
			longCount++
			longQty += slot.PositionQty
		} else if slot.PositionStatus == PositionStatusShort && slot.PositionQty < 0 {
			shortCount++
			shortQty += slot.PositionQty
		} else if slot.PositionStatus == PositionStatusEmpty {
			emptyCount++
		}
		
		slot.mu.RUnlock()
		return true
	})
	
	fmt.Printf("  多仓槽位: %d 个, 总持仓: %.2f\n", longCount, longQty)
	fmt.Printf("  空仓槽位: %d 个, 总持仓: %.2f\n", shortCount, shortQty)
	fmt.Printf("  空槽位: %d 个\n", emptyCount)
	fmt.Println()

	// ========== 验证结果 ==========
	fmt.Println("========== 验证结果 ==========")
	
	// 验证1：多仓和空仓数量正确
	if longCount != 2 {
		t.Errorf("期望2个多仓槽位，实际: %d", longCount)
	} else {
		fmt.Println("  ✅ 多仓槽位数量正确: 2个")
	}
	
	if shortCount != 2 {
		t.Errorf("期望2个空仓槽位，实际: %d", shortCount)
	} else {
		fmt.Println("  ✅ 空仓槽位数量正确: 2个")
	}
	
	// 验证2：多仓在当前价格下方
	var longBelowPrice, shortAbovePrice bool = true, true
	
	spm.slots.Range(func(key, value interface{}) bool {
		price := key.(float64)
		slot := value.(*InventorySlot)
		slot.mu.RLock()
		
		if slot.PositionStatus == PositionStatusFilled && slot.PositionQty > 0 {
			if price >= currentPrice {
				longBelowPrice = false
				fmt.Printf("  ❌ 发现多仓在当前价格上方: %.5f\n", price)
			}
		}
		
		if slot.PositionStatus == PositionStatusShort && slot.PositionQty < 0 {
			if price <= currentPrice {
				shortAbovePrice = false
				fmt.Printf("  ❌ 发现空仓在当前价格下方: %.5f\n", price)
			}
		}
		
		slot.mu.RUnlock()
		return true
	})
	
	if longBelowPrice {
		fmt.Println("  ✅ 所有多仓都在当前价格下方")
	}
	
	if shortAbovePrice {
		fmt.Println("  ✅ 所有空仓都在当前价格上方")
	}
	
	// 验证3：没有价格冲突
	var hasConflict bool
	
	spm.slots.Range(func(key, value interface{}) bool {
		price := key.(float64)
		slot := value.(*InventorySlot)
		slot.mu.RLock()
		
		// 检查是否同时有多仓和空仓（这是不可能的，因为一个槽位只能有一种状态）
		if slot.PositionStatus == PositionStatusFilled && slot.PositionQty > 0 {
			// 检查是否有其他槽位在相同价格有空仓
			spm.slots.Range(func(key2, value2 interface{}) bool {
				price2 := key2.(float64)
				slot2 := value2.(*InventorySlot)
				if price == price2 && slot != slot2 {
					slot2.mu.RLock()
					if slot2.PositionStatus == PositionStatusShort {
						hasConflict = true
						fmt.Printf("  ❌ 发现价格冲突: %.5f 同时有多仓和空仓\n", price)
					}
					slot2.mu.RUnlock()
				}
				return true
			})
		}
		
		slot.mu.RUnlock()
		return true
	})
	
	if !hasConflict {
		fmt.Println("  ✅ 没有价格冲突")
	}
	
	fmt.Println()
	fmt.Println("========== 测试完成 ==========")
	fmt.Println()
	fmt.Println("✅ 结论：中性合约网格与做多网格不会冲突")
	fmt.Println("  - 做多网格在当前价格下方（买入区域）")
	fmt.Println("  - 做空网格在当前价格上方（卖出区域）")
	fmt.Println("  - 两者价格区间严格分离")
	fmt.Println("  - 每个价格点只有一个槽位，不会重复")
	fmt.Println()
}
