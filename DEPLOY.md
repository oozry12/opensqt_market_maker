# OpenSQT 部署指南

本文档提供完整的部署流程说明，包括手动部署、自动部署和故障排除。

## 📋 目录

- [快速开始](#快速开始)
- [手动部署](#手动部署)
- [自动部署（Webhook）](#自动部署webhook)
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
2. 下载最新的预编译二进制文件
3. 解压并设置权限
4. 启动 Telegram Bot

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
chmod +x opensqt telegram_bot webhook_server
```

### 3. 下载配置文件和脚本

```bash
# 配置文件
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/.env.example -O .env
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/config.yaml

# 管理脚本
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/start_bot.sh
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/stop_bot.sh
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/quick_deploy.sh

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

## 🔄 自动部署（Webhook）

配置 Webhook 后，每次 push 代码到 GitHub，服务器会自动更新。

### 快速启用（推荐）

```bash
# 1. 配置 Webhook 环境变量
echo "WEBHOOK_SECRET=$(openssl rand -hex 32)" >> .env
echo "WEBHOOK_PORT=9001" >> .env

# 2. 重新部署并启用 Webhook
./quick_deploy.sh --enable-webhook

# 3. 配置防火墙
sudo ufw allow 9001/tcp

# 4. 测试
curl http://localhost:9001/health
```

### 手动配置（备选）

**1. 配置 Webhook 环境变量**

编辑 `.env` 文件，添加：
```bash
# Webhook 配置
WEBHOOK_SECRET=your_strong_secret_here  # 生成强密码
WEBHOOK_PORT=9001                        # 监听端口
DEPLOY_SCRIPT=./quick_deploy.sh         # 部署脚本
WORK_DIR=.                               # 工作目录
```

生成强密码：
```bash
openssl rand -hex 32
```

**2. 启动 Webhook 服务器**

```bash
# 使用 quick_deploy.sh（推荐）
./quick_deploy.sh --enable-webhook

# 或使用独立脚本
./start_webhook.sh
```

**3. 配置防火墙**

```bash
# Ubuntu/Debian
sudo ufw allow 9001/tcp

# CentOS/RHEL
sudo firewall-cmd --permanent --add-port=9001/tcp
sudo firewall-cmd --reload
```

### GitHub 配置

**1. 添加 Secrets**

进入仓库 Settings → Secrets and variables → Actions，添加：
- `WEBHOOK_URL`: `http://your-server-ip:9001/webhook`
- `WEBHOOK_SECRET`: 与服务器 `.env` 中相同的密码

**2. 测试**

```bash
# 提交一个测试更新
git commit -m "test webhook" --allow-empty
git push origin main

# 查看服务器日志
tail -f webhook.log
```

### 工作流程

```
开发者 Push 代码
    ↓
GitHub Actions 编译
    ↓
发布到 Releases
    ↓
触发 Webhook
    ↓
⏰ 等待1分钟
    ↓
📥 更新 Git 仓库
    ├─ git fetch --all
    ├─ git reset --hard origin/main
    └─ git pull
    ↓
服务器下载新版本
    ↓
自动重启服务
    ↓
部署完成 ✅
```

**注意**：
- Webhook 收到后会等待1分钟，确保 GitHub Actions 编译完成
- 部署前会先更新 Git 仓库，确保脚本和配置文件是最新的

详细配置请参阅 [WEBHOOK_SETUP.md](WEBHOOK_SETUP.md)

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

### Webhook 服务器管理

```bash
# 启动
./start_webhook.sh

# 停止
./stop_webhook.sh

# 查看日志
tail -f webhook.log

# 查看进程
ps aux | grep webhook_server
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
./quick_deploy.sh
```

## 🔧 故障排除

### Telegram Bot 冲突

**问题**：`Conflict: terminated by other getUpdates request`

**原因**：多个 Bot 实例同时运行

**解决**：
```bash
# 停止所有实例
./stop_bot.sh

# 或手动停止
pkill -f telegram_bot

# 重新启动
./start_bot.sh
```

### Webhook 未触发

**检查服务器状态**：
```bash
ps aux | grep webhook_server
tail -f webhook.log
```

**检查防火墙**：
```bash
sudo ufw status
curl http://localhost:9001/health
```

**检查 GitHub Webhook**：
- Settings → Webhooks → 点击你的 webhook
- 查看 "Recent Deliveries"

### 下载失败

**问题**：无法下载 GitHub Releases

**解决**：
```bash
# 检查网络
ping github.com

# 使用代理（如果需要）
export https_proxy=http://your-proxy:port

# 手动下载
wget https://github.com/dennisyang1986/opensqt_market_maker/releases/download/latest/opensqt-linux-amd64.tar.gz
```

### 权限问题

**问题**：`Permission denied`

**解决**：
```bash
# 添加执行权限
chmod +x opensqt telegram_bot webhook_server
chmod +x *.sh
```

### 配置文件缺失

**问题**：`.env` 或 `config.yaml` 不存在

**解决**：
```bash
# 下载配置模板
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/.env.example -O .env
wget https://raw.githubusercontent.com/dennisyang1986/opensqt_market_maker/main/config.yaml

# 编辑配置
nano .env
nano config.yaml
```

## 📚 相关文档

- [README.md](README.md) - 项目介绍和快速开始
- [WEBHOOK_SETUP.md](WEBHOOK_SETUP.md) - Webhook 详细配置
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - 常见问题解决
- [ARCHITECTURE.md](ARCHITECTURE.md) - 系统架构说明
- [USAGE.md](USAGE.md) - 使用指南

## 🔐 安全建议

1. **保护 API 密钥**
   - 不要将 `.env` 文件提交到 Git
   - 使用只读权限的 API 密钥（如果可能）
   - 定期轮换密钥

2. **Webhook 安全**
   - 使用强密码（至少 32 字符）
   - 配置防火墙限制访问
   - 使用 HTTPS（通过 Nginx 反向代理）

3. **服务器安全**
   - 定期更新系统
   - 使用 SSH 密钥认证
   - 配置防火墙规则

## 💡 最佳实践

1. **测试环境**
   - 先在测试网测试
   - 使用小额资金测试
   - 验证所有功能正常

2. **监控**
   - 定期查看日志
   - 设置 Telegram 通知
   - 监控服务器资源

3. **备份**
   - 备份配置文件
   - 记录交易参数
   - 保存重要日志

4. **更新**
   - 关注 GitHub Releases
   - 阅读更新日志
   - 测试后再部署到生产环境

---

如有问题，请查看 [TROUBLESHOOTING.md](TROUBLESHOOTING.md) 或提交 Issue。
