#!/bin/bash

# OpenSQT 部署脚本
# 用法: ./scripts/deploy.sh [production|staging]

set -e

ENVIRONMENT=${1:-production}
REPO_URL="https://github.com/your-username/opensqt_market_maker"
DEPLOY_DIR="/opt/opensqt"
SERVICE_USER="opensqt"

echo "🚀 开始部署 OpenSQT ($ENVIRONMENT 环境)"

# 检查是否为 root 用户
if [[ $EUID -eq 0 ]]; then
   echo "❌ 请不要使用 root 用户运行此脚本"
   exit 1
fi

# 创建部署目录
sudo mkdir -p $DEPLOY_DIR
sudo chown $USER:$USER $DEPLOY_DIR

# 克隆或更新代码
if [ -d "$DEPLOY_DIR/.git" ]; then
    echo "📥 更新代码..."
    cd $DEPLOY_DIR
    git pull origin main
else
    echo "📥 克隆代码..."
    git clone $REPO_URL $DEPLOY_DIR
    cd $DEPLOY_DIR
fi

# 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo "❌ Go 未安装，请先安装 Go 1.21+"
    exit 1
fi

# 编译二进制文件
echo "🔨 编译二进制文件..."
go mod download
go build -ldflags="-s -w" -o opensqt .
go build -ldflags="-s -w" -o telegram_bot ./cmd/telegram_bot

# 设置权限
chmod +x opensqt telegram_bot

# 复制配置文件（如果不存在）
if [ ! -f "config.yaml" ]; then
    echo "📝 创建配置文件..."
    cp config.example.yaml config.yaml
    echo "⚠️  请编辑 config.yaml 配置文件"
fi

if [ ! -f ".env" ]; then
    echo "📝 创建环境变量文件..."
    cp .env.example .env
    echo "⚠️  请编辑 .env 文件设置 API 密钥"
fi

# 创建 systemd 服务文件
echo "📋 创建 systemd 服务..."

# OpenSQT 交易服务
sudo tee /etc/systemd/system/opensqt.service > /dev/null <<EOF
[Unit]
Description=OpenSQT Market Maker
After=network.target
Wants=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$DEPLOY_DIR
ExecStart=$DEPLOY_DIR/opensqt config.yaml
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=opensqt

# 环境变量
EnvironmentFile=-$DEPLOY_DIR/.env

# 安全设置
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DEPLOY_DIR

[Install]
WantedBy=multi-user.target
EOF

# Telegram Bot 服务
sudo tee /etc/systemd/system/opensqt-telegram.service > /dev/null <<EOF
[Unit]
Description=OpenSQT Telegram Bot
After=network.target
Wants=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$DEPLOY_DIR
ExecStart=$DEPLOY_DIR/telegram_bot -dir $DEPLOY_DIR -config config.yaml
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=opensqt-telegram

# 环境变量
EnvironmentFile=-$DEPLOY_DIR/.env

# 安全设置
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DEPLOY_DIR

[Install]
WantedBy=multi-user.target
EOF

# 重新加载 systemd
sudo systemctl daemon-reload

# 启用服务（但不立即启动）
sudo systemctl enable opensqt.service
sudo systemctl enable opensqt-telegram.service

echo "✅ 部署完成！"
echo ""
echo "📋 下一步操作："
echo "1. 编辑配置文件: nano $DEPLOY_DIR/config.yaml"
echo "2. 设置环境变量: nano $DEPLOY_DIR/.env"
echo "3. 启动 Telegram Bot: sudo systemctl start opensqt-telegram"
echo "4. 启动交易程序: sudo systemctl start opensqt"
echo ""
echo "📊 管理命令:"
echo "- 查看状态: sudo systemctl status opensqt"
echo "- 查看日志: sudo journalctl -u opensqt -f"
echo "- 停止服务: sudo systemctl stop opensqt"
echo "- 重启服务: sudo systemctl restart opensqt"