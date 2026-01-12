<div align="center">
  <img src="https://r2.opensqt.com/opensqt_logo.png" alt="OpenSQT Logo" width="600"/>
  
  # OpenSQT Market Maker
  
  **毫秒级高频加密货币做市商系统 | High-Frequency Crypto Market Maker**

  [![Go Version](https://img.shields.io/badge/Go-1.21%2B-blue.svg)](https://golang.org/dl/)
  [![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
</div>

---
## **还在测试中，功能尚不完善~**

## 📖 项目简介 (Introduction)

OpenSQT Market Maker 是一个高性能、低延迟的加密货币做市商系统，专注于永续合约市场的单向做多无限独立网格交易策略。系统采用 Go 语言开发，基于 WebSocket 实时数据流驱动，旨在为 Binance、Bitget、Gate.io 等主流交易所提供稳定的流动性支持。

经过数个版本迭代，我们已经使用此系统交易超过1亿美元的虚拟货币，例如，交易币安ETHUSDC，0手续，价格间隔1美元，每笔购买300美元，每天的交易量将达到300万美元以上，一个月可以交易5000万美元以上，只要市场是震荡或向上将持续产生盈利，如果市场单边下跌，3万美元保证金可以保证下跌1000个点不爆仓，通过不断交易拉低成本，只要回涨50%即可保本，涨回开仓原价可以赚到丰厚利润，如果出现单边极速下跌，主动风控系统将会自动识别立刻停止交易，当市场恢复后才允许继续下单，不担心插针爆仓。

举例： eth 3000点开始交易，价格下跌到2700点，亏损约3000美元，价格涨回2850点以上已经保本，涨回3000点，盈利在1000-3000美元。

OpenSQT is a high-performance, low-latency cryptocurrency market maker system focusing on long grid trading strategies for perpetual contract markets. Developed in Go and driven by WebSocket real-time data streams, it aims to provide stable liquidity support for major exchanges like Binance, Bitget, and Gate.io.

## 📺 实时演示 (Live Demo)

<video src="https://r2.opensqt.com/product_review.mp4" controls="controls" width="100%"></video>

[点击观看演示视频 / Watch Demo Video](https://r2.opensqt.com/product_review.mp4)

## ✨ 核心特性 (Key Features)

- **多交易所支持**: 适配 Binance, Bitget, Gate.io, Bybit, EdgeX 等主流平台。
- **毫秒级响应**: 全 WebSocket 驱动（行情与订单流），拒绝轮询延迟。
- **智能网格策略**: 
  - **固定金额模式**: 资金利用率更可控。
  - **超级槽位系统 (Super Slot)**: 智能管理挂单与持仓状态，防止并发冲突。
- **强大的风控系统**:
  - **主动风控**: 实时监控 K 线成交量异常，自动暂停交易。
  - **资金安全**: 启动前自动检查余额、杠杆倍数与最大持仓风险。
  - **自动对账**: 定期同步本地与交易所状态，确保数据一致性。
- **高并发架构**: 基于 Goroutine + Channel + Sync.Map 的高效并发模型。

## 🏦 支持的交易所 (Supported Exchanges)

| 交易所 (Exchange) | 状态 (Status) 
|-------------------|---------------
| **Binance**       | ✅ Stable      
| **Bitget**        | ✅ Stable      
| **Gate.io**       | ✅ Stable      


## 模块架构

```
opensqt_platform/
├── main.go                    # 主程序入口，组件编排
│
├── config/                    # 配置管理
│   └── config.go              # YAML配置加载与验证
│
├── exchange/                  # 交易所抽象层（核心）
│   ├── interface.go           # IExchange 统一接口
│   ├── factory.go             # 工厂模式创建交易所实例
│   ├── types.go               # 通用数据结构
│   ├── wrapper_*.go           # 适配器（包装各交易所）
│   ├── binance/               # 币安实现
│   ├── bitget/                # Bitget实现
│   └── gate/                  # Gate.io实现
│
├── logger/                    # 日志系统
│   └── logger.go              # 文件日志 + 控制台日志
│
├── monitor/                   # 价格监控
│   └── price_monitor.go       # 全局唯一价格流
│
├── order/                     # 订单执行层
│   └── executor_adapter.go    # 订单执行器（限流+重试）
│
├── position/                  # 仓位管理（核心）
│   └── super_position_manager.go  # 超级槽位管理器
│
├── safety/                    # 安全与风控
│   ├── safety.go              # 启动前安全检查
│   ├── risk_monitor.go        # 主动风控（K线监控）
│   ├── reconciler.go          # 持仓对账
│   └── order_cleaner.go       # 订单清理
│
└── utils/                     # 工具函数
    └── orderid.go             # 自定义订单ID生成
```

## 最佳实践
1.用来刷交易所vip，本系统是刷量神器，如果上涨下跌幅度不大，3000美元保证金两三天即可刷出1000万美元交易量。

2.赚钱的最佳实践，在市场经过一轮下跌后介入，先买一笔持仓，然后再启动软件，会自动向上一格格卖出，当你的持仓卖光以后停止系统，或不确定当前市场是否是低点，可以不买底仓启动，如果下跌在低点再补一笔持仓重新启动持续给你卖出，利润将最大化，如此循环往复持续赚钱，下跌也不怕，程序持续拉低成本，只要涨回一半即可保本。

## 🚀 快速开始 (Getting Started)

### 环境要求 (Prerequisites)
- Linux 服务器（支持 x86_64 或 ARM64）
- 网络环境需能访问交易所 API 和 GitHub

### 快速部署（推荐）

**一键部署脚本**：
```bash
# 下载部署脚本
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/quick_deploy.sh

# 添加执行权限
chmod +x quick_deploy.sh

# 运行部署脚本
./quick_deploy.sh
```

脚本会自动：
1. 检测系统架构
2. 下载最新的编译好的二进制文件
3. 解压并设置权限
4. 启动 Telegram Bot

### 手动部署

1. **下载最新版本**
   ```bash
   # 根据你的架构选择下载
   # x86_64 架构:
   wget https://github.com/dennisyang1986/opensqt_market_maker/releases/download/latest/opensqt-linux-amd64.tar.gz
   
   # ARM64 架构:
   wget https://github.com/dennisyang1986/opensqt_market_maker/releases/download/latest/opensqt-linux-arm64.tar.gz
   ```

2. **解压文件**
   ```bash
   tar -xzf opensqt-linux-amd64.tar.gz
   chmod +x opensqt telegram_bot
   ```

3. **配置环境变量**
   ```bash
   # 下载配置文件模板
   wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/.env.example -O .env
   wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/config.yaml -O config.yaml
   
   # 编辑配置
   nano .env        # 填入 API 密钥和 Telegram Bot 配置
   nano config.yaml # 配置交易参数
   ```

4. **启动服务**
   ```bash
   # 下载启动脚本
   wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/start_bot.sh
   chmod +x start_bot.sh
   
   # 启动 Telegram Bot
   ./start_bot.sh
   ```

### 配置 (Configuration)

编辑 `.env` 文件，填入你的配置：

```bash
# Telegram Bot 配置
TELEGRAM_BOT_TOKEN=你的Bot Token
TELEGRAM_ALLOWED_USERS=你的用户ID

# 交易所 API 密钥（根据使用的交易所填写）
BINANCE_API_KEY=你的API Key
BINANCE_SECRET_KEY=你的Secret Key
```

编辑 `config.yaml`，配置交易参数：

```yaml
app:
  current_exchange: "binance"  # 选择交易所

trading:
  symbol: "DOGEUSDC"           # 交易对
  price_interval: 0.00002      # 价格间隔
  order_quantity: 12           # 每单金额 (USDT)
  buy_window_size: 40          # 买单数量
  sell_window_size: 30         # 卖单数量
```

### 运行 (Usage)

#### 推荐方式：直接下载预编译的二进制文件

1. **下载最新版本**
   ```bash
   # 下载对应架构的文件（自动检测）
   wget https://github.com/oozry12/opensqt_market_maker/releases/download/latest/opensqt-linux-amd64.tar.gz
   
   # 或 ARM64 架构
   # wget https://github.com/oozry12/opensqt_market_maker/releases/download/latest/opensqt-linux-arm64.tar.gz
   ```

2. **解压文件**
   ```bash
   tar -xzf opensqt-linux-amd64.tar.gz
   chmod +x opensqt telegram_bot
   ```

3. **配置环境变量**
   
   创建 `.env` 文件：
   ```bash
   nano .env
   ```
   
   填入配置：
   ```bash
   # Telegram Bot 配置
   TELEGRAM_BOT_TOKEN=你的Bot Token
   TELEGRAM_ALLOWED_USERS=你的用户ID
   
   # 交易所 API 密钥
   BINANCE_API_KEY=你的API Key
   BINANCE_SECRET_KEY=你的Secret Key
   ```

4. **配置交易参数**
   
   下载配置文件模板：
   ```bash
   wget https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/config.yaml
   nano config.yaml  # 编辑交易参数
   ```

5. **启动 Telegram Bot**
   ```bash
   ./telegram_bot
   ```

6. **在 Telegram 中控制**
   - `/run` - 启动交易程序
   - `/stop` - 停止交易程序
   - `/status` - 查看运行状态
   - `/logs` - 查看最近日志
   - `/update` - 自动下载并更新到最新版本
   - `/help` - 查看所有命令

   **优势**：
   - 🚀 **无需编译**：直接下载预编译的二进制文件
   - 🌐 **远程控制**：在任何地方通过手机控制服务器
   - 🔄 **一键更新**：通过 `/update` 命令自动下载最新版本
   - 📊 **实时监控**：接收交易成交、风控触发等关键事件通知
   - ⚙️ **配置管理**：通过聊天界面修改交易参数

### 自动部署（高级功能）

配置 Webhook 后，每次 push 代码到 GitHub，服务器会自动更新到最新版本：

1. **启动 Webhook 服务器**
   ```bash
   # 配置 .env 文件
   echo "WEBHOOK_SECRET=$(openssl rand -hex 32)" >> .env
   echo "WEBHOOK_PORT=9000" >> .env
   
   # 启动服务
   ./start_webhook.sh
   ```

2. **配置 GitHub Secrets**
   - 进入仓库 Settings → Secrets and variables → Actions
   - 添加 `WEBHOOK_URL`: `http://your-server-ip:9000/webhook`
   - 添加 `WEBHOOK_SECRET`: 与 .env 中相同的密码

3. **完成！**
   - 现在每次 push 代码，服务器会自动下载并部署最新版本
   - 详细配置请参阅 [WEBHOOK_SETUP.md](WEBHOOK_SETUP.md)

   **优势**：
   - 🔄 **全自动部署**：push 代码后自动更新服务器
   - 🔒 **安全验证**：使用 HMAC-SHA256 签名验证
   - 📦 **零停机更新**：自动停止旧版本，启动新版本
   - 📝 **完整日志**：记录每次部署的详细信息

#### 方式二：从源码编译（开发者）

如果你需要修改代码：

```bash
git clone https://github.com/dennisyang1986/opensqt_market_maker.git
cd opensqt_market_maker
chmod +x scripts/build.sh
./scripts/build.sh
```

## 🏗️ 系统架构 (Architecture)

系统采用模块化设计，核心组件包括：

- **Exchange Layer**: 统一的交易所接口抽象，屏蔽底层 API 差异。
- **Price Monitor**: 全局唯一的 WebSocket 价格源，确保决策一致性。
- **Super Position Manager**: 核心仓位管理器，基于槽位 (Slot) 机制管理订单生命周期。
- **Safety & Risk Control**: 多层级风控，包含启动检查、运行时监控和异常熔断。

更多详细架构说明请参阅 [ARCHITECTURE.md](ARCHITECTURE.md)。

## ⚠️ 免责声明 (Disclaimer)

本软件仅供学习和研究使用。加密货币交易具有极高风险，可能导致资金损失。
- 使用本软件产生的任何盈亏由用户自行承担。
- 请务必在实盘前使用测试网 (Testnet) 进行充分测试。
- 开发者不对因软件错误、网络延迟或交易所故障导致的损失负责。

This software is for educational and research purposes only. Cryptocurrency trading involves high risk.
- Users are solely responsible for any profits or losses.
- Always test thoroughly on Testnet before using real funds.
- The developers are not liable for losses due to software bugs, network latency, or exchange failures.

## 🤝 贡献 (Contributing)

欢迎提交 Issue 和 Pull Request！

---
Copyright © 2025 OpenSQT Team. All Rights Reserved.
