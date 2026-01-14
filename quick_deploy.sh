#!/bin/bash

# OpenSQT 一键部署脚本
# 使用方法: ./quick_deploy.sh

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 OpenSQT 一键部署脚本${NC}"
echo "================================"

# 配置变量
REPO="oozry12/opensqt_market_maker"
API_URL="https://api.github.com/repos/$REPO/releases/latest"
WORK_DIR="."
BACKUP_DIR="backup_$(date +%Y%m%d_%H%M%S)"

# 检测架构
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ARCH="arm64"
else
    echo -e "${RED}❌ 不支持的架构: $ARCH${NC}"
    exit 1
fi

echo -e "${YELLOW}📦 检测到系统架构: $ARCH${NC}"

# 停止现有服务
echo -e "${YELLOW}🛑 停止现有服务...${NC}"
if [ -f "./telegram_bot" ]; then
    ./stop_bot.sh 2>/dev/null || true
fi

if pgrep -f "telegram_bot" > /dev/null 2>&1; then
    echo "正在停止 telegram_bot..."
    pkill -f telegram_bot || true
    sleep 2
fi

if pgrep -f "opensqt" > /dev/null 2>&1; then
    echo "正在停止 opensqt..."
    pkill -f opensqt || true
    sleep 2
fi

# 备份配置文件
echo -e "${YELLOW}💾 备份配置文件...${NC}"
if [ ! -d "$BACKUP_DIR" ]; then
    mkdir -p "$BACKUP_DIR"
fi

for file in .env config.yaml; do
    if [ -f "$file" ]; then
        cp "$file" "$BACKUP_DIR/"
        echo "  备份: $file"
    fi
done

# 下载最新版本
echo -e "${YELLOW}📥 下载最新版本...${NC}"
TAR_NAME="opensqt-linux-$ARCH.tar.gz"
DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/$TAR_NAME"

echo "  下载地址: $DOWNLOAD_URL"

# 下载（带重试）
MAX_RETRIES=3
RETRY_COUNT=0
while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if curl -L --connect-timeout 30 --max-time 300 -o "$TAR_NAME" "$DOWNLOAD_URL" 2>/dev/null; then
        if [ -s "$TAR_NAME" ]; then
            break
        fi
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo -e "${YELLOW}  下载失败，重试 ($RETRY_COUNT/$MAX_RETRIES)...${NC}"
    sleep 3
done

if [ ! -s "$TAR_NAME" ]; then
    echo -e "${RED}❌ 下载失败，请检查网络连接或手动下载${NC}"
    exit 1
fi

# 解压
echo -e "${YELLOW}📦 解压文件...${NC}"
tar -xzf "$TAR_NAME"

# 设置权限
echo -e "${YELLOW}🔧 设置权限...${NC}"
chmod +x opensqt telegram_bot *.sh 2>/dev/null || true

# 恢复配置文件
echo -e "${YELLOW}🔄 恢复配置文件...${NC}"
for file in .env config.yaml; do
    if [ -f "$BACKUP_DIR/$file" ]; then
        cp "$BACKUP_DIR/$file" ./
        echo "  恢复: $file"
    fi
done

# 清理
echo -e "${YELLOW}🧹 清理临时文件...${NC}"
rm -f "$TAR_NAME"
rm -rf "$BACKUP_DIR"

# 验证文件
echo -e "${YELLOW}✅ 验证文件...${NC}"
if [ ! -f "./telegram_bot" ]; then
    echo -e "${RED}❌ telegram_bot 文件不存在${NC}"
    exit 1
fi

if [ ! -f "./.env" ]; then
    echo -e "${RED}⚠️  .env 文件不存在，请创建配置文件${NC}"
    echo "  参考 .env.example 创建 .env 文件"
fi

if [ ! -f "./config.yaml" ]; then
    echo -e "${RED}⚠️  config.yaml 文件不存在，请创建配置文件${NC}"
fi

# 启动服务
echo -e "${GREEN}🚀 启动服务...${NC}"
nohup ./telegram_bot > telegram_bot.log 2>&1 &
sleep 3

# 检查进程
if pgrep -f "telegram_bot" > /dev/null; then
    PID=$(pgrep -f "telegram_bot")
    echo -e "${GREEN}✅ 服务启动成功!${NC}"
    echo "  PID: $PID"
    echo "  日志: telegram_bot.log"
else
    echo -e "${RED}❌ 服务启动失败${NC}"
    echo "  查看日志: tail -f telegram_bot.log"
    exit 1
fi

echo ""
echo -e "${GREEN}================================${NC}"
echo -e "${GREEN}✅ 部署完成!${NC}"
echo ""
echo "📝 下一步:"
echo "  1. 检查 .env 和 config.yaml 配置"
echo "  2. 发送 /start 给 Bot 测试"
echo "  3. 发送 /run 启动交易程序"
echo ""
echo "📊 查看日志:"
echo "  tail -f telegram_bot.log"
echo "  tail -f opensqt.log"
