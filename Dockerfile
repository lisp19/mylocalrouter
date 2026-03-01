# Build Stage
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o agentic-llm-gateway ./cmd/server/main.go

# Run Stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/agentic-llm-gateway .
VOLUME ["/app/config"]
ENV LOCALROUTER_CONFIG_PATH=/app/config/config.yaml
EXPOSE 8080
ENTRYPOINT ["./agentic-llm-gateway"]
