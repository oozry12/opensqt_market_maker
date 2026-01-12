#!/bin/bash

# OpenSQT 一键启动脚本
# 自动拉取最新镜像并启动Telegram Bot

set -e

echo "🚀 OpenSQT 自动化部署启动..."

# 检查Docker是否安装
if ! command -v docker &> /dev/null; then
    echo "❌ Docker 未安装，请先安装Docker"
    echo "安装命令: curl -fsSL https://get.docker.com | sh"
    exit 1
fi

# 检查Docker Compose是否安装
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo "❌ Docker Compose 未安装，请先安装Docker Compose"
    exit 1
fi

# 检查Docker是否运行
if ! docker info &> /dev/null; then
    echo "❌ Docker 未运行，请启动Docker服务"
    echo "启动命令: sudo systemctl start docker"
    exit 1
fi

# 检查.env文件是否存在
if [ ! -f ".env" ]; then
    echo "❌ .env 文件不存在，请先配置环境变量"
    echo ""
    echo "请按以下步骤配置："
    echo "1. 复制示例文件: cp .env.example .env"
    echo "2. 编辑 .env 文件，填入以下必需的环境变量："
    echo "   - TELEGRAM_BOT_TOKEN=你的Bot Token"
    echo "   - TELEGRAM_ALLOWED_USERS=你的用户ID"
    echo "   - BINANCE_API_KEY=你的币安API Key (如果使用币安)"
    echo "   - BINANCE_SECRET_KEY=你的币安Secret Key"
    echo "   - 其他交易所的API密钥..."
    echo ""
    exit 1
fi

# 检查config.yaml是否存在
if [ ! -f "config.yaml" ]; then
    echo "❌ config.yaml 文件不存在，请先配置交易参数"
    echo "复制示例文件: cp config.example.yaml config.yaml"
    echo "然后编辑 config.yaml 文件，设置交易对、价格间隔等参数"
    exit 1
fi

# 创建必要的目录
mkdir -p logs

echo "📥 拉取最新Docker镜像..."
docker pull ghcr.io/dennisyang1986/opensqt-telegram:latest

echo "🚀 启动服务..."

# 使用docker-compose启动
if command -v docker-compose &> /dev/null; then
    docker-compose up -d
else
    docker compose up -d
fi

echo ""
echo "✅ OpenSQT 已成功启动！"
echo ""
echo "📱 现在可以在Telegram中向你的Bot发送命令："
echo "   /run - 启动交易程序"
echo "   /status - 查看运行状态"
echo "   /logs - 查看日志"
echo "   /help - 查看所有命令"
echo ""
echo "🔧 管理命令："
echo "   查看日志: docker logs -f opensqt-telegram"
echo "   停止服务: docker-compose down"
echo "   重启服务: docker-compose restart"
echo ""
