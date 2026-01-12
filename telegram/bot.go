package telegram

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gopkg.in/yaml.v3"
)

// Bot Telegram 机器人控制器
type Bot struct {
	api           *tgbotapi.BotAPI
	allowedUsers  map[int64]bool // 允许操作的用户ID
	tradingCmd    *exec.Cmd      // 交易进程
	tradingMu     sync.Mutex     // 进程锁
	configPath    string         // 配置文件路径
	workDir       string         // 工作目录（交易程序所在目录）
	exeName       string         // 可执行文件名
	isRunning     bool           // 交易程序是否运行中
	startTime     time.Time      // 启动时间
	logBuffer     []string       // 最近日志缓存
	logMu         sync.RWMutex   // 日志锁
	notifyChat    int64          // 通知聊天ID
	manualPID     int            // 手动启动的进程ID
}

// NewBot 创建 Telegram Bot
// workDir: 交易程序所在目录（服务器上的绝对路径）
// exeName: 可执行文件名（如 opensqt）
func NewBot(token string, allowedUserIDs []int64, workDir, exeName, configPath string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("创建 Telegram Bot 失败: %v", err)
	}

	allowedUsers := make(map[int64]bool)
	for _, id := range allowedUserIDs {
		allowedUsers[id] = true
	}

	// 如果未指定可执行文件名，根据系统自动判断
	if exeName == "" {
		if runtime.GOOS == "windows" {
			exeName = "opensqt.exe"
		} else {
			exeName = "opensqt"
		}
	}

	return &Bot{
		api:          api,
		allowedUsers: allowedUsers,
		workDir:      workDir,
		exeName:      exeName,
		configPath:   configPath,
		logBuffer:    make([]string, 0, 100),
	}, nil
}

// Start 启动 Bot 监听
func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	// 启动后主动发送功能面板给所有授权用户
	go func() {
		time.Sleep(2 * time.Second)
		for userID := range b.allowedUsers {
			b.sendWelcomePanel(userID)
		}
	}()

	for update := range updates {
		// 处理回调查询（按钮点击）
		if update.CallbackQuery != nil {
			b.handleCallbackQuery(update.CallbackQuery)
			continue
		}

		// 处理消息
		if update.Message == nil {
			continue
		}

		// 权限检查
		if !b.allowedUsers[update.Message.From.ID] {
			b.sendMessage(update.Message.Chat.ID, "⛔ 无权限操作")
			continue
		}

		b.handleCommand(update.Message)
	}
}

// handleCommand 处理命令
func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	switch msg.Command() {
	case "start", "help":
		b.sendHelp(chatID)
	case "run":
		b.startTrading(chatID)
	case "stop":
		b.stopTrading(chatID)
	case "status":
		b.sendStatus(chatID)
	case "restart":
		b.restartTrading(chatID)
	case "logs":
		b.sendLogs(chatID)
	case "update":
		b.gitPullAndRebuild(chatID)
	case "setsymbol":
		b.setSymbol(chatID, msg.CommandArguments())
	case "setpriceinterval":
		b.setPriceInterval(chatID, msg.CommandArguments())
	case "setorderquantity":
		b.setOrderQuantity(chatID, msg.CommandArguments())
	case "setminordervalue":
		b.setMinOrderValue(chatID, msg.CommandArguments())
	case "config":
		b.showConfig(chatID)
	case "panel":
		b.showConfigPanel(chatID)
	default:
		if msg.Text != "" && msg.Text[0] == '/' {
			b.sendMessage(chatID, "❓ 未知命令，输入 /help 查看帮助")
		}
	}
}

