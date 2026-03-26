FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY main.go .
RUN go mod init product-service && \
    go mod tidy && \
    CGO_ENABLED=0 go build -o product-service .

FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/product-service .

EXPOSE 8082

CMD ["./product-service"]
