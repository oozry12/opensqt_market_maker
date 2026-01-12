#!/bin/bash

# OpenSQT Webhook 自动部署设置脚本

set -e

echo "🔧 OpenSQT Webhook 自动部署设置"
echo "================================"
echo ""

# 检查是否为root用户
if [ "$EUID" -ne 0 ]; then 
    echo "❌ 请使用 sudo 运行此脚本"
    exit 1
fi

# 获取当前目录
WORK_DIR=$(pwd)
echo "📁 工作目录: $WORK_DIR"

# 获取当前用户
CURRENT_USER=${SUDO_USER:-$USER}
echo "👤 运行用户: $CURRENT_USER"

# 生成随机的 webhook secret
WEBHOOK_SECRET=$(openssl rand -hex 32)
echo "🔑 生成的 Webhook Secret: $WEBHOOK_SECRET"
echo ""
echo "⚠️ 请保存此 Secret，稍后需要在 GitHub 仓库设置中使用"
echo ""

# 询问端口
read -p "🔌 Webhook 端口 (默认 9000): " WEBHOOK_PORT
WEBHOOK_PORT=${WEBHOOK_PORT:-9001}

# 编译 webhook_server
echo "🔨 编译 webhook_server..."
go build -ldflags="-s -w" -o webhook_server webhook_server.go
chmod +x webhook_server
echo "✅ webhook_server 编译完成"

# 创建 systemd 服务文件
echo "📝 创建 systemd 服务文件..."
cat > /etc/systemd/system/opensqt-webhook.service <<EOF
[Unit]
Description=OpenSQT Webhook Server
After=network.target

[Service]
Type=simple
User=$CURRENT_USER
WorkingDirectory=$WORK_DIR
Environment="WORK_DIR=$WORK_DIR"
Environment="WEBHOOK_SECRET=$WEBHOOK_SECRET"
Environment="WEBHOOK_PORT=$WEBHOOK_PORT"
ExecStart=$WORK_DIR/webhook_server
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# 重新加载 systemd
echo "🔄 重新加载 systemd..."
systemctl daemon-reload

# 启用并启动服务
echo "🚀 启动 webhook 服务..."
systemctl enable opensqt-webhook
systemctl start opensqt-webhook

# 检查服务状态
sleep 2
if systemctl is-active --quiet opensqt-webhook; then
    echo "✅ Webhook 服务已成功启动"
else
    echo "❌ Webhook 服务启动失败"
    systemctl status opensqt-webhook
    exit 1
fi

echo ""
echo "================================"
echo "✅ Webhook 自动部署设置完成！"
echo ""
echo "📋 下一步操作："
echo ""
echo "1. 在 GitHub 仓库设置中添加 Webhook:"
echo "   - 进入仓库 Settings > Webhooks > Add webhook"
echo "   - Payload URL: http://你的服务器IP:$WEBHOOK_PORT/webhook"
echo "   - Content type: application/json"
echo "   - Secret: $WEBHOOK_SECRET"
echo "   - 选择事件: Just the push event"
echo ""
echo "2. 如果服务器有防火墙，需要开放端口:"
echo "   sudo ufw allow $WEBHOOK_PORT"
echo ""
echo "3. 查看 webhook 日志:"
echo "   sudo journalctl -u opensqt-webhook -f"
echo ""
echo "4. 管理服务:"
echo "   sudo systemctl status opensqt-webhook  # 查看状态"
echo "   sudo systemctl restart opensqt-webhook # 重启服务"
echo "   sudo systemctl stop opensqt-webhook    # 停止服务"
echo ""