// sendHelp 发送帮助信息
func (b *Bot) sendHelp(chatID int64) {
	help := `🤖 *OpenSQT 交易控制*

*交易控制:*
/run - 启动交易程序
/stop - 停止交易程序
/restart - 重启交易程序
/status - 查看运行状态
/logs - 查看最近日志
/update - 下载最新版本并更新

*配置管理:*
/panel - 打开配置面板（推荐）
/setsymbol <交易对> - 设置交易对 (如 DOGEUSDC)
/setpriceinterval <价格间隔> - 设置价格间隔 (如 0.0001)
/setorderquantity <订单金额> - 设置每单金额 (如 12)
/setminordervalue <最小价值> - 设置最小订单价值 (如 10)
/config - 查看当前配置

*帮助:*
/help - 显示帮助`

	msg := tgbotapi.NewMessage(chatID, help)
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

// sendWelcomePanel 发送欢迎面板
func (b *Bot) sendWelcomePanel(chatID int64) {
	welcome := `🤖 *OpenSQT 交易控制 Bot 已上线*

欢迎使用交易控制面板！点击下方按钮快速操作`

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 查看状态", "status"),
			tgbotapi.NewInlineKeyboardButtonData("⚙️ 配置面板", "config_panel"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🚀 启动交易", "start_trading"),
			tgbotapi.NewInlineKeyboardButtonData("🛑 停止交易", "stop_trading"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📝 查看日志", "logs"),
			tgbotapi.NewInlineKeyboardButtonData("🔄 更新代码", "update_code"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❓ 帮助", "help"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, welcome)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

// startTrading 启动交易程序
func (b *Bot) startTrading(chatID int64) {
	b.tradingMu.Lock()
	defer b.tradingMu.Unlock()

	if b.isRunning {
		b.sendMessage(chatID, "⚠️ 交易程序已在运行中")
		return
	}

	// 检查是否有手动启动的进程
	isRunning, pid := b.checkTradingProcess()
	if isRunning {
		b.sendMessage(chatID, fmt.Sprintf("⚠️ 交易程序已在运行中 (手动启动, PID: %d)\n请先使用 /stop 停止现有进程", pid))
		return
	}

	b.sendMessage(chatID, "🚀 正在启动交易程序...")

	// 构建配置文件路径
	configPath := b.configPath
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(b.workDir, configPath)
	}

	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		b.sendMessage(chatID, fmt.Sprintf("❌ 配置文件不存在: %s", configPath))
		return
	}

	// 检查可执行文件是否存在
	exePath := filepath.Join(b.workDir, b.exeName)
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		b.sendMessage(chatID, fmt.Sprintf("❌ 可执行文件不存在: %s\n请先运行 /update 下载最新版本", exePath))
		return
	}

	// 使用二进制文件启动
	cmd := exec.Command("./"+b.exeName, configPath)
	cmd.Dir = b.workDir

	// 获取输出管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 获取输出管道失败: %v", err))
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 获取错误管道失败: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 启动失败: %v", err))
		return
	}

	b.tradingCmd = cmd
	b.isRunning = true
	b.startTime = time.Now()
	b.notifyChat = chatID

	// 清空日志缓存
	b.logMu.Lock()
	b.logBuffer = make([]string, 0, 100)
	b.logMu.Unlock()

	// 捕获输出
	go b.readOutput(stdout, chatID)
	go b.readOutput(stderr, chatID)

	// 监控进程退出
	go b.watchProcess(chatID)

	b.sendMessage(chatID, fmt.Sprintf("✅ 交易程序已启动\n📁 目录: %s\n⚙️ 配置: %s\n🚀 命令: ./%s", b.workDir, configPath, b.exeName))
}

// stopTrading 停止交易程序
func (b *Bot) stopTrading(chatID int64) {
	b.tradingMu.Lock()
	defer b.tradingMu.Unlock()

	if b.isRunning && b.tradingCmd != nil {
		b.sendMessage(chatID, "🛑 正在停止交易程序...")

		// 发送中断信号（优雅关闭）
		if err := b.tradingCmd.Process.Signal(os.Interrupt); err != nil {
			// 如果发送信号失败，直接 Kill
			b.tradingCmd.Process.Kill()
		}

		// 等待进程退出（最多15秒）
		done := make(chan error, 1)
		go func() {
			done <- b.tradingCmd.Wait()
		}()

		select {
		case <-done:
			b.sendMessage(chatID, "✅ 交易程序已停止")
		case <-time.After(15 * time.Second):
			b.tradingCmd.Process.Kill()
			b.sendMessage(chatID, "⚠️ 强制终止交易程序")
		}

		b.isRunning = false
		b.tradingCmd = nil
		return
	}

	// 检查是否有手动启动的进程
	isRunning, pid := b.checkTradingProcess()
	if isRunning {
		b.sendMessage(chatID, fmt.Sprintf("🛑 正在停止手动启动的交易程序 (PID: %d)...", pid))

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		} else {
			cmd = exec.Command("kill", "-9", strconv.Itoa(pid))
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			b.sendMessage(chatID, fmt.Sprintf("⚠️ 停止进程失败: %v\n输出: %s", err, string(output)))
			return
		}

		b.sendMessage(chatID, "✅ 交易程序已停止")
		return
	}

	b.sendMessage(chatID, "⚠️ 交易程序未运行")
}

