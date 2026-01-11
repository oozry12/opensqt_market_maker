# OpenSQT Trading Bot Dockerfile
FROM golang:1.21-alpine AS builder

# 安装必要的包
RUN apk add --no-cache git ca-certificates tzdata

# 设置工作目录
WORKDIR /app

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 编译二进制文件
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o opensqt .

# 最终镜像
FROM alpine:latest

# 安装必要的包
RUN apk --no-cache add ca-certificates tzdata

# 创建非root用户
RUN adduser -D -s /bin/sh opensqt

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/opensqt .

# 复制配置文件模板
COPY config.yaml .
COPY .env.example .

# 设置权限
RUN chown -R opensqt:opensqt /app
USER opensqt

# 暴露端口（如果需要）
# EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD pgrep opensqt || exit 1

# 启动命令
ENTRYPOINT ["./opensqt"]
CMD ["config.yaml"]