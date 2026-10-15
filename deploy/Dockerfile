# anonfile 多阶段构建镜像
# 用法：docker build -f deploy/Dockerfile -t anonfile:latest .
#
# 目录约定（容器内）：
#   /app/anonfile          后端二进制
#   /app/frontend/dist     Vue 构建产物
#   /app/config.json       站点配置（可挂载卷持久化）
#   /app/data/             文件与元数据（挂载卷持久化）

# --- 前端构建 ---
FROM node:20-alpine AS fe
WORKDIR /fe
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# --- 后端构建 ---
FROM golang:1.26-alpine AS be
WORKDIR /be
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o anonfile .

# --- 运行镜像 ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=be /be/anonfile ./
COPY --from=fe /fe/dist ./frontend/dist
COPY deploy/config.json.sample ./config.json
ENV TZ=Asia/Shanghai
EXPOSE 8080
VOLUME ["/app/data"]
HEALTHCHECK --interval=30s --timeout=3s --retries=3 --start-period=5s \
  CMD wget -qO- http://127.0.0.1:8080/api/info >/dev/null 2>&1 || exit 1
ENTRYPOINT ["./anonfile"]
