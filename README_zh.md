# LocalRouter

LocalRouter 是一个轻量级、高性能的本地大模型网关，对外暴露标准 OpenAI API 协议。其核心职责是接收客户端请求，通过 HTTP 请求拉取远程的动态路由策略，并将请求无缝代理至后端的各大云端 Provider 或本地模型（Ollama/vLLM），最后将结果（支持流式 SSE）返回给客户端。

## 核心特性
1. **统一接入**：100% 兼容 OpenAI Chat Completions 协议。
2. **动态路由**：基于远程 HTTP 接口返回的 JSON 配置进行毫秒级流量分发。
3. **多源异构**：内置适配非 OpenAI 协议的 Provider（如 Google Gemini, Anthropic）。
4. **极简部署**：无数据库依赖，纯本地 YAML 配置 + 内存运行时。

## 构建与运行
依赖 Go 1.24 环境
```bash
make build
./bin/localrouter

# 测试 Mock 服务器
make build-mock
./bin/localrouter-mock
```
使用 Docker：
```bash
docker-compose up -d
```

## 配置指南
系统首次启动如果找不到配置文件，会自动在系统配置目录（或 `LOCALROUTER_CONFIG_PATH` 指定的地址）生成基础 YAML 模板。

通过修改配置能够将策略路由交由远端网端托管查询。
