FROM golang:1.23-alpine

WORKDIR /app

COPY . .

# 使用国内代理解决 go mod tidy 网络超时问题
ENV GOPROXY=https://goproxy.cn,direct

RUN go mod tidy
RUN go build -o app main.go

EXPOSE 8000

CMD ["./app"]
