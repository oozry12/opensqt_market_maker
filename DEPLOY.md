# OpenSQT 部署指南

## 📋 Linux 服务器部署

### 环境要求

- **操作系统**: Linux (Ubuntu 18.04+, CentOS 7+, Debian 9+)
- **Go 版本**: 1.21 或更高版本
- **内存**: 最少 512MB，推荐 1GB+
- **网络**: 能访问交易所 API

### 1. 安装 Go 环境

#### Ubuntu/Debian:
```bash
# 更新包列表
sudo apt update

# 安装 Go
sudo apt install golang-go

# 验证安装
go version
```

#### CentOS/RHEL:
```bash
# 安装 Go
sudo yum install golang

# 或者使用 dnf (CentOS 8+)
sudo dnf install golang

# 验证安装
go version
```

#### 手动安装最新版本:
```bash
# 下载 Go 1.21+
wget https://go.dev/dl/go1.21.6.linux-amd64.tar.gz

# 解压到 /usr/local
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz

# 添加到 PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 验证安装
go version
```

### 2. 部署 OpenSQT

#### 克隆代码
```bash
# 克隆仓库
git clone https://github.com/your-username/opensqt_market_maker.git
cd opensqt_market_maker
```

#### 编译程序
```bash
# 添加执行权限
chmod +x scripts/build.sh

# 编译
./scripts/build.sh
```

#### 配置交易参数
```bash
# 复制配置文件
cp config.example.yaml config.yaml

# 编辑配置（使用你喜欢的编辑器）
nano config.yaml
# 或
vim config.yaml
```

#### 配置 Telegram Bot（可选）
```bash
# 复制环境变量文件
cp .env.example .env

# 编辑环境变量
nano .env
```

在 `.env` 文件中填入：
```bash
TELEGRAM_BOT_TOKEN=你的Bot Token
TELEGRAM_ALLOWED_USERS=你的用户ID
```

### 3. 启动程序

#### 方式一：直接启动
```bash
# 启动交易程序
./opensqt config.yaml
```

#### 方式二：Telegram Bot 控制
```bash
# 启动 Telegram Bot
./telegram_bot

# 然后在 Telegram 中发送 /run 启动交易程序
```

#### 方式三：后台运行
```bash
# 使用 nohup 后台运行
nohup ./opensqt config.yaml > opensqt.log 2>&1 &

# 或者使用 screen
screen -S opensqt
./opensqt config.yaml
# 按 Ctrl+A, D 分离会话

# 重新连接会话
screen -r opensqt
```

### 4. 进程管理

#### 查看进程
```bash
# 查看 OpenSQT 进程
ps aux | grep opensqt

# 查看端口占用
netstat -tlnp | grep opensqt
```

#### 停止进程
```bash
# 通过 Telegram Bot
# 发送 /stop 命令

# 或者直接杀进程
pkill opensqt
pkill telegram_bot
```

#### 查看日志
```bash
# 实时查看日志
tail -f opensqt.log

# 查看最近100行
tail -n 100 opensqt.log
```

### 5. 系统服务配置（可选）

创建 systemd 服务文件：

```bash
sudo nano /etc/systemd/system/opensqt.service
```

内容：
```ini
[Unit]
Description=OpenSQT Market Maker
After=network.target

[Service]
Type=simple
User=your-username
WorkingDirectory=/path/to/opensqt_market_maker
ExecStart=/path/to/opensqt_market_maker/opensqt config.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启用服务：
```bash
# 重新加载 systemd
sudo systemctl daemon-reload

# 启用服务
sudo systemctl enable opensqt

# 启动服务
sudo systemctl start opensqt

# 查看状态
sudo systemctl status opensqt

# 查看日志
sudo journalctl -u opensqt -f
```

### 6. 安全建议

#### 防火墙配置
```bash
# Ubuntu/Debian
sudo ufw enable
sudo ufw allow ssh
sudo ufw allow from your-ip-address

# CentOS/RHEL
sudo firewall-cmd --permanent --add-service=ssh
sudo firewall-cmd --permanent --add-source=your-ip-address
sudo firewall-cmd --reload
```

#### SSH 密钥认证
```bash
# 生成密钥对（在本地机器）
ssh-keygen -t rsa -b 4096

# 复制公钥到服务器
ssh-copy-id user@server-ip

# 禁用密码登录
sudo nano /etc/ssh/sshd_config
# 设置: PasswordAuthentication no
sudo systemctl restart sshd
```

#### 定期备份
```bash
# 创建备份脚本
nano backup.sh
```

内容：
```bash
#!/bin/bash
DATE=$(date +%Y%m%d_%H%M%S)
tar -czf "opensqt_backup_$DATE.tar.gz" config.yaml .env *.log
```

### 7. 监控和维护

#### 系统资源监控
```bash
# 查看系统资源
htop
# 或
top

# 查看磁盘使用
df -h

# 查看内存使用
free -h
```

#### 自动更新脚本
```bash
nano update.sh
```

内容：
```bash
#!/bin/bash
echo "🔄 更新 OpenSQT..."

# 停止程序
pkill opensqt
pkill telegram_bot

# 拉取最新代码
git pull

# 重新编译
./scripts/build.sh

# 重新启动
nohup ./telegram_bot > telegram_bot.log 2>&1 &
echo "✅ 更新完成"
```

### 8. 故障排除

#### 常见问题

**Q: 编译失败**
```bash
# 检查 Go 版本
go version

# 清理模块缓存
go clean -modcache
go mod download
```

**Q: 网络连接问题**
```bash
# 测试网络连接
curl -I https://api.binance.com/api/v3/ping

# 检查 DNS
nslookup api.binance.com
```

**Q: 权限问题**
```bash
# 检查文件权限
ls -la opensqt telegram_bot

# 添加执行权限
chmod +x opensqt telegram_bot
```

**Q: 端口占用**
```bash
# 查看端口占用
netstat -tlnp | grep :端口号

# 杀死占用进程
sudo kill -9 PID
```

### 9. 性能优化

#### 系统优化
```bash
# 增加文件描述符限制
echo "* soft nofile 65536" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 65536" | sudo tee -a /etc/security/limits.conf

# 优化网络参数
echo "net.core.rmem_max = 16777216" | sudo tee -a /etc/sysctl.conf
echo "net.core.wmem_max = 16777216" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

#### Go 程序优化
```bash
# 设置 Go 环境变量
export GOGC=100
export GOMAXPROCS=2
```

---

## 📞 技术支持

如果在部署过程中遇到问题：

1. 检查系统日志：`sudo journalctl -xe`
2. 检查程序日志：`tail -f opensqt.log`
3. 验证配置文件：确保 API 密钥正确
4. 测试网络连接：确保能访问交易所 API
5. 提交 GitHub Issue 并附上错误日志

**祝您部署成功！** 🎉