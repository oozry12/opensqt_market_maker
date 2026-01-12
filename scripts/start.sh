#!/bin/bash

# OpenSQT 启动脚本 (Linux)

echo "🚀 OpenSQT 启动脚本"
echo

# 检查可执行文件是否存在
if [ ! -f "opensqt" ]; then
    echo "❌ opensqt 不存在，请先运行编译脚本"
    echo "运行: ./scripts/build.sh"
    exit 1
fi

if [ ! -f "telegram_bot" ]; then
    echo "❌ telegram_bot 不存在，请先运行编译脚本"
    echo "运行: ./scripts/build.sh"
    exit 1
fi

# 检查配置文件
if [ ! -f "config.yaml" ]; then
    echo "❌ config.yaml 不存在，请先配置交易参数"
    exit 1
fi

echo "请选择启动方式："
echo "1. 直接启动交易程序"
echo "2. 启动 Telegram Bot（推荐，支持远程控制）"
echo "3. 同时启动两个程序（后台运行）"
echo
read -p "请输入选择 (1/2/3): " choice

case $choice in
    1)
        echo "🚀 启动交易程序..."
        ./opensqt config.yaml
        ;;
    2)
        echo "🤖 启动 Telegram Bot..."
        ./telegram_bot
        ;;
    3)
        echo "🚀 同时启动两个程序..."
        nohup ./opensqt config.yaml > opensqt.log 2>&1 &
        echo "✅ 交易程序已在后台启动，日志: opensqt.log"
        nohup ./telegram_bot > telegram_bot.log 2>&1 &
        echo "✅ Telegram Bot 已在后台启动，日志: telegram_bot.log"
        echo ""
        echo "查看进程: ps aux | grep opensqt"
        echo "停止所有进程: pkill opensqt && pkill telegram_bot"
        echo "查看交易日志: tail -f opensqt.log"
        echo "查看Bot日志: tail -f telegram_bot.log"
        ;;
    *)
        echo "❌ 无效选择"
        exit 1
        ;;
esac