// restartTrading 重启交易程序
func (b *Bot) restartTrading(chatID int64) {
	b.sendMessage(chatID, "🔄 正在重启交易程序...")

	// 先停止
	b.tradingMu.Lock()
	if b.isRunning && b.tradingCmd != nil {
		b.tradingCmd.Process.Signal(os.Interrupt)
		time.Sleep(3 * time.Second)
		if b.isRunning {
			b.tradingCmd.Process.Kill()
		}
		b.isRunning = false
		b.tradingCmd = nil
	} else {
		// 检查是否有手动启动的进程
		isRunning, pid := b.checkTradingProcess()
		if isRunning {
			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
			} else {
				cmd = exec.Command("kill", "-9", strconv.Itoa(pid))
			}
			cmd.Run()
		}
	}
	b.tradingMu.Unlock()

	time.Sleep(2 * time.Second)

	// 再启动
	b.startTrading(chatID)
}

// sendStatus 发送状态信息
func (b *Bot) sendStatus(chatID int64) {
	b.tradingMu.Lock()
	defer b.tradingMu.Unlock()

	var status string
	if b.isRunning {
		uptime := time.Since(b.startTime).Round(time.Second)
		pid := 0
		if b.tradingCmd != nil && b.tradingCmd.Process != nil {
			pid = b.tradingCmd.Process.Pid
		}
		status = fmt.Sprintf(`✅ *交易程序运行中* (Bot 启动)

⏱ 运行时间: %v
🔢 进程PID: %d
📁 工作目录: %s
⚙️ 配置文件: %s
🚀 启动命令: ./%s`, uptime, pid, b.workDir, b.configPath, b.exeName)
	} else {
		isRunning, pid := b.checkTradingProcess()
		if isRunning {
			status = fmt.Sprintf(`✅ *交易程序运行中* (手动启动)

🔢 进程PID: %d
📁 工作目录: %s
⚙️ 配置文件: %s
🚀 启动方式: 手动启动

⚠️ 注意: Bot 无法控制手动启动的进程，请手动停止`, pid, b.workDir, b.configPath)
		} else {
			status = fmt.Sprintf(`❌ *交易程序未运行*

📁 工作目录: %s
⚙️ 配置文件: %s`, b.workDir, b.configPath)
		}
	}

	msg := tgbotapi.NewMessage(chatID, status)
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

// sendLogs 发送最近日志
func (b *Bot) sendLogs(chatID int64) {
	b.logMu.RLock()
	bufferLen := len(b.logBuffer)
	var bufferLogs []string
	if bufferLen > 0 {
		bufferLogs = make([]string, bufferLen)
		copy(bufferLogs, b.logBuffer)
	}
	b.logMu.RUnlock()

	var logLines []string
	var source string

	// 如果内存缓存有日志，使用缓存
	if len(bufferLogs) > 0 {
		logLines = bufferLogs
		source = "实时"
	} else {
		// 否则尝试从日志文件读取（增加到100行）
		logLines = b.readLogFile(100)
		source = "文件"
		if len(logLines) == 0 {
			b.sendMessage(chatID, "📝 暂无日志\n\n💡 提示: 如果交易程序是手动启动的，请确保日志文件存在于 log/ 目录")
			return
		}
	}

	// 分段发送日志，每段不超过 3800 字符（留余量给格式）
	const maxChunkSize = 3800
	var chunks []string
	currentChunk := ""

	for _, line := range logLines {
		// 如果当前行加上已有内容超过限制，保存当前块并开始新块
		if len(currentChunk)+len(line)+1 > maxChunkSize {
			if currentChunk != "" {
				chunks = append(chunks, currentChunk)
			}
			currentChunk = line
		} else {
			if currentChunk != "" {
				currentChunk += "\n"
			}
			currentChunk += line
		}
	}
	if currentChunk != "" {
		chunks = append(chunks, currentChunk)
	}

	// 发送每个日志块
	for i, chunk := range chunks {
		var header string
		if len(chunks) == 1 {
			header = fmt.Sprintf("📝 *最近日志 (%s):*\n", source)
		} else {
			header = fmt.Sprintf("📝 *日志 (%s) [%d/%d]:*\n", source, i+1, len(chunks))
		}
		
		logs := header + "```\n" + chunk + "\n```"
		
		msg := tgbotapi.NewMessage(chatID, logs)
		msg.ParseMode = "Markdown"
		b.api.Send(msg)
		
		// 多条消息之间稍微延迟，避免发送过快
		if i < len(chunks)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// readLogFile 从日志文件读取最近的日志行
func (b *Bot) readLogFile(lines int) []string {
	// 获取今天的日志文件
	today := time.Now().Format("2006-01-02")
	logFileName := filepath.Join(b.workDir, "log", fmt.Sprintf("opensqt-%s.log", today))

	file, err := os.Open(logFileName)
	if err != nil {
		// 尝试昨天的日志文件
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		logFileName = filepath.Join(b.workDir, "log", fmt.Sprintf("opensqt-%s.log", yesterday))
		file, err = os.Open(logFileName)
		if err != nil {
			return nil
		}
	}
	defer file.Close()

	// 读取文件所有行
	var allLines []string
	scanner := bufio.NewScanner(file)
	// 增大缓冲区以处理长行
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	// 返回最后 N 行
	if len(allLines) <= lines {
		return allLines
	}
	return allLines[len(allLines)-lines:]
}

// sendMessage 发送消息
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	b.api.Send(msg)
}

// watchProcess 监控进程退出
func (b *Bot) watchProcess(chatID int64) {
	if b.tradingCmd == nil {
		return
	}

	err := b.tradingCmd.Wait()

	b.tradingMu.Lock()
	wasRunning := b.isRunning
	b.isRunning = false
	b.tradingCmd = nil
	b.tradingMu.Unlock()

	if !wasRunning {
		return // 已经被手动停止
	}

	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("⚠️ 交易程序异常退出: %v", err))
	} else {
		b.sendMessage(chatID, "ℹ️ 交易程序已退出")
	}
}

