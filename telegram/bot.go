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
/run - 启动交易程序 (go run main.go)
/stop - 停止交易程序
/restart - 重启交易程序
/status - 查看运行状态
/logs - 查看最近日志
/update - 拉取代码更新 (git pull)

*配置管理:*
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

// startTrading 启动交易程序
func (b *Bot) startTrading(chatID int64) {
	b.tradingMu.Lock()
	defer b.tradingMu.Unlock()

	if b.isRunning {
		b.sendMessage(chatID, "⚠️ 交易程序已在运行中")
		return
	}

	b.sendMessage(chatID, "📥 正在拉取最新代码...")

	pullCmd := exec.Command("git", "pull")
	pullCmd.Dir = b.workDir
	pullOutput, err := pullCmd.CombinedOutput()
	
	if err != nil {
		b.sendMessage(chatID, fmt.Sprintf("⚠️ Git pull 失败，继续启动:\n```\n%s\n```", string(pullOutput)))
	} else {
		b.sendMessage(chatID, fmt.Sprintf("✅ Git pull 完成:\n```\n%s\n```", string(pullOutput)))
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

	// 检查 main.go 是否存在
	mainFile := filepath.Join(b.workDir, "main.go")
	if _, err := os.Stat(mainFile); os.IsNotExist(err) {
		b.sendMessage(chatID, fmt.Sprintf("❌ main.go 不存在: %s", mainFile))
		return
	}

	// 使用 go run main.go 启动
	cmd := exec.Command("go", "run", "main.go", configPath)
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

	b.sendMessage(chatID, fmt.Sprintf("✅ 交易程序已启动\n📁 目录: %s\n⚙️ 配置: %s\n🚀 命令: go run main.go", b.workDir, configPath))
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
⚙️ 配置文件: %s
🚀 启动命令: go run main.go`, uptime, pid, b.workDir, b.configPath)
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

// gitPullAndRebuild 拉取更新
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

	// 如果之前在运行，重新启动
	if wasRunning {
		b.sendMessage(chatID, "🔄 重新启动交易程序...")
		time.Sleep(1 * time.Second)
		b.startTrading(chatID)
	}
}

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
