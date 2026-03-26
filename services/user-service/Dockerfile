FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY main.go .
RUN go mod init user-service && \
    go mod tidy && \
    CGO_ENABLED=0 go build -o user-service .

FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/user-service .

EXPOSE 8081

CMD ["./user-service"]
