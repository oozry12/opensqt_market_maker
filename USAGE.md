# OpenSQT 使用指南

## 📋 目录

1. [快速开始](#快速开始)
2. [编译程序](#编译程序)
3. [配置设置](#配置设置)
4. [启动方式](#启动方式)
5. [Telegram Bot 控制](#telegram-bot-控制)
6. [常见问题](#常见问题)

## 🚀 快速开始

### 1. 编译程序

首次使用需要编译程序：

```bash
chmod +x scripts/build.sh
./scripts/build.sh
```

编译完成后会生成：
- `opensqt` - 主交易程序
- `telegram_bot` - Telegram Bot 控制程序

### 2. 配置设置

#### 交易配置

1. 复制配置文件：
   ```bash
   cp config.example.yaml config.yaml
   ```

2. 编辑 `config.yaml`，设置：
   - 交易所选择和 API 密钥
   - 交易对和网格参数
   - 风控设置

#### Telegram Bot 配置（可选但推荐）

1. 复制环境变量文件：
   ```bash
   cp .env.example .env
   ```

2. 编辑 `.env` 文件：
   ```bash
   TELEGRAM_BOT_TOKEN=你的Bot Token
   TELEGRAM_ALLOWED_USERS=你的用户ID
   ```

   **获取 Bot Token:**
   - 在 Telegram 搜索 @BotFather
   - 发送 `/newbot` 创建机器人
   - 获取 Token

   **获取用户ID:**
   - 在 Telegram 搜索 @userinfobot
   - 发送任意消息获取你的用户ID

## 🎯 启动方式

### 方式一：直接启动交易程序

```bash
./opensqt config.yaml
```

### 方式二：Telegram Bot 远程控制（推荐）

1. **启动 Telegram Bot:**
   ```bash
   ./telegram_bot
   ```

2. **在 Telegram 中控制:**
   - `/run` - 启动交易程序
   - `/stop` - 停止交易程序
   - `/status` - 查看运行状态
   - `/restart` - 重启交易程序
   - `/logs` - 查看最近日志
   - `/update` - 拉取代码更新并重新编译

### 方式三：使用启动脚本

```bash
chmod +x scripts/start.sh
./scripts/start.sh
```

## 🤖 Telegram Bot 控制

### 基本命令

| 命令 | 功能 | 说明 |
|------|------|------|
| `/run` | 启动交易程序 | 自动拉取最新代码后启动 |
| `/stop` | 停止交易程序 | 优雅关闭，会撤销所有订单 |
| `/restart` | 重启交易程序 | 先停止再启动 |
| `/status` | 查看运行状态 | 显示进程信息和运行时间 |
| `/logs` | 查看最近日志 | 显示最近100条日志 |
| `/update` | 更新代码 | git pull + 重新编译 + 重启 |
| `/help` | 显示帮助 | 查看所有可用命令 |

### 配置管理命令

| 命令 | 功能 | 示例 |
|------|------|------|
| `/panel` | 打开配置面板 | 图形化配置界面 |
| `/config` | 查看当前配置 | 显示交易参数 |
| `/setsymbol` | 设置交易对 | `/setsymbol DOGEUSDC` |
| `/setpriceinterval` | 设置价格间隔 | `/setpriceinterval 0.0001` |
| `/setorderquantity` | 设置订单金额 | `/setorderquantity 12` |
| `/setminordervalue` | 设置最小订单价值 | `/setminordervalue 10` |

### 实时通知

Bot 会自动推送以下事件通知：
- 💰 **交易成交**: 买单/卖单成交通知
- 🚨 **风控触发**: 市场异常时的风控通知
- ⚠️ **错误警告**: 系统错误和异常情况
- 📊 **状态变化**: 程序启动/停止状态

## 🔧 高级配置

### Telegram Bot 参数

启动 Telegram Bot 时可以指定参数：

```bash
./telegram_bot -dir /path/to/trading -exe opensqt -config config.yaml
```

参数说明：
- `-dir`: 交易程序所在目录（默认当前目录）
- `-exe`: 可执行文件名（默认自动检测）
- `-config`: 配置文件路径（默认 config.yaml）

### 环境变量优先级

配置加载优先级（从高到低）：
1. 系统环境变量
2. `.env` 文件
3. `config.yaml` 文件

## ❓ 常见问题

### Q: 编译失败怎么办？
A: 确保安装了 Go 1.21+ 版本，并且网络能访问 Go 模块代理。

### Q: Telegram Bot 无法启动？
A: 检查 `.env` 文件中的 `TELEGRAM_BOT_TOKEN` 是否正确设置。

### Q: 交易程序启动失败？
A: 检查：
- `config.yaml` 文件是否存在
- API 密钥是否正确
- 网络是否能访问交易所 API

### Q: 如何停止所有程序？
A: 
- **通过 Telegram**: 发送 `/stop` 命令
- **直接停止**: `Ctrl+C` 或 `pkill opensqt`

### Q: 如何查看日志？
A: 
- **通过 Telegram**: 发送 `/logs` 命令
- **直接查看**: 程序会输出到控制台

### Q: 更新代码后需要重新编译吗？
A: 
- **通过 Telegram**: 发送 `/update` 命令会自动处理
- **手动更新**: 运行 `git pull` 后重新编译

## 🛡️ 安全建议

1. **API 密钥安全**:
   - 使用环境变量存储 API 密钥
   - 不要将 `.env` 文件提交到版本控制

2. **Telegram Bot 安全**:
   - 只将你的用户ID添加到 `TELEGRAM_ALLOWED_USERS`
   - 定期更换 Bot Token

3. **服务器安全**:
   - 使用防火墙限制访问
   - 定期更新系统和依赖

4. **资金安全**:
   - 先在测试网测试
   - 设置合理的风控参数
   - 不要投入超过承受能力的资金

## 📞 技术支持

如果遇到问题，请：
1. 查看日志输出
2. 检查配置文件
3. 参考本文档的常见问题
4. 提交 GitHub Issue

---

**祝您交易愉快！** 🎉