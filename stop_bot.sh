#!/bin/bash

# 停止所有运行中的 Telegram Bot 实例

echo "🛑 正在停止所有 Telegram Bot 实例..."

# 查找并停止所有 telegram_bot 进程
pkill -f telegram_bot

# 等待进程完全停止
sleep 2

# 检查是否还有残留进程
if pgrep -f telegram_bot > /dev/null; then
    echo "⚠️ 发现残留进程，强制终止..."
    pkill -9 -f telegram_bot
    sleep 1
fi

# 验证是否已停止
if pgrep -f telegram_bot > /dev/null; then
    echo "❌ 无法停止 Telegram Bot 进程"
    echo "请手动检查: ps aux | grep telegram_bot"
    exit 1
else
    echo "✅ 所有 Telegram Bot 实例已停止"
fi