// readOutput 读取进程输出并缓存
func (b *Bot) readOutput(reader io.Reader, chatID int64) {
	scanner := bufio.NewScanner(reader)
	// 增大缓冲区以处理长行
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		b.appendLog(line)

		// 检测关键事件并推送通知
		b.checkAndNotify(chatID, line)
	}
}

// checkAndNotify 检测关键日志并推送通知
func (b *Bot) checkAndNotify(chatID int64, line string) {
	// 检测成交通知
	if contains(line, "买单成交") || contains(line, "卖单成交") {
		b.sendMessage(chatID, "💰 "+line)
	}
	// 检测风控触发
	if contains(line, "风控触发") || contains(line, "风控解除") {
		b.sendMessage(chatID, "🚨 "+line)
	}
	// 检测错误
	if contains(line, "❌") || contains(line, "失败") {
		b.sendMessage(chatID, "⚠️ "+line)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// appendLog 添加日志到缓存
func (b *Bot) appendLog(line string) {
	b.logMu.Lock()
	defer b.logMu.Unlock()

	b.logBuffer = append(b.logBuffer, line)
	// 保留最近100条
	if len(b.logBuffer) > 100 {
		b.logBuffer = b.logBuffer[len(b.logBuffer)-100:]
	}
}

// Notify 发送通知给所有授权用户
func (b *Bot) Notify(message string) {
	for userID := range b.allowedUsers {
		b.sendMessage(userID, message)
	}
}

// GetBotUsername 获取 Bot 用户名
func (b *Bot) GetBotUsername() string {
	return b.api.Self.UserName
}

// Stop 停止 Bot
func (b *Bot) Stop() {
	b.api.StopReceivingUpdates()
}

// checkTradingProcess 检查交易程序进程是否正在运行
// 返回：是否运行，进程ID
func (b *Bot) checkTradingProcess() (bool, int) {
	var cmd *exec.Cmd
	var processName string

	if runtime.GOOS == "windows" {
		cmd = exec.Command("tasklist", "/FI", "IMAGENAME eq "+b.exeName, "/FO", "CSV")
		processName = b.exeName
	} else {
		cmd = exec.Command("pgrep", "-f", "opensqt")
		processName = "opensqt"
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, 0
	}

	outputStr := string(output)

	if runtime.GOOS == "windows" {
		if strings.Contains(outputStr, processName) && !strings.Contains(outputStr, "No tasks are running") {
			lines := strings.Split(outputStr, "\n")
			for _, line := range lines {
				if strings.Contains(line, processName) {
					fields := strings.Split(line, ",")
					if len(fields) >= 2 {
						pidStr := strings.Trim(fields[1], "\"")
						pid, err := strconv.Atoi(pidStr)
						if err == nil && pid > 0 {
							return true, pid
						}
					}
				}
			}
		}
	} else {
		if len(strings.TrimSpace(outputStr)) > 0 {
			pids := strings.Fields(outputStr)
			if len(pids) > 0 {
				pid, err := strconv.Atoi(pids[0])
				if err == nil && pid > 0 {
					return true, pid
				}
			}
		}
	}

	return false, 0
}

// gitPullAndRebuild 下载最新的编译好的二进制文件
func (b *Bot) gitPullAndRebuild(chatID int64) {
	b.tradingMu.Lock()
	wasRunning := b.isRunning
	b.tradingMu.Unlock()

	// 如果正在运行，先停止
	if wasRunning {
		b.sendMessage(chatID, "⏸️ 先停止交易程序...")
		b.stopTrading(chatID)
		time.Sleep(2 * time.Second)
	}

	b.sendMessage(chatID, "📥 正在下载最新版本...")

	// 检测系统架构
	arch := runtime.GOARCH
	downloadURL := fmt.Sprintf("https://github.com/dennisyang1986/opensqt_market_maker/releases/download/latest/opensqt-linux-%s.tar.gz", arch)
	
	b.sendMessage(chatID, fmt.Sprintf("🔗 下载地址: %s", downloadURL))

	// 下载文件
	downloadPath := filepath.Join(b.workDir, "opensqt-latest.tar.gz")
	downloadCmd := exec.Command("wget", "-O", downloadPath, downloadURL)
	downloadCmd.Dir = b.workDir
	downloadOutput, err := downloadCmd.CombinedOutput()
	
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 下载失败:\n```\n%s\n```", string(downloadOutput)))
		return
	}

	b.sendMessage(chatID, "✅ 下载完成")

	// 备份当前版本
	b.sendMessage(chatID, "💾 备份当前版本...")
	backupDir := filepath.Join(b.workDir, "backup")
	os.MkdirAll(backupDir, 0755)
	
	if _, err := os.Stat(filepath.Join(b.workDir, b.exeName)); err == nil {
		exec.Command("cp", filepath.Join(b.workDir, b.exeName), filepath.Join(backupDir, b.exeName+".bak")).Run()
	}
	if _, err := os.Stat(filepath.Join(b.workDir, "telegram_bot")); err == nil {
		exec.Command("cp", filepath.Join(b.workDir, "telegram_bot"), filepath.Join(backupDir, "telegram_bot.bak")).Run()
	}

	// 解压文件
	b.sendMessage(chatID, "📦 正在解压...")
	extractCmd := exec.Command("tar", "-xzf", downloadPath, "-C", b.workDir)
	extractCmd.Dir = b.workDir
	extractOutput, err := extractCmd.CombinedOutput()
	
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 解压失败:\n```\n%s\n```", string(extractOutput)))
		return
	}

	// 添加执行权限
	os.Chmod(filepath.Join(b.workDir, b.exeName), 0755)
	os.Chmod(filepath.Join(b.workDir, "telegram_bot"), 0755)

	// 删除下载的压缩包
	os.Remove(downloadPath)

	b.sendMessage(chatID, "✅ 更新完成")

	b.sendMessage(chatID, "🔄 正在重启 Telegram Bot...")

	// 延迟一下，确保消息发送完成
	time.Sleep(2 * time.Second)

	// 重启 Telegram Bot
	restartCmd := exec.Command("nohup", "./telegram_bot", "&")
	restartCmd.Dir = b.workDir

	if err := restartCmd.Start(); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("⚠️ 重启 Bot 失败: %v", err))
		return
	}

	b.sendMessage(chatID, "✅ Telegram Bot 已重启")

	// 如果之前在运行，自动重新启动交易程序
	if wasRunning {
		time.Sleep(3 * time.Second)
		b.sendMessage(chatID, "🚀 自动重新启动交易程序...")
		b.startTrading(chatID)
	}

	// 延迟一下，确保消息发送完成
	time.Sleep(1 * time.Second)

	// 退出当前 Bot 进程
	b.Stop()
	os.Exit(0)
}

