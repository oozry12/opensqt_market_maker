#!/bin/bash

# OpenSQT 初始化安装脚本
# 用于首次部署，下载最新的编译好的二进制文件

set -e

echo "🚀 OpenSQT 初始化安装"
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

echo "✅ 检测到系统架构: $ARCH (Go架构: $GOARCH)"

# 下载地址
DOWNLOAD_URL="https://github.com/oozry12/opensqt_market_maker/releases/download/latest/opensqt-linux-$GOARCH.tar.gz"
echo "📥 下载地址: $DOWNLOAD_URL"

# 下载文件
echo "📥 正在下载最新版本..."
wget -O opensqt-latest.tar.gz "$DOWNLOAD_URL"

if [ $? -ne 0 ]; then
    echo "❌ 下载失败"
    exit 1
fi

echo "✅ 下载完成"

# 解压文件
echo "📦 正在解压..."
tar -xzf opensqt-latest.tar.gz

if [ $? -ne 0 ]; then
    echo "❌ 解压失败"
    exit 1
fi

# 添加执行权限
chmod +x opensqt telegram_bot

# 删除压缩包
rm opensqt-latest.tar.gz

echo "✅ 解压完成"
echo ""

# 检查配置文件
if [ ! -f "config.yaml" ]; then
    echo "⚠️ 配置文件不存在"
    if [ -f "config.example.yaml" ]; then
        echo "📝 复制示例配置文件..."
        cp config.example.yaml config.yaml
        echo "✅ 已创建 config.yaml，请编辑配置文件"
    else
        echo "❌ 未找到 config.example.yaml"
    fi
fi

# 检查环境变量文件
if [ ! -f ".env" ]; then
    echo "⚠️ 环境变量文件不存在"
    if [ -f ".env.example" ]; then
        echo "📝 复制示例环境变量文件..."
        cp .env.example .env
        echo "✅ 已创建 .env，请编辑环境变量文件"
    else
        echo "❌ 未找到 .env.example"
    fi
fi

echo ""
echo "🎉 安装完成！"
echo ""
echo "下一步操作："
echo "1. 编辑 config.yaml 配置交易参数"
echo "2. 编辑 .env 配置 API 密钥和 Telegram Bot"
echo "3. 启动 Telegram Bot: ./telegram_bot"
echo "4. 在 Telegram 中发送 /run 启动交易程序"
echo ""
echo "或者直接启动交易程序: ./opensqt config.yaml"