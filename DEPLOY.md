# OpenSQT 部署指南

本文档提供完整的手动部署流程。

## 📋 目录

- [快速开始](#快速开始)
- [手动部署](#手动部署)
- [管理命令](#管理命令)
- [故障排除](#故障排除)

## 🚀 快速开始

### 最简单的方式：一键部署

```bash
# 下载并运行部署脚本
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/quick_deploy.sh
chmod +x quick_deploy.sh
./quick_deploy.sh
```

这个脚本会：
1. 自动检测系统架构（amd64/arm64）
2. 从 GitHub Releases 下载最新的预编译二进制文件
3. 停止现有服务
4. 备份并恢复配置文件
5. 解压并设置权限
6. 启动 Telegram Bot

### 配置文件

部署后需要配置两个文件：

**1. .env 文件**（API 密钥和 Bot 配置）
```bash
# Telegram Bot
TELEGRAM_BOT_TOKEN=你的Bot Token
TELEGRAM_ALLOWED_USERS=你的用户ID

# 交易所 API（根据使用的交易所填写）
BINANCE_API_KEY=你的API Key
BINANCE_SECRET_KEY=你的Secret Key

BITGET_API_KEY=你的API Key
BITGET_SECRET_KEY=你的Secret Key
BITGET_PASSPHRASE=你的Passphrase

GATE_API_KEY=你的API Key
GATE_SECRET_KEY=你的Secret Key
```

**2. config.yaml 文件**（交易参数）
```yaml
app:
  current_exchange: "binance"  # 交易所: binance/bitget/gate

trading:
  symbol: "DOGEUSDC"           # 交易对
  price_interval: 0.00002      # 价格间隔
  order_quantity: 12           # 每单金额 (USDT)
  buy_window_size: 40          # 买单数量
  sell_window_size: 30         # 卖单数量
```

## 📦 手动部署

### 1. 下载二进制文件

```bash
# 检测架构
uname -m
# x86_64 = amd64
# aarch64 或 arm64 = arm64

# 下载对应版本
wget https://github.com/dennisyang1986/opensqt_market_maker/releases/download/latest/opensqt-linux-amd64.tar.gz

# 或 ARM64
# wget https://github.com/dennisyang1986/opensqt_market_maker/releases/download/latest/opensqt-linux-arm64.tar.gz
```

### 2. 解压文件

```bash
tar -xzf opensqt-linux-amd64.tar.gz
chmod +x opensqt telegram_bot
```

### 3. 下载配置文件和脚本

```bash
# 配置文件
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/.env.example -O .env
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/config.yaml

# 管理脚本
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/start_bot.sh
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/stop_bot.sh

chmod +x *.sh
```

### 4. 编辑配置

```bash
nano .env          # 填入 API 密钥
nano config.yaml   # 配置交易参数
```

### 5. 启动服务

```bash
./start_bot.sh
```

## 🎮 管理命令

### Telegram Bot 管理

```bash
# 启动（会自动停止旧实例）
./start_bot.sh

# 停止
./stop_bot.sh

# 查看日志
tail -f telegram_bot.log

# 查看进程
ps aux | grep telegram_bot
```

### 交易程序管理

通过 Telegram Bot 控制：
- `/run` - 启动交易程序
- `/stop` - 停止交易程序
- `/restart` - 重启交易程序
- `/status` - 查看运行状态
- `/logs` - 查看最近日志
- `/update` - 更新到最新版本

### 一键部署/更新

```bash
# 下载并部署最新版本
./start_bot.sh
```

### 查看日志

```bash
# 实时查看日志
tail -f opensqt.log

# 查看最近100行
tail -n 100 opensqt.log

# 搜索关键词
grep "error" opensqt.log
```

## 🔧 故障排除

### 常见问题

#### 1. 进程启动失败

**检查方法**：
```bash
# 查看进程是否在运行
ps aux | grep opensqt

# 查看端口是否被占用
netstat -tlnp | grep 9000
```

**解决方法**：
1. 停止现有进程
   ```bash
   pkill -f opensqt
   pkill -f telegram_bot
   ```

2. 清除残留的进程锁
   ```bash
   rm -f opensqt.pid
   ```

3. 检查配置文件
   ```bash
   cat .env
   cat config.yaml
   ```

#### 2. Telegram Bot 无法接收消息

**检查方法**：
```bash
# 查看 Bot 日志
tail -f telegram_bot.log
```

**可能原因**：
1. Bot Token 错误
2. 用户 ID 未在允许列表中
3. 网络问题无法访问 Telegram API

#### 3. 交易所 API 调用失败

**检查方法**：
```bash
# 查看交易日志
grep -i "error\|api\|auth" opensqt.log | tail -n 50
```

**可能原因**：
1. API Key/Secret 错误
2. 权限不足（未开启期货交易）
3. IP 限制

#### 4. 内存或 CPU 占用过高

**检查方法**：
```bash
# 查看资源占用
top -c

# 查看进程详情
ps -p $(cat opensqt.pid) -o %cpu,%mem
```

**解决方法**：
1. 重启服务
   ```bash
   ./stop_bot.sh
   ./start_bot.sh
   ```

#### 5. 交易程序意外停止

**检查方法**：
```bash
# 查看日志中的异常
grep -i "panic\|fatal\|crash" opensqt.log
```

**解决方法**：
1. 检查系统资源是否充足
2. 查看是否有 OOM Killer 杀进程
   ```bash
   dmesg | grep -i kill
   ```

### 日志位置

| 日志文件 | 说明 |
|---------|------|
| `opensqt.log` | 交易程序日志 |
| `telegram_bot.log` | Telegram Bot 日志 |

### 重启服务

```bash
# 完整重启
./stop_bot.sh
./start_bot.sh
```

### 检查服务状态

```bash
# 检查进程
ps aux | grep -E "opensqt|telegram_bot" | grep -v grep

# 检查端口
netstat -tlnp | grep -E "9000|9001"
```

### 监控服务（使用我们的监控脚本）

```bash
# 查看状态
bash status_check.sh

# 或使用 Systemd（如果已配置服务）
systemctl status opensqt
systemctl status telegram_bot
```

### 性能优化建议

1. **内存优化**
   - 确保服务器有足够内存（建议 2GB+）
   - 监控内存使用情况
   - 定期重启清理内存

2. **CPU 优化**
   - 避免同时运行多个实例
   - 合理设置价格监控频率
   - 减少不必要的日志输出

3. **磁盘优化**
   - 定期清理日志文件
   - 使用 logrotate 自动轮转日志
   - 监控磁盘空间使用

### 获得帮助

如果遇到问题：

1. 查看[故障排除](#故障排除)章节
2. 查看日志文件中的错误信息
3. 在 GitHub Issues 中搜索类似问题
4. 提交新的 Issue 描述问题
