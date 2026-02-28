LocalRouter: 本地大模型统一反向代理网关设计文档
1. 架构概述
LocalRouter 是一个轻量级、高性能的本地大模型网关，对外暴露标准 OpenAI API 协议。其核心职责是接收客户端请求，通过 HTTP 请求拉取远程的动态路由策略，并将请求无缝代理至后端的各大云端 Provider 或本地模型（Ollama/vLLM），最后将结果（支持流式 SSE）返回给客户端。

核心特性
统一接入：100% 兼容 OpenAI Chat Completions 协议。

动态路由：基于远程 HTTP 接口返回的 JSON 配置进行毫秒级流量分发（本质上为标准的 HTTP GET 请求解析，默认以 Cloudflare Worker 举例，但完全解耦，支持任意 HTTP 方案）。

多源异构：内置适配非 OpenAI 协议的 Provider（如 Google Gemini, Anthropic）。

极简部署：无数据库依赖，纯本地 YAML 配置 + 内存运行时。

2. 目录结构设计 (Standard Go Layout)
良好的目录结构是代码生成的基础，建议按照以下结构组织项目：

Plaintext
localrouter/
├── cmd/
│   ├── server/           # 主程序入口
│   │   └── main.go
│   └── mock/             # Mock 模式入口
│       └── main.go
├── internal/
│   ├── config/           # 配置管理 (本地 YAML + 远程策略 URL 轮询)
│   ├── server/           # HTTP Server 与中间件 (基于 Go 1.24 增强的路由)
│   ├── router/           # 策略分流引擎
│   ├── providers/        # 各大厂商的接入实现
│   │   ├── openai/       # 包含纯 OpenAI 协议的厂商 (Deepseek, xAI, Qwen 等)
│   │   ├── anthropic/
│   │   ├── google/
│   │   └── local/        # Ollama / vLLM 适配
│   └── models/           # 统一的数据结构定义 (OpenAI 协议相关 struct)
├── pkg/
│   └── httputil/         # HTTP 客户端封装、SSE 流式解析工具
├── scripts/              # 构建与部署脚本
├── tests/                # 集成测试与单元测试
├── Dockerfile
├── docker-compose.yml
├── Makefile              # 常用命令集合
├── README.md             # 英文文档
└── README_zh.md          # 中文文档
3. 核心模块与接口定义
3.1 统一数据结构 (internal/models/openai.go)
所有的内外交互都应首先映射为标准的 OpenAI 数据结构。

Go
// 核心请求结构
type ChatCompletionRequest struct {
    Model    string    `json:"model"`
    Messages []Message `json:"messages"`
    Stream   bool      `json:"stream,omitempty"`
    // 其他基础字段 (Temperature, MaxTokens 等)
}

// 核心返回结构
type ChatCompletionResponse struct { ... }
type ChatCompletionStreamResponse struct { ... }
3.2 Provider 接口 (internal/providers/provider.go)
所有外部大模型服务必须实现此接口。对于支持 OpenAI 协议的厂商（DeepSeek, Minimax, Doubao, ZAI, xAI, OpenRouter 等），直接复用通用的 OpenAICompatibleProvider；对于 Google 和 Anthropic，则编写专门的协议转换层。

Go
type Provider interface {
    // 获取当前 Provider 的标识
    Name() string
    // 非流式请求
    ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error)
    // 流式请求，通过 channel 推送数据片段
    ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest, streamChan chan<- *models.ChatCompletionStreamResponse) error
}
3.3 路由策略引擎 (internal/router/engine.go)
负责解析从远程 URL 获取的 JSON 配置，并决定请求去向。

Go
type StrategyEngine interface {
    // 根据请求和远程配置，返回目标 Provider 和实际需要请求的 Model 名称
    SelectProvider(req *models.ChatCompletionRequest, remoteCfg *config.RemoteStrategy) (Provider, string, error)
}
自定义算子扩展方案：基础实现可通过纯 Go 逻辑判断。为满足动态脚本需求，建议引入 github.com/antonmedv/expr，极其轻量且执行速度极快，适合在网关层做规则求值。

4. 配置管理体系
系统依赖两套配置：本地静态配置（密钥、端口等）和远程动态配置（分流策略）。

4.1 本地配置文件管理 (internal/config/local.go)
加载逻辑：

读取环境变量 LOCALROUTER_CONFIG_PATH。

若为空，默认查找应用专有配置目录（如 Linux/Mac 下的 $HOME/.config/localrouter/config.yaml）。

