# 中性合约网格测试报告

## 测试日期
2026-01-12

## 测试目的
验证中性合约（做空网格）与做多网格是否会产生订单冲突

## 测试结果：✅ 通过

### 测试1：槽位冲突预防测试
**文件**: `position/slot_conflict_test.go`

**测试内容**：
1. ✅ 槽位状态管理正确
2. ✅ 买单成交可以正确开多仓
3. ✅ 卖单成交可以正确开空仓
4. ✅ 不同槽位可以同时持有多仓和空仓
5. ✅ 同一槽位可以切换持仓状态
6. ✅ 槽位锁机制正常工作
7. ✅ 订单ID匹配机制正常工作

**结论**：
- 每个价格点都有独立的槽位
- 槽位锁机制防止同一槽位同时挂买单和卖单
- 持仓状态（多仓/空仓/空仓）清晰分离
- 订单ID匹配机制确保只处理相关订单更新

### 测试2：中性网格实际场景测试
**文件**: `position/neutral_grid_test.go`

**测试场景**：
- 当前市场价格: 0.14000 USDC
- 价格间隔: 0.0001 USDC

**做多网格（当前价格下方）**：
```
🟢 0.13999 - 多仓: 71.43 DOGE
🟢 0.13998 - 多仓: 71.43 DOGE
⚪ 0.13997 - 空槽位
⚪ 0.13996 - 空槽位
⚪ 0.13995 - 空槽位
```

**做空网格（当前价格上方）**：
```
⚪ 0.14003 - 空槽位
🔴 0.14002 - 空仓: -71.42 DOGE
🔴 0.14001 - 空仓: -71.42 DOGE
```

**验证结果**：
- ✅ 多仓槽位数量正确: 2个
- ✅ 空仓槽位数量正确: 2个
- ✅ 所有多仓都在当前价格下方
- ✅ 所有空仓都在当前价格上方
- ✅ 没有价格冲突

## 价格分布图

```
        做空网格区域（卖出开仓）
        ↓
⚪ 0.14003 [卖单区] 空槽位
🔴 0.14002 [卖单区] 空仓: -71.42
🔴 0.14001 [卖单区] 空仓: -71.42
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📍 当前价格: 0.14000
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🟢 0.13999 [买单区] 多仓: 71.43
🟢 0.13998 [买单区] 多仓: 71.43
⚪ 0.13997 [买单区] 空槽位
⚪ 0.13996 [买单区] 空槽位
⚪ 0.13995 [买单区] 空槽位
        ↑
        做多网格区域（买入开仓）
```

## 核心机制

### 1. 价格区间严格分离
```go
// 做空网格只在当前价格上方操作
shortGridStartPrice := currentPrice + priceInterval
```

**做多网格**：
- 价格范围：当前价格 - N×间隔
- 操作：买入开仓，卖出平仓

**做空网格**：
- 价格范围：当前价格 + N×间隔
- 操作：卖出开仓，买入平仓

### 2. 槽位独立管理
每个价格点都有独立的槽位（InventorySlot）：
```go
type InventorySlot struct {
    Price          float64
    PositionStatus string  // EMPTY, FILLED, SHORT
    PositionQty    float64 // 正数=多仓，负数=空仓
    SlotStatus     string  // FREE, LOCKED
    OrderSide      string  // BUY, SELL
    // ...
}
```

### 3. 槽位锁机制
```go
// 挂单前锁定槽位
slot.SlotStatus = SlotStatusLocked
slot.OrderSide = "BUY" // 或 "SELL"

// 订单完成后释放
slot.SlotStatus = SlotStatusFree
slot.OrderSide = ""
```

防止同一槽位同时挂买单和卖单。

### 4. 持仓状态管理
```go
const (
    PositionStatusEmpty  = "EMPTY"  // 空仓位
    PositionStatusFilled = "FILLED" // 多仓（持仓数量 > 0）
    PositionStatusShort  = "SHORT"  // 空仓（持仓数量 < 0）
)
```

同一槽位只能有一种持仓状态。

## 触发条件

### 中性网格触发条件（非常严格）

**必须同时满足**：
1. ✅ `neutral_grid.enabled = true`
2. ✅ `crash_detection.enabled = true`
3. ⚠️ 暴跌检测器确认（关键条件）
4. ✅ 空仓数量 < max_short_positions

**暴跌检测器判定逻辑**：
```go
func (d *CrashDetector) ShouldOpenShort() bool {
    // 1. 必须检测到暴跌
    if d.currentLevel == CrashNone {
        return false
    }
    
    // 2. 连续上涨K线数 >= 配置值
    return d.uptrendCandles >= cfg.MinUptrendCandles
}
```

**暴跌检测条件**：
1. 单边上涨趋势：
   - MA20 > MA60
   - 连续 N 根 K线收阳（Close > Open）
   
2. 暴跌：
   - 从最近10根K线最高点下跌 >= 配置的暴跌幅度

**当前配置**（已调整）：
```yaml
crash_detection:
  enabled: true
  min_uptrend_candles: 2      # 连续2根上涨K线（10分钟）
  mild_crash_rate: 0.003      # 0.3%暴跌即触发
  severe_crash_rate: 0.008    # 0.8%严重暴跌
  kline_interval: "5m"
```

## 安全机制

### 1. 价格区间分离
- 做多网格：当前价格下方
- 做空网格：当前价格上方
- **不会在同一价格点同时挂买单和卖单**

### 2. 槽位锁机制
- 挂单时锁定槽位
- 防止并发冲突
- 订单完成后释放

### 3. 订单ID匹配
- 每个订单有唯一的 ClientOrderID
- 只处理匹配的订单更新
- 防止误处理其他订单

### 4. 持仓状态验证
- 开空仓前检查：`PositionStatus == EMPTY`
- 平空仓前检查：`PositionStatus == SHORT`
- 确保状态正确

## 运行测试

```bash
# 测试槽位冲突预防
go test -v ./position -run TestSlotConflictPrevention

# 测试中性网格场景
go test -v ./position -run TestNeutralGridScenario

# 运行所有测试
go test -v ./position
```

## 结论

✅ **中性合约网格与做多网格不会产生冲突**

**原因**：
1. 价格区间严格分离（上方 vs 下方）
2. 每个价格点只有一个槽位
3. 槽位锁机制防止并发冲突
4. 持仓状态清晰分离（EMPTY/FILLED/SHORT）
5. 订单ID匹配机制确保准确性

**建议**：
1. 保持当前的价格区间分离机制
2. 监控日志中的 `[中性网格]` 信息
3. 根据实际市场情况调整暴跌检测参数
4. 建议先在测试环境验证后再用于生产

## 相关文件

- `position/super_position_manager.go` - 核心逻辑
- `position/slot_conflict_test.go` - 冲突预防测试
- `position/neutral_grid_test.go` - 场景测试
- `monitor/crash_detector.go` - 暴跌检测器
- `config.yaml` - 配置文件

---

**测试完成时间**: 2026-01-12  
**测试状态**: ✅ 全部通过  
**风险等级**: 🟢 低风险（机制完善，测试通过）
