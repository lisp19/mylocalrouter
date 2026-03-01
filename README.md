# LocalRouter
LocalRouter is a lightweight, high-performance local LLM gateway proxy that strictly adheres to standard OpenAI Chat Completions API protocol.

Its core responsibility is receiving client requests, pulling remote JSON-based streaming route strategies via HTTP, and proxying requests to upstream endpoints seamlessly (supporting Google Gemini, Anthropic Claude, Cloud OpenAI compatible services, or local vLLM).

## Features
- **100% Compatible**: Serves standard OpenAI protocol
- **Dynamic Routing**: Fetch strategy JSON dynamically
- **Multi-Source Heterogeneous**: Auto-maps Gemini/Anthropic models to OpenAI APIs.
- **Easy Deploy**: Pure Go application, zero-dependencies. Single binary via docker.
- **Generative Smart Routing (Experimental / 实验性功能)**: Dynamically route requests based on real-time intent classification using local small LLMs. (基于本地小模型实时意图识别的动态智能路由)

---
### 🧬 Experimental: Generative Smart Routing (智能化生成式路由)
**[EN]** LocalRouter now supports *Generative Smart Routing* (Experimental). By configuring multiple concurrent intent evaluators (e.g. complexity, context dependency), the gateway delegates simple queries to local small models and complex queries to remote large models. Define rules using dynamic expressions in `config.yaml`. To debug evaluators independently, use the `eval-cli` tool.

**[ZH]** LocalRouter 现已支持**智能化生成式路由**（实验性功能）。通过配置多个并发的意图判别算子（如：复杂度评估、上下文依赖评估），网关可将简单的自然语言请求拦截并路由至本地小参数模型，将复杂长文本路由至云端大模型。可在 `config.yaml` 中使用动态逻辑表达式定义路由条件。支持使用 `eval-cli` 工具进行算子独立调试。
---

## Build
```bash
make build
# or run with docker
docker-compose up -d
```

## Setup Configuration
By default, the server expects `LOCALROUTER_CONFIG_PATH` to point to a yaml file. 
If no configuration file exists, the server automatically generates a template at `~/.config/localrouter/config.yaml`.

Please refer to `config.example.yaml` in the repository root for a complete local configuration example.
To implement remote dynamic strategy distribution via HTTP, see `strategy.example.json` for the expected JSON return structure.

## Usage
Simply point your OpenAI client Base URL to `http://localhost:8080/v1` instead of `https://api.openai.com/v1`.

## Development
This project follows a standard branching strategy. The `main` branch is for stable releases, and all active development occurs on the `dev` branch.

**Acknowledgements**: This project is assisted by [Google Antigravity](https://ai.google.dev/).
