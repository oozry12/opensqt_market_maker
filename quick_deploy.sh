#!/bin/bash

# OpenSQT 快速部署脚本
# 自动下载最新版本并配置

set -e

echo "🚀 OpenSQT 快速部署脚本"
echo ""

# 检测系统架构
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    GOARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    GOARCH="arm64"
else
    echo "❌ 不支持的架构: $ARCH"
    exit 1
fi

echo "✅ 检测到系统架构: $ARCH (Go架构: $GOARCH)"

# 下载最新版本
DOWNLOAD_URL="https://github.com/oozry12/opensqt_market_maker/releases/download/latest/opensqt-linux-${GOARCH}.tar.gz"
echo "📥 正在下载最新版本..."
echo "   下载地址: $DOWNLOAD_URL"

if ! wget -O opensqt-latest.tar.gz "$DOWNLOAD_URL"; then
    echo "❌ 下载失败"
    exit 1
fi

echo "✅ 下载完成"

# 解压文件
echo "📦 正在解压..."
tar -xzf opensqt-latest.tar.gz
chmod +x opensqt telegram_bot
rm opensqt-latest.tar.gz

echo "✅ 解压完成"

# 下载配置文件模板（如果不存在）
if [ ! -f "config.yaml" ]; then
    echo "📥 下载配置文件模板..."
    wget -O config.yaml https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/config.yaml || true
fi

if [ ! -f ".env" ]; then
    echo "📥 下载环境变量模板..."
    wget -O .env https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/.env.example || true
fi

echo ""
echo "🎉 部署完成！"
echo ""
echo "接下来的步骤："
echo "1. 编辑 .env 文件，填入 Telegram Bot Token 和 API 密钥"
echo "   nano .env"
echo ""
echo "2. 编辑 config.yaml 文件，配置交易参数"
echo "   nano config.yaml"
echo ""
echo "3. 启动 Telegram Bot"
echo "   ./telegram_bot"
echo ""
echo "4. 在 Telegram 中发送 /run 启动交易程序"
echo ""
echo "更多帮助: https://github.com/dennisyang1986/opensqt_market_maker"
