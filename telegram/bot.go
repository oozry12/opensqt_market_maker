package telegram

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
}

// NewBot 创建 Telegram Bot
// workDir: 交易程序所在目录（VPS上的绝对路径）
// exeName: 可执行文件名（如 opensqt 或 opensqt.exe）
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

	for update := range updates {
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
	default:
		if msg.Text != "" && msg.Text[0] == '/' {
			b.sendMessage(chatID, "❓ 未知命令，输入 /help 查看帮助")
		}
	}
}

// sendHelp 发送帮助信息
func (b *Bot) sendHelp(chatID int64) {
	help := `🤖 *OpenSQT 交易控制*

*可用命令:*
/run - 启动交易程序
/stop - 停止交易程序
/restart - 重启交易程序
/status - 查看运行状态
/logs - 查看最近日志
/update - 拉取更新并重新编译
/help - 显示帮助`

	msg := tgbotapi.NewMessage(chatID, help)
	msg.ParseMode = "Markdown"
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

	b.sendMessage(chatID, "🚀 正在启动交易程序...")

	// 构建可执行文件完整路径
	exePath := filepath.Join(b.workDir, b.exeName)
	configPath := b.configPath
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(b.workDir, configPath)
	}

	// 启动交易程序
	cmd := exec.Command(exePath, configPath)
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

	b.sendMessage(chatID, fmt.Sprintf("✅ 交易程序已启动\n📁 路径: %s\n⚙️ 配置: %s", exePath, configPath))
}

// stopTrading 停止交易程序
func (b *Bot) stopTrading(chatID int64) {
	b.tradingMu.Lock()
	defer b.tradingMu.Unlock()

	if !b.isRunning || b.tradingCmd == nil {
		b.sendMessage(chatID, "⚠️ 交易程序未运行")
		return
	}

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
		status = fmt.Sprintf(`✅ *交易程序运行中*

⏱ 运行时间: %v
🔢 进程PID: %d
📁 工作目录: %s
⚙️ 配置文件: %s`, uptime, pid, b.workDir, b.configPath)
	} else {
		status = fmt.Sprintf(`❌ *交易程序未运行*

📁 工作目录: %s
⚙️ 配置文件: %s`, b.workDir, b.configPath)
	}

	msg := tgbotapi.NewMessage(chatID, status)
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

// sendLogs 发送最近日志
func (b *Bot) sendLogs(chatID int64) {
	b.logMu.RLock()
	defer b.logMu.RUnlock()

	if len(b.logBuffer) == 0 {
		b.sendMessage(chatID, "📝 暂无日志")
		return
	}

	logs := "📝 *最近日志:*\n```\n"
	for _, line := range b.logBuffer {
		logs += line + "\n"
	}
	logs += "```"

	msg := tgbotapi.NewMessage(chatID, logs)
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
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

// gitPullAndRebuild 拉取更新并重新编译
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

	b.sendMessage(chatID, "📥 正在拉取更新...")

	// git pull
	pullCmd := exec.Command("git", "pull")
	pullCmd.Dir = b.workDir
	pullOutput, err := pullCmd.CombinedOutput()
	
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ Git pull 失败:\n```\n%s\n```", string(pullOutput)))
		return
	}

	b.sendMessage(chatID, fmt.Sprintf("✅ Git pull 完成:\n```\n%s\n```", string(pullOutput)))

	// 检查是否有更新
	if contains(string(pullOutput), "Already up to date") || contains(string(pullOutput), "已经是最新") {
		b.sendMessage(chatID, "ℹ️ 代码已是最新，无需重新编译")
		if wasRunning {
			b.sendMessage(chatID, "🔄 重新启动交易程序...")
			b.startTrading(chatID)
		}
		return
	}

	// 重新编译
	b.sendMessage(chatID, "🔨 正在重新编译...")

	buildCmd := exec.Command("go", "build", "-o", b.exeName, ".")
	buildCmd.Dir = b.workDir
	buildOutput, err := buildCmd.CombinedOutput()

	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("❌ 编译失败:\n```\n%s\n```", string(buildOutput)))
		return
	}

	b.sendMessage(chatID, "✅ 编译完成")

	// 如果之前在运行，重新启动
	if wasRunning {
		b.sendMessage(chatID, "🔄 重新启动交易程序...")
		b.startTrading(chatID)
	}
}
