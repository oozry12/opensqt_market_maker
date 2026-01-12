# 部署检查清单

## ✅ 已完成的功能

### 1. Webhook 延迟部署
- **状态**: ✅ 已实现
- **功能**: Webhook 收到后等待指定时间再执行部署
- **配置**: `DEPLOY_DELAY` 环境变量（默认60秒）
- **文件**: `cmd/webhook_server/main.go`
- **代码位置**: `executeDeploy()` 函数

### 2. 自动权限管理
- **状态**: ✅ 已实现
- **功能**: 自动给部署脚本添加执行权限
- **实现**: `ensureExecutable()` 函数
- **触发时机**:
  - Webhook 服务器启动时
  - 每次执行部署前
- **文件**: `cmd/webhook_server/main.go`

### 3. Git 仓库自动更新
- **状态**: ✅ 已实现
- **功能**: 部署前先更新 Git 仓库
- **实现**: `updateGitRepo()` 函数
- **执行步骤**:
  1. `git fetch --all`
  2. `git reset --hard origin/main`
  3. `git pull`
- **文件**: `cmd/webhook_server/main.go`

### 4. 固定下载地址
- **状态**: ✅ 已实现
- **地址**: `https://github.com/oozry12/opensqt_market_maker/releases/download/latest/opensqt-linux-${GOARCH}.tar.gz`
- **文件**: `quick_deploy.sh`

### 5. Webhook 端口配置
- **状态**: ✅ 已实现
- **默认端口**: 9001（避免8080冲突）
- **配置**: `WEBHOOK_PORT` 环境变量
- **文件**: `cmd/webhook_server/main.go`, `.env.example`

### 6. 完整文档
- **状态**: ✅ 已完成
- **文件**:
  - `WEBHOOK_SETUP.md` - Webhook 配置指南
  - `DEPLOY.md` - 部署指南
  - `CHANGELOG_WEBHOOK.md` - 更新日志
  - `.env.example` - 环境变量示例

## 🔄 工作流程

```
GitHub Push
    ↓
GitHub Actions 编译
    ↓
发布到 Releases
    ↓
触发 Webhook
    ↓
⏰ 等待 60 秒（DEPLOY_DELAY）
    ↓
📥 更新 Git 仓库
    ├─ git fetch --all
    ├─ git reset --hard origin/main
    └─ git pull
    ↓
🔧 设置脚本执行权限
    └─ chmod +x quick_deploy.sh
    ↓
🚀 执行 quick_deploy.sh --enable-webhook
    ├─ 下载最新二进制文件
    ├─ 解压文件
    ├─ 停止旧服务
    └─ 启动新服务（包括 Webhook）
    ↓
✅ 部署完成
```

## 📋 环境变量配置

### .env 文件必需配置

```bash
# Webhook 服务器配置
WEBHOOK_SECRET=your_strong_secret_here  # 强密码（至少32字符）
WEBHOOK_PORT=9001                        # 监听端口
DEPLOY_SCRIPT=./quick_deploy.sh         # 部署脚本路径
WORK_DIR=.                               # 工作目录
DEPLOY_DELAY=60                          # 部署延迟（秒）

# Telegram Bot 配置
TELEGRAM_BOT_TOKEN=your_bot_token
TELEGRAM_ALLOWED_USERS=123456789

# 交易所 API 密钥
BINANCE_API_KEY=your_api_key
BINANCE_SECRET_KEY=your_secret_key
```

### GitHub Secrets 配置

在 GitHub 仓库 Settings → Secrets and variables → Actions 中添加：

- `WEBHOOK_URL`: `http://your-server-ip:9001/webhook`
- `WEBHOOK_SECRET`: 与服务器 `.env` 中相同的密码

## 🚀 部署步骤

### 首次部署

```bash
# 1. 下载部署脚本
wget https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/quick_deploy.sh
chmod +x quick_deploy.sh

# 2. 运行部署脚本
./quick_deploy.sh

# 3. 配置环境变量
nano .env
# 填入 API 密钥和 Bot Token

# 4. 配置交易参数
nano config.yaml

# 5. 启用 Webhook（可选）
echo "WEBHOOK_SECRET=$(openssl rand -hex 32)" >> .env
echo "WEBHOOK_PORT=9001" >> .env
echo "DEPLOY_DELAY=60" >> .env

# 6. 重新部署并启用 Webhook
./quick_deploy.sh --enable-webhook

# 7. 配置防火墙
sudo ufw allow 9001/tcp

# 8. 配置 GitHub Secrets
# 在 GitHub 仓库设置中添加 WEBHOOK_URL 和 WEBHOOK_SECRET
```

### 后续更新

```bash
# 自动更新（通过 Webhook）
git push origin main
# 服务器会自动部署

# 或手动更新
./quick_deploy.sh
```

## 🧪 测试清单

### Webhook 服务器测试