// 配置管理相关函数
type ConfigData struct {
	Trading struct {
		Symbol        string  `yaml:"symbol"`
		PriceInterval float64 `yaml:"price_interval"`
		OrderQuantity float64 `yaml:"order_quantity"`
		MinOrderValue float64 `yaml:"min_order_value"`
	} `yaml:"trading"`
}

func (b *Bot) loadConfig() (*ConfigData, error) {
	configPath := b.configPath
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(b.workDir, configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var cfg ConfigData
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	return &cfg, nil
}

func (b *Bot) saveConfig(cfg *ConfigData) error {
	configPath := b.configPath
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(b.workDir, configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	var fullConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &fullConfig); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	if trading, ok := fullConfig["trading"].(map[string]interface{}); ok {
		trading["symbol"] = cfg.Trading.Symbol
		trading["price_interval"] = cfg.Trading.PriceInterval
		trading["order_quantity"] = cfg.Trading.OrderQuantity
		trading["min_order_value"] = cfg.Trading.MinOrderValue
	}

	newData, err := yaml.Marshal(fullConfig)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

func (b *Bot) setSymbol(chatID int64, args string) {
	symbol := strings.TrimSpace(args)
	if symbol == "" {
		b.sendMessage(chatID, "❓ 用法: /setsymbol <交易对>\n示例: /setsymbol DOGEUSDC")
		return
	}

	cfg, err := b.loadConfig()
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 读取配置失败: %v", err))
		return
	}

	oldSymbol := cfg.Trading.Symbol
	cfg.Trading.Symbol = symbol

	if err := b.saveConfig(cfg); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 保存配置失败: %v", err))
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ 交易对已更新\n旧值: %s\n新值: %s", oldSymbol, symbol))
}

func (b *Bot) setPriceInterval(chatID int64, args string) {
	value, err := strconv.ParseFloat(strings.TrimSpace(args), 64)
	if err != nil || value <= 0 {
		b.sendMessage(chatID, "❓ 用法: /setpriceinterval <价格间隔>\n示例: /setpriceinterval 0.0001")
		return
	}

	cfg, err := b.loadConfig()
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 读取配置失败: %v", err))
		return
	}

	oldValue := cfg.Trading.PriceInterval
	cfg.Trading.PriceInterval = value

	if err := b.saveConfig(cfg); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 保存配置失败: %v", err))
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ 价格间隔已更新\n旧值: %.6f\n新值: %.6f", oldValue, value))
}

