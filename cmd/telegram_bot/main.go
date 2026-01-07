package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"opensqt/telegram"
)

func main() {
	// 命令行参数
	workDir := flag.String("dir", ".", "交易程序所在目录")
	exeName := flag.String("exe", "", "可执行文件名（默认自动检测）")
	configPath := flag.String("config", "config.yaml", "交易配置文件路径")
	flag.Parse()

	fmt.Println("🤖 OpenSQT Telegram 控制器启动中...")

	// 加载 .env 文件
	loadEnvFile(".env")

	// 加载 Telegram 配置
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		fmt.Println("❌ 未设置 TELEGRAM_BOT_TOKEN 环境变量")
		printUsage()
		os.Exit(1)
	}

	userIDsStr := os.Getenv("TELEGRAM_ALLOWED_USERS")
	if userIDsStr == "" {
		fmt.Println("❌ 未设置 TELEGRAM_ALLOWED_USERS 环境变量")
		printUsage()
		os.Exit(1)
	}

	// 解析用户ID
	userIDs := parseUserIDs(userIDsStr)
	if len(userIDs) == 0 {
		fmt.Println("❌ TELEGRAM_ALLOWED_USERS 格式错误")
		printUsage()
		os.Exit(1)
	}

	// 创建 Bot
	bot, err := telegram.NewBot(token, userIDs, *workDir, *exeName, *configPath)
	if err != nil {
		fmt.Printf("❌ 创建 Bot 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Bot @%s 已启动\n", bot.GetBotUsername())
	fmt.Printf("📁 工作目录: %s\n", *workDir)
	fmt.Printf("⚙️ 配置文件: %s\n", *configPath)
	fmt.Printf("👤 授权用户: %v\n", userIDs)
	fmt.Println("\n可用命令: /run /stop /restart /status /logs /help")

	// 优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n🛑 正在关闭 Bot...")
		bot.Stop()
		os.Exit(0)
	}()

	// 启动监听
	bot.Start()
}

func printUsage() {
	fmt.Println("\n请设置以下环境变量:")
	fmt.Println("  TELEGRAM_BOT_TOKEN=你的Bot Token")
	fmt.Println("  TELEGRAM_ALLOWED_USERS=用户ID1,用户ID2")
	fmt.Println("\n或在 .env 文件中配置")
	fmt.Println("\n命令行参数:")
	fmt.Println("  -dir    交易程序所在目录（默认当前目录）")
	fmt.Println("  -exe    可执行文件名（默认自动检测）")
	fmt.Println("  -config 配置文件路径（默认config.yaml）")
}

func parseUserIDs(s string) []int64 {
	var ids []int64
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if id, err := strconv.ParseInt(part, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// loadEnvFile 从 .env 文件加载环境变量
func loadEnvFile(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return // 文件不存在是正常的
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 移除引号
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// 系统环境变量优先
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
}
