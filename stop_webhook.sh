#!/bin/bash

# 停止 Webhook 服务器

echo "🛑 正在停止 Webhook 服务器..."

# 查找并停止所有 webhook_server 进程
pkill -f webhook_server

# 等待进程完全停止
sleep 2

# 检查是否还有残留进程
if pgrep -f webhook_server > /dev/null; then
    echo "⚠️ 发现残留进程，强制终止..."
    pkill -9 -f webhook_server
    sleep 1
fi

# 验证是否已停止
if pgrep -f webhook_server > /dev/null; then
    echo "❌ 无法停止 Webhook 服务器"
    echo "请手动检查: ps aux | grep webhook_server"
    exit 1
else
    echo "✅ Webhook 服务器已停止"
fi
