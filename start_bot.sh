#!/bin/bash

# 启动 Telegram Bot（确保只有一个实例运行）

echo "🤖 启动 Telegram Bot..."

# 检查是否已有实例在运行
if pgrep -f telegram_bot > /dev/null; then
    echo "⚠️ 检测到已有 Telegram Bot 实例在运行"
    echo "正在停止旧实例..."
    pkill -f telegram_bot
    sleep 2
    
    # 如果还有残留，强制终止
    if pgrep -f telegram_bot > /dev/null; then
        pkill -9 -f telegram_bot
        sleep 1
    fi
fi

# 检查必要文件
if [ ! -f "telegram_bot" ]; then
    echo "❌ telegram_bot 文件不存在"
    echo "请先下载或编译: ./scripts/build.sh"
    exit 1
fi

if [ ! -f ".env" ]; then
    echo "❌ .env 文件不存在"
    echo "请先配置: cp .env.example .env"
    exit 1
fi

if [ ! -f "config.yaml" ]; then
    echo "❌ config.yaml 文件不存在"
    echo "请先配置: cp config.example.yaml config.yaml"
    exit 1
fi

# 确保有执行权限
chmod +x telegram_bot

# 启动 Bot（后台运行）
nohup ./telegram_bot > telegram_bot.log 2>&1 &

# 等待启动
sleep 2

# 检查是否启动成功
if pgrep -f telegram_bot > /dev/null; then
    PID=$(pgrep -f telegram_bot)
    echo "✅ Telegram Bot 已启动 (PID: $PID)"
    echo "📝 查看日志: tail -f telegram_bot.log"
    echo "🛑 停止服务: ./stop_bot.sh"
else
    echo "❌ Telegram Bot 启动失败"
    echo "查看日志: cat telegram_bot.log"
    exit 1
fi
