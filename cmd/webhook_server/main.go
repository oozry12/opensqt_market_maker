package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// WebhookPayload GitHub webhook payload
type WebhookPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	Pusher struct {
		Name string `json:"name"`
	} `json:"pusher"`
}

var (
	workDir       string
	webhookSecret string
	isDeploying   bool
)

func main() {
	// 从环境变量读取配置
	workDir = os.Getenv("WORK_DIR")
	if workDir == "" {
		workDir = "." // 默认当前目录
	}

	webhookSecret = os.Getenv("WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Println("⚠️ 警告: 未设置 WEBHOOK_SECRET，webhook 验证已禁用")
	}

	port := os.Getenv("WEBHOOK_PORT")
	if port == "" {
		port = "9000" // 默认端口
	}

	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/health", handleHealth)

	log.Printf("🚀 Webhook 服务器启动在端口 %s", port)
	log.Printf("📁 工作目录: %s", workDir)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("❌ 读取请求体失败: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 验证签名
	if webhookSecret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, signature, webhookSecret) {
			log.Printf("❌ Webhook 签名验证失败")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// 解析 payload
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("❌ 解析 payload 失败: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// 只处理 push 到 main 或 master 分支的事件
	if !strings.HasSuffix(payload.Ref, "/main") && !strings.HasSuffix(payload.Ref, "/master") {
		log.Printf("⏭️ 忽略非主分支的推送: %s", payload.Ref)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ignored: not main/master branch"))
		return
	}

	log.Printf("📥 收到 push 事件: %s by %s", payload.Repository.FullName, payload.Pusher.Name)

	// 检查是否正在部署
	if isDeploying {
		log.Printf("⚠️ 部署正在进行中，跳过此次请求")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("Deployment in progress"))
		return
	}

	// 异步执行部署
	go deploy()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Deployment started"))
}

func verifySignature(payload []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}

	// 移除 "sha256=" 前缀
	signature = strings.TrimPrefix(signature, "sha256=")

	// 计算 HMAC
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedMAC))
}

func deploy() {
	isDeploying = true
	defer func() {
		isDeploying = false
	}()

	log.Printf("🔄 开始部署...")

	// 1. 停止 telegram_bot
	log.Printf("⏸️ 停止 telegram_bot...")
	stopCmd := exec.Command("pkill", "-f", "telegram_bot")
	stopCmd.Dir = workDir
	if err := stopCmd.Run(); err != nil {
		log.Printf("⚠️ 停止 telegram_bot 失败 (可能未运行): %v", err)
	}
	time.Sleep(2 * time.Second)

	// 2. Git pull
	log.Printf("📥 拉取最新代码...")
	pullCmd := exec.Command("git", "pull")
	pullCmd.Dir = workDir
	pullOutput, err := pullCmd.CombinedOutput()
	if err != nil {
		log.Printf("❌ Git pull 失败: %v\n输出: %s", err, string(pullOutput))
		return
	}
	log.Printf("✅ Git pull 完成:\n%s", string(pullOutput))

	// 3. 编译主程序
	log.Printf("🔨 编译主程序...")
	buildMainCmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", "opensqt", ".")
	buildMainCmd.Dir = workDir
	buildMainOutput, err := buildMainCmd.CombinedOutput()
	if err != nil {
		log.Printf("❌ 编译主程序失败: %v\n输出: %s", err, string(buildMainOutput))
		return
	}
	log.Printf("✅ 主程序编译完成")

	// 4. 编译 telegram_bot
	log.Printf("🔨 编译 telegram_bot...")
	buildBotCmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", "telegram_bot", "./cmd/telegram_bot")
	buildBotCmd.Dir = workDir
	buildBotOutput, err := buildBotCmd.CombinedOutput()
	if err != nil {
		log.Printf("❌ 编译 telegram_bot 失败: %v\n输出: %s", err, string(buildBotOutput))
		return
	}
	log.Printf("✅ telegram_bot 编译完成")

	// 5. 启动 telegram_bot
	log.Printf("🚀 启动 telegram_bot...")
	startCmd := exec.Command("nohup", "./telegram_bot", "&")
	startCmd.Dir = workDir
	if err := startCmd.Start(); err != nil {
		log.Printf("❌ 启动 telegram_bot 失败: %v", err)
		return
	}

	log.Printf("✅ 部署完成！")
}
