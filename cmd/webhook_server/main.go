package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	port       = flag.String("port", "8080", "Webhook服务器端口")
	secret     = flag.String("secret", "", "GitHub Webhook Secret")
	workDir    = flag.String("dir", ".", "工作目录")
	autoRestart = flag.Bool("restart", true, "是否自动重启服务")
)

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

func main() {
	flag.Parse()

	if *secret == "" {
		*secret = os.Getenv("WEBHOOK_SECRET")
	}

	log.Printf("🚀 启动 Webhook 服务器...")
	log.Printf("📡 监听端口: %s", *port)
	log.Printf("📁 工作目录: %s", *workDir)
	log.Printf("🔄 自动重启: %v", *autoRestart)

	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/health", handleHealth)

	log.Printf("✅ Webhook 服务器已启动: http://0.0.0.0:%s", *port)
	log.Printf("💡 Webhook URL: http://your-server-ip:%s/webhook", *port)
	
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatalf("❌ 启动服务器失败: %v", err)
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

	// 验证签名
	if *secret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, signature, *secret) {
			log.Printf("⚠️ 签名验证失败")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// 解析payload
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("❌ 解析payload失败: %v", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// 检查是否是main或master分支的push
	if payload.Ref != "refs/heads/main" && payload.Ref != "refs/heads/master" {
		log.Printf("⏭️ 忽略非主分支的push: %s", payload.Ref)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ignored"))
		return
	}

	log.Printf("📥 收到 push 事件:")
	log.Printf("   仓库: %s", payload.Repository.FullName)
	log.Printf("   分支: %s", payload.Ref)
	log.Printf("   提交: %s", payload.HeadCommit.ID[:7])
	log.Printf("   信息: %s", payload.HeadCommit.Message)

	// 异步处理更新
	go handleUpdate(payload)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Update triggered"))
}

func verifySignature(payload []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}

	// 移除 "sha256=" 前缀
	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedMAC))
}

func handleUpdate(payload WebhookPayload) {
	log.Printf("🔄 开始更新流程...")

	// 等待GitHub Actions完成编译（大约需要2-3分钟）
	log.Printf("⏳ 等待 GitHub Actions 完成编译...")
	time.Sleep(3 * time.Minute)

	// 停止当前运行的服务
	if *autoRestart {
		log.Printf("🛑 停止当前服务...")
		stopServices()
	}

	// 下载最新的二进制文件
	if err := downloadLatestRelease(); err != nil {
		log.Printf("❌ 下载失败: %v", err)
		return
	}

	// 重启服务
	if *autoRestart {
		log.Printf("🚀 重启服务...")
		time.Sleep(2 * time.Second)
		startServices()
	}

	log.Printf("✅ 更新完成!")
}

func stopServices() {
	// 停止 opensqt
	exec.Command("pkill", "-f", "opensqt").Run()
	
	// 停止 telegram_bot (但不停止当前的webhook服务器)
	exec.Command("pkill", "-f", "telegram_bot").Run()
	
	time.Sleep(2 * time.Second)
	log.Printf("✅ 服务已停止")
}

func downloadLatestRelease() error {
	log.Printf("📥 下载最新版本...")

	// 检测系统架构
	arch := runtime.GOARCH
	downloadURL := fmt.Sprintf("https://github.com/dennisyang1986/opensqt_market_maker/releases/download/latest/opensqt-linux-%s.tar.gz", arch)
	
	log.Printf("🔗 下载地址: %s", downloadURL)

	// 下载文件
	downloadCmd := exec.Command("wget", "-O", "opensqt-latest.tar.gz", downloadURL)
	downloadCmd.Dir = *workDir
	output, err := downloadCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("下载失败: %v, 输出: %s", err, string(output))
	}

	log.Printf("✅ 下载完成")

	// 解压文件
	log.Printf("📦 解压文件...")
	extractCmd := exec.Command("tar", "-xzf", "opensqt-latest.tar.gz")
	extractCmd.Dir = *workDir
	output, err = extractCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("解压失败: %v, 输出: %s", err, string(output))
	}

	// 添加执行权限
	chmodCmd := exec.Command("chmod", "+x", "opensqt", "telegram_bot")
	chmodCmd.Dir = *workDir
	chmodCmd.Run()

	// 删除压缩包
	os.Remove(filepath.Join(*workDir, "opensqt-latest.tar.gz"))

	log.Printf("✅ 文件已更新")
	return nil
}

func startServices() {
	// 启动 telegram_bot
	cmd := exec.Command("nohup", "./telegram_bot", ">", "telegram_bot.log", "2>&1", "&")
	cmd.Dir = *workDir
	if err := cmd.Start(); err != nil {
		log.Printf("⚠️ 启动 telegram_bot 失败: %v", err)
	} else {
		log.Printf("✅ telegram_bot 已启动")
	}

	time.Sleep(1 * time.Second)
}
