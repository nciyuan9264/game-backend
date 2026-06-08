# ---------- 构建阶段 ----------
FROM golang:1.23-alpine AS builder

WORKDIR /app

ARG GAME=acquire

# 只拷贝依赖文件（利用缓存）
COPY go.mod go.sum ./

ENV GOPROXY=https://goproxy.cn,direct

RUN go mod download

# 再拷贝源码
COPY . .

# 构建静态二进制（极小）
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o app ./cmd/${GAME}


# ---------- 运行阶段 ----------
FROM alpine:3.20

WORKDIR /app

# 只复制编译结果
COPY --from=builder /app/app .

EXPOSE 8000

CMD ["./app"]
