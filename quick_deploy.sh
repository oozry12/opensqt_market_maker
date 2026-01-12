#!/bin/bash

# OpenSQT 快速部署脚本
# 自动下载最新的二进制文件并启动

set -e

echo "🚀 OpenSQT 快速部署脚本"
echo ""

# 检测系统架构
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        GOARCH="amd64"
        ;;
    aarch64|arm64)
        GOARCH="arm64"
        ;;
    *)
        echo "❌ 不支持的架构: $ARCH"
        exit 1
        ;;
esac

echo "📋 检测到系统架构: $ARCH (Go: $GOARCH)"

# 下载地址
DOWNLOAD_URL="https://github.com/dennisyang1986/opensqt_market_maker/releases/download/latest/opensqt-linux-${GOARCH}.tar.gz"

echo "📥 正在下载最新版本..."
echo "🔗 下载地址: $DOWNLOAD_URL"

# 下载文件
if command -v wget &> /dev/null; then
    wget -O opensqt-latest.tar.gz "$DOWNLOAD_URL"
elif command -v curl &> /dev/null; then
    curl -L -o opensqt-latest.tar.gz "$DOWNLOAD_URL"
else
    echo "❌ 需要安装 wget 或 curl"
    exit 1
fi

echo "✅ 下载完成"

# 解压文件
echo "📦 正在解压..."
tar -xzf opensqt-latest.tar.gz

# 添加执行权限
chmod +x opensqt telegram_bot webhook_server

# 删除压缩包
rm opensqt-latest.tar.gz

echo "✅ 解压完成"
echo ""

# 检查配置文件
if [ ! -f ".env" ]; then
    echo "⚠️ .env 文件不存在"
    if [ -f ".env.example" ]; then
        echo "📝 创建 .env 文件..."
        cp .env.example .env
        echo "⚠️ 请编辑 .env 文件，填入以下配置："
        echo "   - TELEGRAM_BOT_TOKEN"
        echo "   - TELEGRAM_ALLOWED_USERS"
        echo "   - API 密钥"
        echo ""
        echo "编辑命令: nano .env"
        exit 0
    fi
fi

if [ ! -f "config.yaml" ]; then
    echo "⚠️ config.yaml 文件不存在"
    if [ -f "config.example.yaml" ]; then
        echo "📝 创建 config.yaml 文件..."
        cp config.example.yaml config.yaml
        echo "⚠️ 请编辑 config.yaml 文件，配置交易参数"
        echo ""
        echo "编辑命令: nano config.yaml"
        exit 0
    fi
fi

# 停止旧的 Bot 实例
if pgrep -f telegram_bot > /dev/null; then
    echo "🛑 停止旧的 Telegram Bot 实例..."
    pkill -f telegram_bot
    sleep 2
fi

# 停止旧的 Webhook 服务器（如果在运行）
WEBHOOK_WAS_RUNNING=false
if pgrep -f webhook_server > /dev/null; then
    echo "🛑 停止旧的 Webhook 服务器..."
    WEBHOOK_WAS_RUNNING=true
    pkill -f webhook_server
    sleep 2
fi

# 启动 Telegram Bot
echo "🤖 启动 Telegram Bot..."
nohup ./telegram_bot > telegram_bot.log 2>&1 &

sleep 2

# 如果之前 Webhook 服务器在运行，重新启动它
if [ "$WEBHOOK_WAS_RUNNING" = true ]; then
    echo "🔄 重启 Webhook 服务器..."
    if [ -f ".env" ]; then
        export $(grep -v '^#' .env | xargs)
        nohup ./webhook_server > webhook.log 2>&1 &
        sleep 2
        if pgrep -f webhook_server > /dev/null; then
            WEBHOOK_PID=$(pgrep -f webhook_server)
            echo "✅ Webhook 服务器已重启 (PID: $WEBHOOK_PID)"
        fi
    fi
fi

# 检查是否启动成功
if pgrep -f telegram_bot > /dev/null; then
    PID=$(pgrep -f telegram_bot)
    echo ""
    echo "✅ 部署完成！"
    echo ""
    echo "📊 状态信息:"
    echo "   - Telegram Bot PID: $PID"
    echo "   - 日志文件: telegram_bot.log"
    echo ""
    echo "📝 常用命令:"
    echo "   - 查看日志: tail -f telegram_bot.log"
    echo "   - 停止服务: ./stop_bot.sh"
    echo "   - 重启服务: ./start_bot.sh"
    echo ""
    echo "💡 现在可以在 Telegram 中向你的 Bot 发送命令："
    echo "   /run - 启动交易程序"
    echo "   /status - 查看状态"
    echo "   /help - 查看帮助"
else
    echo "❌ Telegram Bot 启动失败"
    echo "查看日志: cat telegram_bot.log"
    exit 1
fi