```bash
# 1. 检查服务器是否运行
ps aux | grep webhook_server

# 2. 测试健康检查
curl http://localhost:9001/health
# 应该返回: OK

# 3. 查看日志
tail -f webhook.log

# 4. 测试部署（模拟 webhook）
curl -X POST http://localhost:9001/webhook \
  -H "Content-Type: application/json" \
  -d '{
    "ref": "refs/heads/main",
    "repository": {"full_name": "test/repo"},
    "head_commit": {
      "message": "test deployment",
      "id": "abc123"
    }
  }'
```

### Git 更新测试

```bash
# 1. 修改一个文件
echo "# Test" >> README.md

# 2. 提交并推送
git add README.md
git commit -m "test webhook"
git push origin main

# 3. 查看 webhook 日志
tail -f webhook.log

# 应该看到:
# - 📥 收到 webhook
# - ⏰ 等待 60 秒
# - 📥 正在更新 Git 仓库
# - ✓ git fetch 完成
# - ✓ git reset 完成
# - ✓ git pull 完成
# - 🚀 开始执行部署脚本
# - ✅ 部署成功
```

### 权限测试

```bash
# 1. 删除脚本执行权限
chmod -x quick_deploy.sh

# 2. 触发 webhook
# Webhook 应该自动添加执行权限

# 3. 验证权限
ls -la quick_deploy.sh
# 应该显示: -rwxr-xr-x
```

## 🔍 故障排除

### Webhook 未触发

**检查项**:
1. Webhook 服务器是否运行: `ps aux | grep webhook_server`
2. 端口是否开放: `sudo ufw status`
3. GitHub Secrets 是否配置正确
4. 查看 webhook 日志: `tail -f webhook.log`

**解决方法**:
```bash
# 重启 Webhook 服务器
./stop_webhook.sh
./start_webhook.sh

# 检查配置
cat .env | grep WEBHOOK
```

### Git 更新失败

**可能原因**:
1. 本地有未提交的修改
2. 网络问题
3. Git 权限问题

**解决方法**:
```bash
# 手动更新
git fetch --all
git reset --hard origin/main
git pull

# 检查 Git 状态
git status
```

### 部署脚本执行失败

**检查项**:
1. 脚本是否有执行权限: `ls -la quick_deploy.sh`
2. 脚本路径是否正确: `cat .env | grep DEPLOY_SCRIPT`
3. 查看部署日志: `tail -f webhook.log`

**解决方法**:
```bash
# 手动添加权限
chmod +x quick_deploy.sh

# 手动执行测试
./quick_deploy.sh
```

### 延迟时间不够

**问题**: GitHub Actions 编译时间超过60秒

**解决方法**:
```bash
# 增加延迟时间到120秒
echo "DEPLOY_DELAY=120" >> .env

# 重启 Webhook 服务器
./stop_webhook.sh
./start_webhook.sh
```

## 📊 监控命令

```bash
# 查看所有服务状态
ps aux | grep -E "telegram_bot|webhook_server|opensqt"

# 查看 Webhook 日志（实时）
tail -f webhook.log

# 查看 Bot 日志（实时）
tail -f telegram_bot.log

# 查看最近的部署记录
tail -n 50 webhook.log | grep -E "收到|部署|成功|失败"

# 检查端口占用
netstat -tlnp | grep 9001
```

## 🔐 安全检查

```bash
# 1. 检查 Secret 强度
cat .env | grep WEBHOOK_SECRET
# 应该至少32字符

# 2. 检查防火墙
sudo ufw status
# 应该只开放必要端口

# 3. 检查文件权限
ls -la .env
# 应该是 -rw------- (600)

# 4. 设置正确权限
chmod 600 .env
```

## 📝 维护建议

### 日常维护

```bash
# 每天检查日志
tail -n 100 webhook.log
tail -n 100 telegram_bot.log

# 每周清理旧日志
find . -name "*.log" -mtime +7 -exec truncate -s 0 {} \;

# 每月检查磁盘空间
df -h
```

### 定期更新

```bash
# 检查是否有新版本
git fetch origin
git log HEAD..origin/main --oneline

# 更新到最新版本
./quick_deploy.sh
```

## ✅ 完成标志

当以下所有项都完成时，部署即为成功：

- [ ] Webhook 服务器运行正常
- [ ] 健康检查返回 OK
- [ ] Git 更新功能正常
- [ ] 部署脚本自动获得执行权限
- [ ] Push 代码后自动部署
- [ ] Telegram Bot 正常运行
- [ ] 所有日志正常记录
- [ ] 防火墙配置正确
- [ ] GitHub Secrets 配置正确

## 📚 相关文档

- [WEBHOOK_SETUP.md](WEBHOOK_SETUP.md) - Webhook 详细配置
- [DEPLOY.md](DEPLOY.md) - 部署指南
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - 故障排除
- [CHANGELOG_WEBHOOK.md](CHANGELOG_WEBHOOK.md) - 更新日志
- [README.md](README.md) - 项目介绍

---

**最后更新**: 2026-01-12  
**版本**: v1.0
