#!/bin/bash

# OpenSQT 服务状态检查脚本

echo "================================"
echo "   OpenSQT 服务状态检查"
echo "================================"
echo ""

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 检查函数
check_process() {
    local name=$1
    local display_name=$2
    
    if pgrep -f "$name" > /dev/null 2>&1; then
        PID=$(pgrep -f "$name" | head -1)
        echo -e "${GREEN}✅ $display_name 运行中 (PID: $PID)${NC}"
        return 0
    else
        echo -e "${RED}❌ $display_name 未运行${NC}"
        return 1
    fi
}

echo "📦 二进制文件检查:"
if [ -f "opensqt" ] && [ -x "opensqt" ]; then
    echo "   ✅ opensqt 存在且可执行"
else
    echo "   ⚠️ opensqt 不存在或无执行权限"
fi

if [ -f "telegram_bot" ] && [ -x "telegram_bot" ]; then
    echo "   ✅ telegram_bot 存在且可执行"
else
    echo "   ⚠️ telegram_bot 不存在或无执行权限"
fi

echo ""
echo "🔧 服务状态:"

telegram_running=false
opensqt_running=false

if check_process "telegram_bot" "Telegram Bot"; then
    telegram_running=true
fi

if check_process "opensqt" "交易程序"; then
    opensqt_running=true
fi

echo ""
echo "📝 配置文件检查:"
if [ -f ".env" ]; then
    echo "   ✅ .env 存在"
else
    echo "   ❌ .env 不存在"
fi

if [ -f "config.yaml" ]; then
    echo "   ✅ config.yaml 存在"
else
    echo "   ❌ config.yaml 不存在"
fi

echo ""
echo "📊 日志文件检查:"
if [ -f "telegram_bot.log" ]; then
    SIZE=$(du -h telegram_bot.log | cut -f1)
    echo "   telegram_bot.log: $SIZE"
fi

if [ -f "opensqt.log" ]; then
    SIZE=$(du -h opensqt.log | cut -f1)
    echo "   opensqt.log: $SIZE"
fi

echo ""
echo "================================"
if $telegram_running && $opensqt_running; then
    echo -e "${GREEN}✅ 所有服务运行正常${NC}"
elif $telegram_running || $opensqt_running; then
    echo -e "${YELLOW}⚠️ 部分服务运行中${NC}"
else
    echo -e "${RED}❌ 服务未运行${NC}"
fi
echo "================================"
