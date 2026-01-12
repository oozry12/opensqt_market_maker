# OpenSQT 部署检查清单

## 📋 部署前检查

### 1. 服务器环境

- [ ] Linux 系统（支持 x86_64 或 ARM64）
- [ ] 已安装 wget 或 curl
- [ ] 已安装 tar
- [ ] 网络可访问 GitHub
- [ ] 有足够的磁盘空间（至少 100MB）

### 2. 配置文件准备

#### .env 文件
```bash
# 必填项
- [ ] TELEGRAM_BOT_TOKEN（Telegram Bot Token）
- [ ] TELEGRAM_ALLOWED_USERS（授权用户ID）
- [ ] API 密钥（根据使用的交易所）

# 可选项（Webhook 自动部署）
- [ ] WEBHOOK_SECRET（强密码）
- [ ] WEBHOOK_PORT（默认 9001）
- [ ] DEPLOY_DELAY（默认 60 秒）
```

#### config.yaml 文件
```yaml
- [ ] current_exchange（交易所选择）
- [ ] symbol（交易对）
- [ ] price_interval（价格间隔）
- [ ] order_quantity（每单金额）
- [ ] buy_window_size（买单数量）
- [ ] sell_window_size（卖单数量）
```

### 3. GitHub 配置（可选，用于自动部署）

- [ ] 已 fork 或拥有仓库
- [ ] 已配置 GitHub Secrets：
  - [ ] WEBHOOK_URL
  - [ ] WEBHOOK_SECRET
- [ ] GitHub Actions 已启用

---

## 🚀 部署步骤

### 方式1：快速部署（推荐）

```bash
# 1. 下载部署脚本
wget https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/quick_deploy.sh
chmod +x quick_deploy.sh

# 2. 下载配置文件模板
wget https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/.env.example -O .env
wget https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/config.yaml

# 3. 编辑配置
nano .env          # 填入 API 密钥和 Bot Token
nano config.yaml   # 配置交易参数

# 4. 运行部署
./quick_deploy.sh

# 5. 检查状态
tail -f telegram_bot.log
```

**检查点**：
- [ ] 脚本下载成功
- [ ] 配置文件已编辑
- [ ] 部署脚本执行成功
- [ ] Telegram Bot 启动成功
- [ ] 日志无错误

---

### 方式2：启用 Webhook 自动部署

```bash
# 1. 完成方式1的所有步骤

# 2. 配置 Webhook
echo "WEBHOOK_SECRET=$(openssl rand -hex 32)" >> .env
echo "WEBHOOK_PORT=9001" >> .env
echo "DEPLOY_DELAY=60" >> .env

# 3. 下载 Webhook 脚本
wget https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/start_webhook.sh
wget https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/stop_webhook.sh
chmod +x start_webhook.sh stop_webhook.sh

# 4. 启动 Webhook 服务器
./quick_deploy.sh --enable-webhook

# 5. 配置防火墙
sudo ufw allow 9001/tcp

# 6. 配置 GitHub Secrets
# 在 GitHub 仓库设置中添加：
# - WEBHOOK_URL: http://your-server-ip:9001/webhook
# - WEBHOOK_SECRET: (从 .env 复制)

# 7. 测试 Webhook
curl http://localhost:9001/health
```

**检查点**：
- [ ] Webhook 服务器启动成功
- [ ] 防火墙已配置
- [ ] GitHub Secrets 已添加
- [ ] 健康检查返回 OK
- [ ] Webhook 日志正常

---

## ✅ 部署后验证

### 1. Telegram Bot 验证

```bash
# 查看进程
ps aux | grep telegram_bot

# 查看日志
tail -f telegram_bot.log

# 在 Telegram 中测试
/help    # 查看帮助
/status  # 查看状态
```

**检查点**：
- [ ] Bot 进程正在运行
- [ ] 日志无错误
- [ ] Telegram 可以收到回复
- [ ] 所有命令正常工作

### 2. 交易程序验证

```bash
# 在 Telegram 中启动
/run

# 查看日志
/logs

# 查看状态
/status
```

**检查点**：
- [ ] 交易程序启动成功
- [ ] 价格监控正常
- [ ] 订单可以正常挂单
- [ ] 持仓显示正常

### 3. Webhook 验证（如果启用）

```bash
# 查看 Webhook 进程
ps aux | grep webhook_server

# 查看日志
tail -f webhook.log

# 测试部署
git commit -m "test" --allow-empty
git push origin main

# 等待1分钟后查看日志
tail -f webhook.log
```

**检查点**：
- [ ] Webhook 进程正在运行
- [ ] 收到 GitHub webhook 请求
- [ ] 等待60秒后开始部署
- [ ] 自动下载新版本
- [ ] 自动重启服务

