# Webhook 自动部署配置指南

## 概述

配置 Webhook 后，当你 push 代码到 GitHub，服务器会自动：
1. GitHub Actions 编译最新的二进制文件
2. 发布到 GitHub Releases
3. 触发服务器上的 Webhook
4. 服务器自动下载并部署最新版本

## 服务器端配置

### 1. 获取 Webhook 服务器

Webhook 服务器已包含在发布包中，无需单独编译：

```bash
# 使用 quick_deploy.sh 自动下载（推荐）
./quick_deploy.sh

# 或手动下载
wget https://github.com/oozry12/opensqt_market_maker/releases/download/latest/opensqt-linux-amd64.tar.gz
tar -xzf opensqt-linux-amd64.tar.gz
chmod +x webhook_server

# 如果需要手动编译
go build -o webhook_server ./cmd/webhook_server
```

## 服务器端配置

### 方法一：快速启用（推荐）

使用 `quick_deploy.sh` 一键启用 Webhook：

```bash
# 1. 配置 Webhook 环境变量
echo "WEBHOOK_SECRET=$(openssl rand -hex 32)" >> .env
echo "WEBHOOK_PORT=9001" >> .env

# 2. 部署并启用 Webhook
./quick_deploy.sh --enable-webhook

# 3. 配置防火墙
sudo ufw allow 9001/tcp

# 4. 测试健康检查
curl http://localhost:9001/health
```

### 方法二：手动配置

**1. 获取 Webhook 服务器**

Webhook 服务器已包含在发布包中：

```bash
# 使用 quick_deploy.sh 自动下载
./quick_deploy.sh

# 或手动下载
wget https://github.com/dennisyang1986/opensqt_market_maker/releases/download/latest/opensqt-linux-amd64.tar.gz
tar -xzf opensqt-linux-amd64.tar.gz
chmod +x webhook_server
```

### 2. 配置环境变量

编辑 `.env` 文件：

```bash
# Webhook 配置
WEBHOOK_SECRET=your_strong_secret_here  # 设置一个强密码
WEBHOOK_PORT=9001                        # 监听端口
DEPLOY_SCRIPT=./quick_deploy.sh         # 部署脚本路径
WORK_DIR=.                               # 工作目录
DEPLOY_DELAY=60                          # 部署延迟（秒），默认60秒
```

**DEPLOY_DELAY 说明**：
- Webhook 收到后会等待指定秒数再执行部署
- 默认60秒，确保 GitHub Actions 编译完成
- 可以根据实际编译时间调整（30-120秒）
- 设置为0则立即执行（不推荐）

### 3. 启动 Webhook 服务器

```bash
# 添加执行权限
chmod +x start_webhook.sh stop_webhook.sh

# 启动服务器（会自动给 quick_deploy.sh 添加执行权限）
./start_webhook.sh

# 查看日志
tail -f webhook.log
```

**注意**：
- Webhook 服务器启动时会自动给 `quick_deploy.sh` 添加执行权限
- 每次执行部署前也会再次确认权限
- 无需手动 `chmod +x quick_deploy.sh`

### 4. 配置防火墙

```bash
# Ubuntu/Debian
sudo ufw allow 9001/tcp

# CentOS/RHEL
sudo firewall-cmd --permanent --add-port=9001/tcp
sudo firewall-cmd --reload
```

### 5. 测试 Webhook

```bash
# 测试健康检查
curl http://localhost:9001/health

# 应该返回: OK
```

## GitHub 配置

### 方法一：使用 GitHub Secrets（推荐）

1. **在 GitHub 仓库中添加 Secrets**：
   - 进入仓库 Settings → Secrets and variables → Actions
   - 添加以下 secrets：
     - `WEBHOOK_URL`: `http://your-server-ip:9001/webhook`
     - `WEBHOOK_SECRET`: 与服务器 `.env` 中相同的密码

2. **GitHub Actions 会自动触发**：
   - 当 push 到 main/master 分支时
   - Actions 编译完成后
   - 自动调用你的 webhook

### 方法二：配置 GitHub Webhook（备选）

1. **进入仓库设置**：
   - Settings → Webhooks → Add webhook

2. **配置 Webhook**：
   - **Payload URL**: `http://your-server-ip:9001/webhook`
   - **Content type**: `application/json`
   - **Secret**: 与服务器 `.env` 中相同的密码
   - **Which events**: 选择 "Just the push event"
   - **Active**: 勾选

3. **保存并测试**：
   - 点击 "Add webhook"
   - 在 "Recent Deliveries" 中可以看到测试请求