若文件不存在，程序自动在目标路径创建包含基础结构的默认模板文件。

配置结构：

YAML
server:
  port: 8080
  host: "127.0.0.1"
remote_strategy:
  url: "https://your-config-domain.com/strategy.json" # 任意返回标准 JSON 的 HTTP GET 接口
  poll_interval: 60s # 缓存在内存中，定期拉取
providers:
  openai:
    api_key: "sk-..."
  anthropic:
    api_key: "sk-ant-..."
  google:
    api_key: "AIza..."
  deepseek:
    api_key: "sk-..."
    base_url: "https://api.deepseek.com/v1"
  local_vllm:
    base_url: "http://192.168.1.100:8000/v1"
4.2 远程策略解析 (internal/config/remote.go)
将对远程策略的依赖抽象为一个标准的 HTTP GET 请求。后台运行一个 Goroutine，按照 poll_interval 指定的间隔使用类似 curl <url> 的逻辑请求配置接口，解析返回的 JSON，并将其存入内存级原子变量（atomic.Value 以保证并发安全和极低延迟）。

选型说明：配置源只需能够返回符合格式的 JSON Body 即可。虽然默认架构图和示例中推荐使用 Cloudflare Worker 作为 Serverless 方案，但这并非强绑定。完全可以使用 Nginx 托管静态 JSON 文件、AWS Lambda、自建后端 API 甚至 GitHub Gist 作为配置源。

数据结构：

Go
type RemoteStrategy struct {
    Strategy    string `json:"strategy"`     // "local" or "remote"
    LocalModel  string `json:"local_model"`  // e.g., "qwen-35b-awq"
    RemoteModel string `json:"remote_model"` // e.g., "gemini-3-flash"
    UpdatedAt   string `json:"updated_at"`
}
5. Mock 与测试方案
5.1 Mock 启动入口 (cmd/mock/main.go)
独立入口，用于验证部署及网络连通性。

启动时拦截所有 Provider 调用，注入 MockProvider。

MockProvider 在内存中生成模拟响应，并通过 time.Sleep 模拟真实的流式 Token 吐出效果。

5.2 单元测试规范
路由与配置测试：针对 router 的规则匹配逻辑、HTTP GET 拉取策略的回退机制编写表驱动测试。

并发数据安全：使用 sync.WaitGroup 发起大并发量请求，结合 go test -race 确保 atomic.Value 读写的绝对安全。

6. 构建与部署方案
6.1 构建脚本与 Makefile
Makefile
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/localrouter ./cmd/server/main.go

build-mock:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/localrouter-mock ./cmd/mock/main.go

test:
	go test -v -race ./...
6.2 Docker 化部署 (Dockerfile)
采用基于 Alpine 或 Scratch 的多阶段构建，保持镜像极致轻量：

Dockerfile
# Build Stage
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o localrouter ./cmd/server/main.go

# Run Stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/localrouter .
VOLUME ["/app/config"]
ENV LOCALROUTER_CONFIG_PATH=/app/config/config.yaml
EXPOSE 8080
ENTRYPOINT ["./localrouter"]
配合 docker-compose.yml，可以将宿主机目录直接映射至 /app/config，实现配置热加载与持久化。

7. AI 辅助编程 (Vibe Coding) 实施建议
可按以下阶段将指令和本文档提交给 AI：

Phase 0 - 仓库创建：创建git仓库并初始化，后续的每一步实现需要进行本地git提交。

Phase 1 - 初始化与配置抽象：使用 Go 1.24 创建脚手架。实现 YAML 本地解析，并编写一个通用的 HTTP GET 客户端，利用 atomic.Value 实现对配置 URL 的定时轮询与 JSON 解析。

Phase 2 - 通用模型接入：定义 OpenAI 请求体，实现 OpenAICompatibleProvider 支持自定义 BaseURL、流式/非流式请求。

Phase 3 - 特殊协议转换：为 Google、Anthropic 等非标协议编写专属的 Provider 适配器。

Phase 4 - 网关逻辑与表达式引擎：使用 net/http 原生路由搭建服务器，结合 github.com/antonmedv/expr 将 HTTP 请求与内存中的 JSON 策略进行匹配分发。

Phase 5 - 模拟与完善：实现 cmd/mock 逻辑，编写测试用例，补全 Docker 和 README 文件。
Phase 6 - 整理：按照开源项目的标准review和整理项目代码。并为用户输出提交到远程仓库的步骤。（步骤不需要体现在代码中）
