#!/bin/bash

# OpenSQT 状态检查脚本
# 用于快速检查所有服务的运行状态

echo "=========================================="
echo "  OpenSQT 状态检查"
echo "=========================================="
echo ""

# 检查 Telegram Bot
echo "📱 Telegram Bot 状态:"
if pgrep -f telegram_bot > /dev/null; then
    PID=$(pgrep -f telegram_bot)
    echo "   ✅ 运行中 (PID: $PID)"
    
    # 检查日志中的最近错误
    if [ -f "telegram_bot.log" ]; then
        ERROR_COUNT=$(grep -i "error\|failed\|fatal" telegram_bot.log | tail -n 10 | wc -l)
        if [ $ERROR_COUNT -gt 0 ]; then
            echo "   ⚠️ 最近10行日志中有 $ERROR_COUNT 个错误"
        else
            echo "   ✅ 日志正常"
        fi
    fi
else
    echo "   ❌ 未运行"
fi
echo ""

# 检查 Webhook 服务器
echo "🌐 Webhook 服务器状态:"
if pgrep -f webhook_server > /dev/null; then
    PID=$(pgrep -f webhook_server)
    echo "   ✅ 运行中 (PID: $PID)"
    
    # 检查端口
    if command -v netstat &> /dev/null; then
        PORT=$(netstat -tlnp 2>/dev/null | grep webhook_server | awk '{print $4}' | cut -d: -f2)
        if [ ! -z "$PORT" ]; then
            echo "   ✅ 监听端口: $PORT"
        fi
    fi
    
    # 检查日志
    if [ -f "webhook.log" ]; then
        ERROR_COUNT=$(grep -i "error\|failed\|fatal" webhook.log | tail -n 10 | wc -l)
        if [ $ERROR_COUNT -gt 0 ]; then
            echo "   ⚠️ 最近10行日志中有 $ERROR_COUNT 个错误"
        else
            echo "   ✅ 日志正常"
        fi
    fi
else
    echo "   ⚠️ 未运行（可选服务）"
fi
echo ""

# 检查交易程序
echo "💹 交易程序状态:"
if pgrep -f "opensqt" > /dev/null; then
    PID=$(pgrep -f "opensqt")
    echo "   ✅ 运行中 (PID: $PID)"
else
    echo "   ⚠️ 未运行（通过 Telegram Bot 启动）"
fi
echo ""

# 检查配置文件
echo "📝 配置文件检查:"
if [ -f ".env" ]; then
    echo "   ✅ .env 文件存在"
    
    # 检查必要的配置项
    if grep -q "TELEGRAM_BOT_TOKEN=" .env && [ ! -z "$(grep TELEGRAM_BOT_TOKEN= .env | cut -d= -f2)" ]; then
        echo "   ✅ TELEGRAM_BOT_TOKEN 已配置"
    else
        echo "   ❌ TELEGRAM_BOT_TOKEN 未配置"
    fi
    
    if grep -q "TELEGRAM_ALLOWED_USERS=" .env && [ ! -z "$(grep TELEGRAM_ALLOWED_USERS= .env | cut -d= -f2)" ]; then
        echo "   ✅ TELEGRAM_ALLOWED_USERS 已配置"
    else
        echo "   ❌ TELEGRAM_ALLOWED_USERS 未配置"
    fi
else
    echo "   ❌ .env 文件不存在"
fi

if [ -f "config.yaml" ]; then
    echo "   ✅ config.yaml 文件存在"
else
    echo "   ❌ config.yaml 文件不存在"
fi
echo ""

# 检查二进制文件
echo "📦 二进制文件检查:"
if [ -f "telegram_bot" ] && [ -x "telegram_bot" ]; then
    echo "   ✅ telegram_bot 存在且可执行"
else
    echo "   ❌ telegram_bot 不存在或无执行权限"
fi

if [ -f "opensqt" ] && [ -x "opensqt" ]; then
    echo "   ✅ opensqt 存在且可执行"
else
    echo "   ❌ opensqt 不存在或无执行权限"
fi

if [ -f "webhook_server" ] && [ -x "webhook_server" ]; then
    echo "   ✅ webhook_server 存在且可执行"
else
    echo "   ⚠️ webhook_server 不存在或无执行权限（可选）"
fi
echo ""

# 检查磁盘空间
echo "💾 磁盘空间:"
DISK_USAGE=$(df -h . | tail -1 | awk '{print $5}' | sed 's/%//')
if [ $DISK_USAGE -lt 80 ]; then
    echo "   ✅ 磁盘使用率: ${DISK_USAGE}%"
elif [ $DISK_USAGE -lt 90 ]; then
    echo "   ⚠️ 磁盘使用率: ${DISK_USAGE}% (建议清理)"
else
    echo "   ❌ 磁盘使用率: ${DISK_USAGE}% (空间不足)"
fi
echo ""

# 检查日志文件大小
echo "📊 日志文件大小:"
if [ -f "telegram_bot.log" ]; then
    SIZE=$(du -h telegram_bot.log | cut -f1)
    echo "   telegram_bot.log: $SIZE"
fi

if [ -f "webhook.log" ]; then
    SIZE=$(du -h webhook.log | cut -f1)
    echo "   webhook.log: $SIZE"
fi

if [ -f "opensqt.log" ]; then
    SIZE=$(du -h opensqt.log | cut -f1)
    echo "   opensqt.log: $SIZE"
fi
echo ""

# 总结
echo "=========================================="
echo "  状态检查完成"
echo "=========================================="
echo ""

# 提供建议
BOT_RUNNING=$(pgrep -f telegram_bot > /dev/null && echo "yes" || echo "no")
CONFIG_OK=$([ -f ".env" ] && [ -f "config.yaml" ] && echo "yes" || echo "no")

if [ "$BOT_RUNNING" = "yes" ] && [ "$CONFIG_OK" = "yes" ]; then
    echo "✅ 系统运行正常"
    echo ""
    echo "💡 常用命令:"
    echo "   查看 Bot 日志: tail -f telegram_bot.log"
    echo "   查看 Webhook 日志: tail -f webhook.log"
    echo "   重启 Bot: ./stop_bot.sh && ./start_bot.sh"
    echo "   更新程序: ./quick_deploy.sh"
else
    echo "⚠️ 系统需要注意"
    echo ""
    if [ "$BOT_RUNNING" = "no" ]; then
        echo "❌ Telegram Bot 未运行"
        echo "   启动命令: ./start_bot.sh"
        echo ""
    fi
    if [ "$CONFIG_OK" = "no" ]; then
        echo "❌ 配置文件缺失"
        echo "   下载配置: wget https://raw.githubusercontent.com/oozry12/opensqt_market_maker/main/.env.example -O .env"
        echo "   编辑配置: nano .env"
        echo ""
    fi
fi
