#!/bin/bash

# OpenSQT Webhook 服务器启动脚本

echo "🚀 启动 OpenSQT Webhook 服务器..."

# 检查是否已编译
if [ ! -f "webhook_server" ]; then
    echo "📦 编译 webhook 服务器..."
    go build -o webhook_server ./cmd/webhook_server
    chmod +x webhook_server
fi

# 从环境变量读取配置
WEBHOOK_SECRET=${WEBHOOK_SECRET:-""}
WEBHOOK_PORT=${WEBHOOK_PORT:-"8080"}

# 启动webhook服务器
nohup ./webhook_server \
    -port "$WEBHOOK_PORT" \
    -secret "$WEBHOOK_SECRET" \
    -dir "$(pwd)" \
    -restart=true \
    > webhook.log 2>&1 &

echo "✅ Webhook 服务器已启动"
echo "📡 端口: $WEBHOOK_PORT"
echo "📁 工作目录: $(pwd)"
echo "📝 日志文件: webhook.log"
echo ""
echo "查看日志: tail -f webhook.log"
echo "停止服务: pkill -f webhook_server"
echo ""
echo "GitHub Webhook 配置:"
echo "  Payload URL: http://your-server-ip:$WEBHOOK_PORT/webhook"
echo "  Content type: application/json"
echo "  Secret: $WEBHOOK_SECRET"
echo "  Events: Just the push event"
