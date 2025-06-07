FROM golang:1.23-alpine

WORKDIR /app

COPY . .

RUN go mod tidy
RUN go build -o app main.go

EXPOSE 8080

CMD ["./app"]
