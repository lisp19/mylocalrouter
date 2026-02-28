# LocalRouter
LocalRouter is a lightweight, high-performance local LLM gateway proxy that strictly adheres to standard OpenAI Chat Completions API protocol.

Its core responsibility is receiving client requests, pulling remote JSON-based streaming route strategies via HTTP, and proxying requests to upstream endpoints seamlessly (supporting Google Gemini, Anthropic Claude, Cloud OpenAI compatible services, or local vLLM).

## Features
- **100% Compatible**: Serves standard OpenAI protocol
- **Dynamic Routing**: Fetch strategy JSON dynamically
- **Multi-Source Heterogeneous**: Auto-maps Gemini/Anthropic models to OpenAI APIs.
- **Easy Deploy**: Pure Go application, zero-dependencies. Single binary via docker.

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