func (b *Bot) setOrderQuantity(chatID int64, args string) {
	value, err := strconv.ParseFloat(strings.TrimSpace(args), 64)
	if err != nil || value <= 0 {
		b.sendMessage(chatID, "❓ 用法: /setorderquantity <订单金额>\n示例: /setorderquantity 12")
		return
	}

	cfg, err := b.loadConfig()
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 读取配置失败: %v", err))
		return
	}

	oldValue := cfg.Trading.OrderQuantity
	cfg.Trading.OrderQuantity = value

	if err := b.saveConfig(cfg); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 保存配置失败: %v", err))
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ 订单金额已更新\n旧值: %.2f USDT\n新值: %.2f USDT", oldValue, value))
}

func (b *Bot) setMinOrderValue(chatID int64, args string) {
	value, err := strconv.ParseFloat(strings.TrimSpace(args), 64)
	if err != nil || value <= 0 {
		b.sendMessage(chatID, "❓ 用法: /setminordervalue <最小价值>\n示例: /setminordervalue 10")
		return
	}

	cfg, err := b.loadConfig()
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 读取配置失败: %v", err))
		return
	}

	oldValue := cfg.Trading.MinOrderValue
	cfg.Trading.MinOrderValue = value

	if err := b.saveConfig(cfg); err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 保存配置失败: %v", err))
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ 最小订单价值已更新\n旧值: %.2f USDT\n新值: %.2f USDT", oldValue, value))
}

func (b *Bot) showConfig(chatID int64) {
	cfg, err := b.loadConfig()
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 读取配置失败: %v", err))
		return
	}

	configInfo := fmt.Sprintf(`⚙️ *当前交易配置*

📊 交易对: %s
📏 价格间隔: %.6f
💰 订单金额: %.2f USDT
📉 最小订单价值: %.2f USDT

💡 提示: 修改配置后需要重启交易程序才能生效`, cfg.Trading.Symbol, cfg.Trading.PriceInterval, cfg.Trading.OrderQuantity, cfg.Trading.MinOrderValue)

	msg := tgbotapi.NewMessage(chatID, configInfo)
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

