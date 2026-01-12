#!/bin/bash

# OpenSQT Telegram Bot 启动脚本

set -e

echo "🤖 启动 OpenSQT Telegram Bot..."

# 检查.env文件是否存在
if [ ! -f ".env" ]; then
    echo "❌ .env 文件不存在，请先配置环境变量"
    echo "复制示例文件: cp .env.example .env"
    echo "然后编辑 .env 文件，填入 TELEGRAM_BOT_TOKEN 和 TELEGRAM_ALLOWED_USERS"
    exit 1
fi

# 检查config.yaml是否存在
if [ ! -f "config.yaml" ]; then
    echo "❌ config.yaml 文件不存在，请先配置交易参数"
    echo "复制示例文件: cp config.example.yaml config.yaml"
    echo "然后编辑 config.yaml 文件"
    exit 1
fi

# 检查是否已编译
if [ ! -f "./telegram_bot" ] || [ ! -f "./opensqt" ]; then
    echo "📦 检测到未编译，正在编译程序..."
    ./scripts/build.sh
fi

echo "🚀 启动 Telegram Bot..."

# 启动Telegram Bot
./telegram_bot

echo "✅ Telegram Bot 已启动"
echo ""
echo "现在可以在Telegram中向你的Bot发送命令："
echo "  /run - 启动交易程序"
echo "  /status - 查看状态"
echo "  /help - 查看帮助"