---

## 🔧 常见问题排查

### 问题1：Telegram Bot 冲突

**症状**：
```
Conflict: terminated by other getUpdates request
```

**解决**：
```bash
./stop_bot.sh
./start_bot.sh
```

### 问题2：下载失败

**症状**：
```
❌ 需要安装 wget 或 curl
```

**解决**：
```bash
# Ubuntu/Debian
sudo apt-get install wget

# CentOS/RHEL
sudo yum install wget
```

### 问题3：权限错误

**症状**：
```
Permission denied
```

**解决**：
```bash
chmod +x opensqt telegram_bot webhook_server
chmod +x *.sh
```

### 问题4：配置文件缺失

**症状**：
```
⚠️ .env 文件不存在
```

**解决**：
```bash
wget https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/.env.example -O .env
nano .env
```

### 问题5：Webhook 未触发

**检查步骤**：
```bash
# 1. 检查服务器状态
ps aux | grep webhook_server
tail -f webhook.log

# 2. 检查防火墙
sudo ufw status
curl http://localhost:9001/health

# 3. 检查 GitHub Webhook
# Settings → Webhooks → Recent Deliveries

# 4. 检查签名
cat .env | grep WEBHOOK_SECRET
# 确保与 GitHub Secret 一致
```

---

## 📊 监控和维护

### 日常检查

```bash
# 每天检查一次
./status_check.sh

# 或手动检查
ps aux | grep telegram_bot
ps aux | grep webhook_server
tail -n 50 telegram_bot.log
tail -n 50 webhook.log
```

### 日志管理

```bash
# 查看实时日志
tail -f telegram_bot.log

# 查看最近100行
tail -n 100 telegram_bot.log

# 搜索错误
grep -i error telegram_bot.log

# 清理旧日志（可选）
# 注意：会删除日志历史
> telegram_bot.log
> webhook.log
```

### 更新程序

```bash
# 方式1：通过 Telegram Bot
/update

# 方式2：手动更新
./quick_deploy.sh

# 方式3：自动更新（如果启用 Webhook）
# 只需 push 代码到 GitHub
git push origin main
```

---

## 🔐 安全检查

### 1. API 密钥安全

- [ ] .env 文件权限设置为 600
  ```bash
  chmod 600 .env
  ```
- [ ] .env 文件未提交到 Git
  ```bash
  cat .gitignore | grep .env
  ```
- [ ] 使用只读权限的 API 密钥（如果可能）

### 2. Webhook 安全

- [ ] 使用强密码（至少32字符）
  ```bash
  openssl rand -hex 32
  ```
- [ ] 配置防火墙限制访问
  ```bash
  sudo ufw allow from trusted-ip to any port 9001
  ```
- [ ] 考虑使用 HTTPS（通过 Nginx 反向代理）

### 3. 服务器安全

- [ ] 定期更新系统
  ```bash
  sudo apt-get update && sudo apt-get upgrade
  ```
- [ ] 使用 SSH 密钥认证
- [ ] 禁用 root 登录
- [ ] 配置防火墙规则

---

## 📈 性能优化

### 1. 日志轮转

创建 logrotate 配置：
```bash
sudo nano /etc/logrotate.d/opensqt
```

内容：
```
/path/to/opensqt_market_maker/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0644 user user
}
```

### 2. 系统服务（可选）

创建 systemd 服务：
```bash
sudo nano /etc/systemd/system/opensqt-bot.service
```

内容：
```ini
[Unit]
Description=OpenSQT Telegram Bot
After=network.target

[Service]
Type=simple
User=your-username
WorkingDirectory=/path/to/opensqt_market_maker
EnvironmentFile=/path/to/opensqt_market_maker/.env
ExecStart=/path/to/opensqt_market_maker/telegram_bot
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启用服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable opensqt-bot
sudo systemctl start opensqt-bot
sudo systemctl status opensqt-bot
```

---

## 📚 相关文档

- [README.md](README.md) - 项目介绍
- [DEPLOY.md](DEPLOY.md) - 详细部署指南
- [WEBHOOK_SETUP.md](WEBHOOK_SETUP.md) - Webhook 配置
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - 故障排除
- [USAGE.md](USAGE.md) - 使用指南

---

## ✅ 部署完成确认

完成以下所有检查后，部署即为成功：

- [ ] Telegram Bot 正常运行
- [ ] 可以通过 Telegram 控制
- [ ] 交易程序可以启动
- [ ] 订单可以正常挂单
- [ ] 日志无错误
- [ ] （可选）Webhook 自动部署正常工作

**恭喜！OpenSQT 已成功部署！** 🎉

---

**最后更新**: 2026-01-12  
**版本**: v1.0