func (b *Bot) showConfigPanel(chatID int64) {
	cfg, err := b.loadConfig()
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 读取配置失败: %v", err))
		return
	}

	configInfo := fmt.Sprintf(`⚙️ *交易配置面板*

📊 交易对: %s
📏 价格间隔: %.6f
💰 订单金额: %.2f USDT
📉 最小订单价值: %.2f USDT

点击下方按钮修改配置`, cfg.Trading.Symbol, cfg.Trading.PriceInterval, cfg.Trading.OrderQuantity, cfg.Trading.MinOrderValue)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📊 设置交易对", "config_symbol"),
			tgbotapi.NewInlineKeyboardButtonData("📏 设置价格间隔", "config_price_interval"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💰 设置订单金额", "config_order_quantity"),
			tgbotapi.NewInlineKeyboardButtonData("📉 设置最小价值", "config_min_order_value"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 刷新配置", "config_refresh"),
			tgbotapi.NewInlineKeyboardButtonData("❌ 关闭面板", "config_close"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, configInfo)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	chatID := query.Message.Chat.ID
	data := query.Data

	if !b.allowedUsers[query.From.ID] {
		callback := tgbotapi.NewCallback(query.ID, "⛔ 无权限操作")
		b.api.Request(callback)
		return
	}

	switch data {
	case "status":
		callback := tgbotapi.NewCallback(query.ID, "正在获取状态...")
		b.api.Request(callback)
		b.sendStatus(chatID)
	case "config_panel":
		callback := tgbotapi.NewCallback(query.ID, "正在打开配置面板...")
		b.api.Request(callback)
		b.showConfigPanel(chatID)
	case "start_trading":
		callback := tgbotapi.NewCallback(query.ID, "正在启动交易...")
		b.api.Request(callback)
		b.startTrading(chatID)
	case "stop_trading":
		callback := tgbotapi.NewCallback(query.ID, "正在停止交易...")
		b.api.Request(callback)
		b.stopTrading(chatID)
	case "logs":
		callback := tgbotapi.NewCallback(query.ID, "正在获取日志...")
		b.api.Request(callback)
		b.sendLogs(chatID)
	case "update_code":
		callback := tgbotapi.NewCallback(query.ID, "正在更新代码...")
		b.api.Request(callback)
		b.gitPullAndRebuild(chatID)
	case "help":
		callback := tgbotapi.NewCallback(query.ID, "正在显示帮助...")
		b.api.Request(callback)
		b.sendHelp(chatID)
	case "config_symbol":
		callback := tgbotapi.NewCallback(query.ID, "请输入交易对，例如: DOGEUSDC")
		b.api.Request(callback)
		b.sendMessage(chatID, "请输入交易对，例如: DOGEUSDC\n使用 /setsymbol <交易对> 命令")
	case "config_price_interval":
		callback := tgbotapi.NewCallback(query.ID, "请输入价格间隔，例如: 0.0001")
		b.api.Request(callback)
		b.sendMessage(chatID, "请输入价格间隔，例如: 0.0001\n使用 /setpriceinterval <价格间隔> 命令")
	case "config_order_quantity":
		callback := tgbotapi.NewCallback(query.ID, "请输入订单金额，例如: 12")
		b.api.Request(callback)
		b.sendMessage(chatID, "请输入订单金额，例如: 12\n使用 /setorderquantity <订单金额> 命令")
	case "config_min_order_value":
		callback := tgbotapi.NewCallback(query.ID, "请输入最小订单价值，例如: 10")
		b.api.Request(callback)
		b.sendMessage(chatID, "请输入最小订单价值，例如: 10\n使用 /setminordervalue <最小价值> 命令")
	case "config_refresh":
		callback := tgbotapi.NewCallback(query.ID, "正在刷新配置...")
		b.api.Request(callback)
		b.showConfigPanel(chatID)
	case "config_close":
		callback := tgbotapi.NewCallback(query.ID, "已关闭配置面板")
		b.api.Request(callback)
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, query.Message.MessageID)
		b.api.Request(deleteMsg)
	default:
		callback := tgbotapi.NewCallback(query.ID, "未知操作")
		b.api.Request(callback)
	}
}