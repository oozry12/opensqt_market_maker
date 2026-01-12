#!/bin/bash

# OpenSQT 快速部署脚本
# 自动下载最新的二进制文件并启动

set -e

# 解析命令行参数
ENABLE_WEBHOOK=false
for arg in "$@"; do
    case $arg in
        --enable-webhook)
            ENABLE_WEBHOOK=true
            shift
            ;;
        --help)
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --enable-webhook    启用 Webhook 自动部署服务器"
            echo "  --help              显示此帮助信息"
            echo ""
            echo "示例:"
            echo "  $0                  # 仅部署和启动 Telegram Bot"
            echo "  $0 --enable-webhook # 部署并启动 Webhook 服务器"
            exit 0
            ;;
    esac
done

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
DOWNLOAD_URL="https://github.com/oozry12/opensqt_market_maker/releases/download/latest/opensqt-linux-${GOARCH}.tar.gz"

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

# 启动 Webhook 服务器（如果指定了 --enable-webhook 或之前在运行）
if [ "$ENABLE_WEBHOOK" = true ] || [ "$WEBHOOK_WAS_RUNNING" = true ]; then
    echo "🔄 启动 Webhook 服务器..."
    
    # 检查 .env 文件
    if [ ! -f ".env" ]; then
        echo "⚠️ .env 文件不存在，无法启动 Webhook 服务器"
        echo "请创建 .env 文件并配置 WEBHOOK_SECRET 和 WEBHOOK_PORT"
    else
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
        
        # 检查 WEBHOOK_SECRET
        if [ -z "$WEBHOOK_SECRET" ]; then
            echo "⚠️ WEBHOOK_SECRET 未配置"
            echo "建议运行: echo \"WEBHOOK_SECRET=\$(openssl rand -hex 32)\" >> .env"
        fi
        
        # 启动 Webhook 服务器
        nohup ./webhook_server > webhook.log 2>&1 &
        sleep 2
        
        if pgrep -f webhook_server > /dev/null; then
            WEBHOOK_PID=$(pgrep -f webhook_server)
            echo "✅ Webhook 服务器已启动 (PID: $WEBHOOK_PID, 端口: $WEBHOOK_PORT)"
        else
            echo "❌ Webhook 服务器启动失败，查看日志: cat webhook.log"
        fi
    fi
fi

# 检查是否启动成功
if pgrep -f telegram_bot > /dev/null; then
    PID=$(pgrep -f telegram_bot)
    echo ""
    echo "✅ 部署完成！"
    echo ""
    echo "� 状态信息:""
    echo "   - Telegram Bot PID: $PID"
    echo "   - 日志文件: telegram_bot.log"
    
    # 显示 Webhook 状态
    if pgrep -f webhook_server > /dev/null; then
        WEBHOOK_PID=$(pgrep -f webhook_server)
        echo "   - Webhook 服务器 PID: $WEBHOOK_PID"
        echo "   - Webhook 日志: webhook.log"
    fi
    
    echo ""
    echo "📝 常用命令:"
    echo "   - 查看 Bot 日志: tail -f telegram_bot.log"
    echo "   - 查看 Webhook 日志: tail -f webhook.log"
    echo "   - 停止服务: ./stop_bot.sh"
    echo "   - 重启服务: ./start_bot.sh"
    echo ""
    echo "💡 现在可以在 Telegram 中向你的 Bot 发送命令："
    echo "   /run - 启动交易程序"
    echo "   /status - 查看状态"
    echo "   /help - 查看帮助"
    echo ""
    
    # 如果 Webhook 服务器未运行，提示如何启动
    if ! pgrep -f webhook_server > /dev/null; then
        echo "💡 启用自动部署功能（可选）："
        echo "   1. 配置 .env 文件:"
        echo "      echo \"WEBHOOK_SECRET=\$(openssl rand -hex 32)\" >> .env"
        echo "      echo \"WEBHOOK_PORT=9001\" >> .env"
        echo "   2. 重新运行: ./quick_deploy.sh --enable-webhook"
        echo "   3. 配置 GitHub Secrets:"
        echo "      - WEBHOOK_URL: http://your-server-ip:9001/webhook"
        echo "      - WEBHOOK_SECRET: (从 .env 复制)"
        echo ""
    else
        echo "🎉 Webhook 自动部署已启用！"
        echo "   配置 GitHub Secrets 即可实现自动部署："
        echo "   - WEBHOOK_URL: http://your-server-ip:$WEBHOOK_PORT/webhook"
        echo "   - WEBHOOK_SECRET: (从 .env 复制)"
        echo ""
    fi
else
    echo "❌ Telegram Bot 启动失败"
    echo "查看日志: cat telegram_bot.log"
    exit 1
fi
