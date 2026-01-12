#!/bin/bash

# OpenSQT 编译脚本 (Linux)
# 用于编译主程序和 Telegram Bot

set -e

echo "🔨 开始编译 OpenSQT..."

# 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo "❌ Go 未安装或不在 PATH 中"
    exit 1
fi

echo "✅ Go 版本: $(go version)"

# 编译主程序
echo "🔨 编译主程序..."
go build -ldflags="-s -w" -o opensqt .
chmod +x opensqt
echo "✅ 主程序编译完成: opensqt"

# 编译 Telegram Bot
echo "🔨 编译 Telegram Bot..."
go build -ldflags="-s -w" -o telegram_bot ./cmd/telegram_bot
chmod +x telegram_bot
echo "✅ Telegram Bot 编译完成: telegram_bot"

# 编译 Webhook Server (可选)
echo "🔨 编译 Webhook Server..."
go build -ldflags="-s -w" -o webhook_server ./cmd/webhook_server
chmod +x webhook_server
echo "✅ Webhook Server 编译完成: webhook_server"

echo ""
echo "🎉 编译完成！"
echo ""
echo "使用方法："
echo "1. 启动主程序: ./opensqt config.yaml"
echo "2. 启动 Telegram Bot: ./telegram_bot"
echo "3. 启动 Webhook Server (可选): ./webhook_server"
echo ""
echo "或者使用 Telegram Bot 远程控制："
echo "1. 配置 .env 文件中的 TELEGRAM_BOT_TOKEN 和 TELEGRAM_ALLOWED_USERS"
echo "2. 启动 Bot: ./telegram_bot"
echo "3. 在 Telegram 中发送 /run 启动交易程序"