FROM golang:1.24-alpine AS builder

WORKDIR /app

ENV GOTOOLCHAIN=auto

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o gateway ./cmd/gateway

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/gateway .
COPY --from=builder /app/configs ./configs

EXPOSE 8080

CMD ["./gateway"]
