# OpenSQT 部署检查清单

本清单用于确保部署的完整性和正确性。

## 1. 服务器环境检查

### 操作系统
- [ ] 操作系统: Linux (Ubuntu 20.04+ / CentOS 7+)
- [ ] 架构: amd64 或 arm64

### 系统资源
- [ ] 内存: 2GB+ (建议 4GB)
- [ ] CPU: 1 核+ (建议 2 核+)
- [ ] 磁盘: 10GB+ 可用空间

### 网络
- [ ] 服务器可访问互联网
- [ ] 可访问 Telegram API
- [ ] 可访问交易所 API

## 2. 文件检查

### 必需文件
- [ ] `opensqt` - 交易程序主程序
- [ ] `telegram_bot` - Telegram Bot
- [ ] `.env` - 环境配置文件
- [ ] `config.yaml` - 交易配置文件

### 可选文件
- [ ] `start_bot.sh` - 启动脚本
- [ ] `stop_bot.sh` - 停止脚本

### 文件权限
```bash
# 检查文件存在
ls -la opensqt telegram_bot *.sh

# 确保可执行
chmod +x opensqt telegram_bot *.sh
```

## 3. 配置检查

### .env 文件配置
```bash
# Telegram Bot 配置
TELEGRAM_BOT_TOKEN=已配置 ✅ / 未配置 ❌
TELEGRAM_ALLOWED_USERS=已配置 ✅ / 未配置 ❌

# 交易所 API 配置
# Binance
BINANCE_API_KEY=已配置 ✅ / 未配置 ❌
BINANCE_SECRET_KEY=已配置 ✅ / 未配置 ❌

# 或 Bitget
BITGET_API_KEY=已配置 ✅ / 未配置 ❌
BITGET_SECRET_KEY=已配置 ✅ / 未配置 ❌
BITGET_PASSPHRASE=已配置 ✅ / 未配置 ❌

# 或 Gate
GATE_API_KEY=已配置 ✅ / 未配置 ❌
GATE_SECRET_KEY=已配置 ✅ / 未配置 ❌
```

### config.yaml 配置
```yaml
app:
  current_exchange: "binance"  # binance/bitget/gate

trading:
  symbol: "DOGEUSDC"           # 交易对
  price_interval: 0.00002      # 价格间隔
  order_quantity: 12           # 每单金额
  buy_window_size: 40          # 买单数量
  sell_window_size: 30         # 卖单数量
```

### 验证配置
```bash
# 检查配置文件
cat .env
cat config.yaml
```

## 4. 启动检查

### 启动服务
```bash
# 启动
./start_bot.sh

# 验证进程
ps aux | grep -E "telegram_bot|opensqt" | grep -v grep
```

### 验证端口
```bash
# 检查端口监听
netstat -tlnp | grep -E "9000|9001"

# 预期输出:
# tcp   0   0  0.0.0.0:9000   0.0.0.0:*   LISTEN   [进程名]
```

### 查看日志
```bash
# 查看启动日志
tail -f telegram_bot.log

# 预期看到:
# 🤖 OpenSQT Telegram Bot 已启动
# ✅ Telegram Bot 已连接到服务器
```

## 5. Telegram Bot 测试

### 发送 /start 命令
- [ ] Bot 响应欢迎消息
- [ ] 按钮显示正常

### 发送 /status 命令
- [ ] 状态显示正确
- [ ] 价格信息正常

### 发送 /run 命令
- [ ] 交易程序启动
- [ ] 状态变为运行中

### 发送 /stop 命令
- [ ] 交易程序停止
- [ ] 状态变为已停止

## 6. 交易功能测试

### 基本功能
- [ ] 价格监控正常
- [ ] 订单生成正常
- [ ] 订单执行正常

### 日志检查
```bash
# 实时查看日志
tail -f opensqt.log

# 预期日志:
# 📊 [价格] DOGEUSDC 最新价: xxx
# 📝 [订单] 提交订单成功
```

## 7. 故障排除

### 进程未启动
```bash
# 检查进程
ps aux | grep opensqt

# 如果没有进程，检查日志
tail -f opensqt.log
```

### Bot 无响应
```bash
# 检查 Bot 进程
ps aux | grep telegram_bot

# 检查 Bot 日志
tail -f telegram_bot.log
```

### 无法连接交易所
```bash
# 检查 API 配置
cat .env | grep -E "API_KEY|SECRET"

# 测试网络连通性
curl -v https://api.binance.com
```

### 内存不足
```bash
# 检查内存使用
free -h

# 查看进程内存
ps -p $(pgrep opensqt) -o %mem,%cpu
```

## 8. 部署后检查清单

### 每日检查
- [ ] 查看交易日志是否有错误
- [ ] 检查订单是否正常执行
- [ ] 确认 Telegram 通知正常

### 每周检查
- [ ] 检查服务器资源使用
- [ ] 查看日志文件大小
- [ ] 确认最新版本

### 每月检查
- [ ] 检查 API 密钥有效期
- [ ] 查看交易统计
- [ ] 优化配置参数

## 9. 回滚计划

如果新版本出现问题：

```bash
# 1. 停止当前服务
./stop_bot.sh

# 2. 回退到上一版本
git checkout <previous-tag>

# 3. 重新编译
go build -o opensqt .

# 4. 重启服务
./start_bot.sh
```

## 10. 监控建议

### 系统监控
- 使用 `top` 或 `htop` 监控资源
- 设置磁盘空间告警

### 应用监控
- 使用 `tail -f` 实时查看日志
- 设置错误日志告警

### 交易监控
- 通过 Telegram Bot 定期检查状态
- 监控订单执行情况