## 工作流程

```
开发者 Push 代码
    ↓
GitHub Actions 触发
    ↓
编译 Linux 二进制文件
    ↓
发布到 GitHub Releases
    ↓
触发 Webhook (可选)
    ↓
服务器接收 Webhook
    ↓
⏰ 等待1分钟（确保编译完成）
    ↓
📥 更新 Git 仓库
    ├─ git fetch --all
    ├─ git reset --hard origin/main
    └─ git pull
    ↓
执行 quick_deploy.sh
    ↓
下载最新二进制文件
    ↓
停止旧程序
    ↓
解压新文件
    ↓
重启程序
    ↓
部署完成 ✅
```

**注意**：
- Webhook 收到后会等待1分钟再执行部署，确保 GitHub Actions 已完成编译和发布
- 部署前会先更新 Git 仓库，确保脚本和配置文件是最新的

## 安全建议

### 1. 使用强密码

```bash
# 生成随机密码
openssl rand -hex 32
```

### 2. 使用反向代理（推荐）

使用 Nginx 作为反向代理，添加 HTTPS：

```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location /webhook {
        proxy_pass http://localhost:9000/webhook;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 3. 限制 IP 访问

在防火墙中只允许 GitHub 的 IP：

```bash
# GitHub Webhook IP 范围
# https://api.github.com/meta

# 示例（需要定期更新）
sudo ufw allow from 140.82.112.0/20 to any port 9001
sudo ufw allow from 143.55.64.0/20 to any port 9001
```

## 故障排除

### Webhook 未触发

1. **检查服务器状态**：
   ```bash
   ps aux | grep webhook_server
   tail -f webhook.log
   ```

2. **检查防火墙**：
   ```bash
   sudo ufw status
   curl http://localhost:9001/health
   ```

3. **检查 GitHub Webhook 日志**：
   - Settings → Webhooks → 点击你的 webhook
   - 查看 "Recent Deliveries"

### 部署失败

1. **查看 Webhook 日志**：
   ```bash
   tail -f webhook.log
   ```

2. **手动测试部署脚本**：
   ```bash
   ./quick_deploy.sh
   ```

3. **检查权限**：
   ```bash
   chmod +x quick_deploy.sh webhook_server
   ```

### 签名验证失败

确保服务器和 GitHub 使用相同的 secret：

```bash
# 服务器端
cat .env | grep WEBHOOK_SECRET

# GitHub 端
# 检查 Settings → Secrets → WEBHOOK_SECRET
```

## 管理命令

```bash
# 启动 Webhook 服务器
./start_webhook.sh

# 停止 Webhook 服务器
./stop_webhook.sh

# 查看日志
tail -f webhook.log

# 查看实时日志
tail -f webhook.log | grep -E "收到|部署|成功|失败"

# 重启服务器
./stop_webhook.sh && ./start_webhook.sh
```

## 系统服务配置（可选）

创建 systemd 服务，让 Webhook 服务器开机自启：

```bash
sudo nano /etc/systemd/system/opensqt-webhook.service
```

内容：

```ini
[Unit]
Description=OpenSQT Webhook Server
After=network.target

[Service]
Type=simple
User=your-username
WorkingDirectory=/path/to/opensqt_market_maker
EnvironmentFile=/path/to/opensqt_market_maker/.env
ExecStart=/path/to/opensqt_market_maker/webhook_server
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启用服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable opensqt-webhook
sudo systemctl start opensqt-webhook
sudo systemctl status opensqt-webhook
```

## 测试部署

手动触发一次部署测试：

```bash
# 方法1：直接执行部署脚本
./quick_deploy.sh

# 方法2：模拟 webhook 请求
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

## 完整示例

```bash
# 1. 编译 webhook 服务器
go build -o webhook_server ./cmd/webhook_server

# 2. 配置环境变量
cat >> .env << EOF
WEBHOOK_SECRET=$(openssl rand -hex 32)
WEBHOOK_PORT=9001
DEPLOY_SCRIPT=./quick_deploy.sh
WORK_DIR=.
EOF

# 3. 启动服务
./start_webhook.sh

# 4. 配置 GitHub Secrets
# WEBHOOK_URL=http://your-server-ip:9001/webhook
# WEBHOOK_SECRET=<从 .env 复制>

# 5. 测试
git commit -m "test webhook" --allow-empty
git push origin main

# 6. 查看日志
tail -f webhook.log
```

完成！现在每次 push 代码，服务器都会自动更新了。🎉
