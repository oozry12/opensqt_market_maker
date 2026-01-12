# 故障排除指南

## 常见问题

### 1. Telegram Bot 冲突错误

**错误信息**：
```
Conflict: terminated by other getUpdates request; make sure that only one bot instance is running
```

**原因**：有多个 Telegram Bot 实例同时运行，导致冲突。

**解决方法**：

#### 方法一：使用停止脚本
```bash
./stop_bot.sh
```

#### 方法二：手动停止
```bash
# 查找所有 telegram_bot 进程
ps aux | grep telegram_bot

# 停止所有实例
pkill -f telegram_bot

# 如果还有残留，强制终止
pkill -9 -f telegram_bot
```

#### 方法三：重启服务器
```bash
sudo reboot
```

### 2. 启动后立即退出

**检查步骤**：

1. **查看日志**：
   ```bash
   cat telegram_bot.log
   ```

2. **检查配置文件**：
   ```bash
   # 确保 .env 文件存在且配置正确
   cat .env
   
   # 确保 config.yaml 文件存在
   ls -la config.yaml
   ```

3. **检查权限**：
   ```bash
   chmod +x telegram_bot opensqt
   ```

### 3. 无法下载最新版本

**错误信息**：
```
Failed to download...
```

**解决方法**：

1. **检查网络连接**：
   ```bash
   ping github.com
   ```

2. **手动下载**：
   ```bash
   # 在浏览器中访问
   https://github.com/dennisyang1986/opensqt_market_maker/releases/latest
   
   # 然后上传到服务器
   ```

3. **使用代理**（如果需要）：
   ```bash
   export https_proxy=http://your-proxy:port
   ./quick_deploy.sh
   ```

### 4. 交易程序启动失败

**检查步骤**：

1. **查看 Telegram Bot 日志**：
   ```bash
   tail -f telegram_bot.log
   ```

2. **检查 API 密钥**：
   ```bash
   # 确保 .env 文件中的 API 密钥正确
   cat .env | grep API_KEY
   ```

3. **测试网络连接**：
   ```bash
   # 测试交易所 API
   curl https://api.binance.com/api/v3/ping
   ```

### 5. 权限不足

**错误信息**：
```
Permission denied
```

**解决方法**：
```bash
# 添加执行权限
chmod +x opensqt telegram_bot start_bot.sh stop_bot.sh quick_deploy.sh

# 如果需要 sudo 权限
sudo chown -R $USER:$USER .
```

### 6. 端口被占用

**检查端口占用**：
```bash
# 查看进程
ps aux | grep opensqt

# 停止进程
pkill opensqt
```

## 日志查看

### 实时查看日志
```bash
# Telegram Bot 日志
tail -f telegram_bot.log

# 交易程序日志（如果有）
tail -f opensqt.log
```

### 查看历史日志
```bash
# 查看最近100行
tail -n 100 telegram_bot.log

# 搜索错误
grep -i error telegram_bot.log
grep -i failed telegram_bot.log
```

## 完全重置

如果遇到无法解决的问题，可以完全重置：

```bash
# 1. 停止所有进程
pkill -9 -f telegram_bot
pkill -9 -f opensqt

# 2. 备份配置
cp .env .env.backup
cp config.yaml config.yaml.backup

# 3. 重新部署
./quick_deploy.sh

# 4. 恢复配置
cp .env.backup .env
cp config.yaml.backup config.yaml

# 5. 启动服务
./start_bot.sh
```

## 获取帮助

如果以上方法都无法解决问题：

1. **查看完整日志**：
   ```bash
   cat telegram_bot.log
   ```

2. **提交 Issue**：
   - 访问：https://github.com/dennisyang1986/opensqt_market_maker/issues
   - 附上错误日志和系统信息

3. **系统信息**：
   ```bash
   # 查看系统信息
   uname -a
   cat /etc/os-release
   
   # 查看架构
   uname -m
   ```
