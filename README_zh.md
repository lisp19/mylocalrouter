# Agentic LLM Gateway

Agentic LLM Gateway 是一个轻量级、高性能的本地大模型网关，对外暴露标准 OpenAI API 协议。其核心职责是接收客户端请求，通过 HTTP 请求拉取远程的动态路由策略，并将请求无缝代理至后端的各大云端 Provider 或本地模型（Ollama/vLLM），最后将结果（支持流式 SSE）返回给客户端。

## 核心特性
1. **统一接入**：100% 兼容 OpenAI Chat Completions 协议。
2. **动态路由**：基于远程 HTTP 接口返回的 JSON 配置进行毫秒级流量分发。
3. **多源异构**：内置适配非 OpenAI 协议的 Provider（如 Google Gemini, Anthropic）。
4. **极简部署**：无数据库依赖，纯本地 YAML 配置 + 内存运行时。

## 构建与运行
依赖 Go 1.24 环境
```bash
make build
./bin/agentic-llm-gateway

# 测试 Mock 服务器
make build-mock
./bin/agentic-llm-gateway-mock
```
使用 Docker：
```bash
docker-compose up -d
```

## 配置指南
系统首次启动如果找不到配置文件，会自动在系统配置目录（或 `LOCALROUTER_CONFIG_PATH` 指定的地址）生成基础 YAML 模板。

您可以参考仓库根目录下的 `config.example.yaml` 了解完整的本地代理与路由节点配置方法。
如果需要实现基于外部 HTTP 接口的远端自动策略分发，请参考 `strategy.example.json` 设计您的 JSON 返回结构。

## 开发说明
本项目采用标准的分支管理策略。`main` 分支保持稳定，所有日常功能的添加和修改均在 `dev` 分支上进行。

**致谢**：本项目在 [Google Antigravity](https://ai.google.dev/) 的辅助下完成开发。
