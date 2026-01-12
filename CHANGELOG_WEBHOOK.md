# Webhook 自动部署功能 - 更新日志

## 📅 更新时间
2026-01-12

## ✨ 新增功能

### 1. Webhook 自动部署服务器
- **文件**: `cmd/webhook_server/main.go`
- **功能**:
  - 接收 GitHub webhook 请求
  - 验证 HMAC-SHA256 签名
  - 自动执行部署脚本
  - 健康检查端点 `/health`
  - 详细的日志记录

### 2. 自动部署脚本增强
- **文件**: `quick_deploy.sh`
- **新增**:
  - 自动解压 `webhook_server` 二进制文件
  - 自动重启 Webhook 服务器（如果之前在运行）
  - 智能检测 Webhook 服务器状态

### 3. Webhook 管理脚本
- **start_webhook.sh**: 启动 Webhook 服务器
  - 自动检测并停止旧实例
  - 加载环境变量
  - 后台运行并记录日志
  - 显示配置信息
  
- **stop_webhook.sh**: 停止 Webhook 服务器
  - 安全停止所有实例
  - 清理进程

### 4. GitHub Actions 集成
- **文件**: `.github/workflows/build.yml`
- **新增**:
  - 编译 `webhook_server` 二进制文件
  - 将 `webhook_server` 打包到发布文件中
  - 发布后自动触发 Webhook（如果配置了 Secrets）
  - 更新发布说明，包含 webhook_server 信息

### 5. 完整文档
- **WEBHOOK_SETUP.md**: Webhook 配置完整指南
  - 服务器端配置步骤
  - GitHub 配置步骤
  - 安全建议
  - 故障排除
  - 系统服务配置（systemd）
  
- **DEPLOY.md**: 统一部署指南
  - 快速开始
  - 手动部署
  - 自动部署（Webhook）
  - 管理命令
  - 故障排除
  - 最佳实践

- **README.md**: 更新主文档
  - 添加自动部署章节
  - 说明 Webhook 优势

## 🔄 工作流程

```
开发者 Push 代码到 GitHub
         ↓
GitHub Actions 自动触发
         ↓
编译三个二进制文件:
  - opensqt (主程序)
  - telegram_bot (Telegram 控制器)
  - webhook_server (Webhook 服务器)
         ↓
发布到 GitHub Releases (latest tag)
         ↓
触发 Webhook (POST 请求到服务器)
         ↓
服务器 webhook_server 接收请求
         ↓
⏰ 等待1分钟（确保编译完成）
         ↓
验证签名 (HMAC-SHA256)
         ↓
执行 quick_deploy.sh（默认启用 Webhook）
         ↓
下载最新的 tar.gz 文件
         ↓
解压三个二进制文件
         ↓
停止旧的 telegram_bot
         ↓
停止旧的 webhook_server
         ↓
启动新的 telegram_bot
         ↓
重启 webhook_server
         ↓
部署完成 ✅
```

## 📦 发布包内容

每个 `opensqt-linux-{arch}.tar.gz` 现在包含：
- `opensqt` - 主交易程序
- `telegram_bot` - Telegram 控制器
- `webhook_server` - Webhook 自动部署服务器

## 🔧 配置要求

### 服务器端 (.env)
```bash
# Webhook 配置
WEBHOOK_SECRET=your_strong_secret_here
WEBHOOK_PORT=9001
DEPLOY_SCRIPT=./quick_deploy.sh
WORK_DIR=.
```

### GitHub Secrets
- `WEBHOOK_URL`: `http://your-server-ip:9001/webhook`
- `WEBHOOK_SECRET`: 与服务器相同的密码

## 🚀 使用方法

### 首次部署
```bash
# 1. 下载并部署
./quick_deploy.sh

# 2. 配置 Webhook
nano .env  # 添加 WEBHOOK_SECRET 等配置

# 3. 启动 Webhook 服务器
./start_webhook.sh

# 4. 配置 GitHub Secrets
# 在 GitHub 仓库设置中添加 WEBHOOK_URL 和 WEBHOOK_SECRET
```

### 后续更新
```bash
# 自动更新（通过 Webhook）
git push origin main
# 服务器会自动下载并部署最新版本

# 或手动更新
./quick_deploy.sh
```

## 🔐 安全特性

1. **签名验证**: 使用 HMAC-SHA256 验证 GitHub webhook 请求
2. **Secret 保护**: 日志中自动隐藏 Secret（显示为 `****`）
3. **分支过滤**: 只处理 main/master 分支的 push 事件
4. **异步执行**: 部署脚本异步执行，避免阻塞 webhook 响应

## 📊 监控和日志

### Webhook 服务器日志
```bash
tail -f webhook.log
```

日志包含：
- 📥 收到的 webhook 请求
- 📝 提交信息和 ID
- 🚀 部署脚本执行状态
- ✅ 部署成功/失败信息
- 📜 部署脚本输出

### Telegram Bot 日志
```bash
tail -f telegram_bot.log
```

### 部署脚本输出
部署过程会记录到 webhook.log 中

## 🎯 优势

1. **全自动部署**: Push 代码后无需手动操作
2. **零停机更新**: 自动停止旧版本，启动新版本
3. **安全可靠**: 签名验证，防止未授权访问
4. **完整日志**: 记录每次部署的详细信息
5. **智能重启**: 只重启之前在运行的服务
6. **架构自适应**: 自动检测服务器架构（amd64/arm64）

## 📝 相关文件

### 新增文件
- `cmd/webhook_server/main.go` - Webhook 服务器实现
- `start_webhook.sh` - 启动脚本
- `stop_webhook.sh` - 停止脚本
- `WEBHOOK_SETUP.md` - 配置文档
- `DEPLOY.md` - 部署指南
- `TROUBLESHOOTING.md` - 故障排除

### 修改文件
- `.github/workflows/build.yml` - 添加 webhook_server 编译
- `quick_deploy.sh` - 支持 webhook_server
- `README.md` - 添加自动部署说明
- `.env.example` - 添加 Webhook 配置项

## 🔄 升级步骤

如果你已经部署了旧版本，升级步骤：

```bash
# 1. 拉取最新代码（如果是从源码）
git pull origin main

# 2. 或直接运行部署脚本（推荐）
./quick_deploy.sh

# 3. 配置 Webhook（首次）
nano .env  # 添加 WEBHOOK_SECRET 等

# 4. 启动 Webhook 服务器
./start_webhook.sh

# 5. 配置 GitHub Secrets
# 在 GitHub 仓库设置中添加 WEBHOOK_URL 和 WEBHOOK_SECRET
```

## ✅ 测试清单

- [ ] Webhook 服务器启动成功
- [ ] 健康检查端点正常 (`curl http://localhost:9000/health`)
- [ ] GitHub Secrets 配置正确
- [ ] Push 代码后 GitHub Actions 成功编译
- [ ] Webhook 成功触发（查看 webhook.log）
- [ ] 服务器自动下载新版本
- [ ] Telegram Bot 自动重启
- [ ] Webhook 服务器自动重启（如果之前在运行）
- [ ] 所有服务正常运行

## 🐛 已知问题

无

## 📞 支持

如有问题：
1. 查看 [WEBHOOK_SETUP.md](WEBHOOK_SETUP.md)
2. 查看 [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
3. 查看日志文件
4. 提交 GitHub Issue

---

**完成时间**: 2026-01-12  
**版本**: v1.0 (Webhook Auto-Deploy)
