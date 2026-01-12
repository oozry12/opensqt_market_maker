package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// WebhookPayload GitHub webhook payload
type WebhookPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit struct {
		Message string `json:"message"`
		ID      string `json:"id"`
	} `json:"head_commit"`
}

var (
	webhookSecret string
	deployScript  string
	workDir       string
)

func main() {
	// 从环境变量读取配置
	webhookSecret = os.Getenv("WEBHOOK_SECRET")
	deployScript = os.Getenv("DEPLOY_SCRIPT")
	workDir = os.Getenv("WORK_DIR")
	port := os.Getenv("WEBHOOK_PORT")

	// 设置默认值
	if deployScript == "" {
		deployScript = "./quick_deploy.sh"
	}
	if workDir == "" {
		workDir = "."
	}
	if port == "" {
		port = "9000"
	}

	log.Printf("🚀 Webhook 服务器启动中...")
	log.Printf("📁 工作目录: %s", workDir)
	log.Printf("📜 部署脚本: %s", deployScript)
	log.Printf("🔐 Secret: %s", maskSecret(webhookSecret))
	log.Printf("🌐 监听端口: %s", port)

	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/health", handleHealth)

	log.Printf("✅ Webhook 服务器已启动，监听端口 %s", port)
	log.Printf("📡 Webhook URL: http://your-server:%s/webhook", port)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("❌ 服务器启动失败: %v", err)
	}
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

	// 验证签名（如果配置了 secret）
	if webhookSecret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, signature, webhookSecret) {
			log.Printf("⚠️ 签名验证失败")
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
	if payload.Ref != "refs/heads/main" && payload.Ref != "refs/heads/master" {
		log.Printf("ℹ️ 忽略非主分支的推送: %s", payload.Ref)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ignored"))
		return
	}

	log.Printf("📥 收到 webhook: %s", payload.Repository.FullName)
	log.Printf("📝 提交信息: %s", payload.HeadCommit.Message)
	log.Printf("🔖 提交ID: %s", payload.HeadCommit.ID[:7])

	// 异步执行部署脚本
	go executeDeploy(payload)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Deployment triggered"))
}

func executeDeploy(payload WebhookPayload) {
	log.Printf("🚀 开始执行部署脚本...")

	// 执行部署脚本
	cmd := exec.Command("/bin/bash", deployScript)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("COMMIT_MESSAGE=%s", payload.HeadCommit.Message),
		fmt.Sprintf("COMMIT_ID=%s", payload.HeadCommit.ID),
	)

	output, err := cmd.CombinedOutput()
	
	if err != nil {
		log.Printf("❌ 部署失败: %v", err)
		log.Printf("输出:\n%s", string(output))
		return
	}

	log.Printf("✅ 部署成功")
	log.Printf("输出:\n%s", string(output))
}

func verifySignature(payload []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}

	// GitHub 使用 sha256=<hash> 格式
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	expectedHash := signature[7:] // 移除 "sha256=" 前缀

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	actualHash := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedHash), []byte(actualHash))
}

func maskSecret(secret string) string {
	if secret == "" {
		return "未设置"
	}
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "****" + secret[len(secret)-4:]
}
