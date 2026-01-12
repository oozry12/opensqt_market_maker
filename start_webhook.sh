#!/bin/bash

# 启动 Webhook 服务器

echo "🚀 启动 Webhook 服务器..."

# 检查是否已有实例在运行
if pgrep -f webhook_server > /dev/null; then
    echo "⚠️ 检测到已有 Webhook 服务器在运行"
    echo "正在停止旧实例..."
    pkill -f webhook_server
    sleep 2
fi

# 检查必要文件
if [ ! -f "webhook_server" ]; then
    echo "❌ webhook_server 文件不存在"
    echo "请先编译: go build -o webhook_server ./cmd/webhook_server"
    exit 1
fi

if [ ! -f ".env" ]; then
    echo "❌ .env 文件不存在"
    echo "请先配置: cp .env.example .env"
    exit 1
fi

# 加载环境变量
export $(grep -v '^#' .env | xargs)

# 设置默认值
if [ -z "$WEBHOOK_PORT" ]; then
    export WEBHOOK_PORT=9001
fi

if [ -z "$DEPLOY_SCRIPT" ]; then
    export DEPLOY_SCRIPT="./quick_deploy.sh"
fi

if [ -z "$WORK_DIR" ]; then
    export WORK_DIR="."
fi

# 确保部署脚本有执行权限
if [ -f "$DEPLOY_SCRIPT" ]; then
    chmod +x "$DEPLOY_SCRIPT"
fi

# 确保有执行权限
chmod +x webhook_server

# 启动服务器（后台运行）
nohup ./webhook_server > webhook.log 2>&1 &

# 等待启动
sleep 2

# 检查是否启动成功
if pgrep -f webhook_server > /dev/null; then
    PID=$(pgrep -f webhook_server)
    echo "✅ Webhook 服务器已启动 (PID: $PID)"
    echo "🌐 监听端口: $WEBHOOK_PORT"
    echo "📝 查看日志: tail -f webhook.log"
    echo ""
    echo "📡 配置 GitHub Webhook:"
    echo "   URL: http://your-server-ip:$WEBHOOK_PORT/webhook"
    echo "   Content type: application/json"
    echo "   Secret: (使用 .env 中的 WEBHOOK_SECRET)"
    echo "   Events: Just the push event"
else
    echo "❌ Webhook 服务器启动失败"
    echo "查看日志: cat webhook.log"
    exit 1
fi
