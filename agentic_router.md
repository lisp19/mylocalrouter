# mylocalrouter vNext: 智能生成式路由 / Generative Smart Routing PRD & Design Doc

**[ZH]** 本文档旨在为 mylocalrouter 的下一代“智能生成式路由”特性提供完整的需求定义、架构设计、技术实现细节以及任务拆分。
**[EN]** This document aims to provide complete requirements, architectural design, technical details, and task breakdowns for the next-generation "Generative Smart Routing" feature of mylocalrouter.

## 1. 需求设计说明 (PRD) / Product Requirements Document

### 1.1 核心痛点与目标 / Core Pain Points & Objectives
**[ZH]** 当前基于静态规则的路由无法应对对话的多样性，导致大量简单请求（如寒暄）消耗昂贵的远程大模型 Token。
**目标**：引入基于本地小模型（如 Qwen-0.5B）并发“意图判别算子”，生成“意图向量”进行智能分流：极简请求本地消化，复杂请求远程处理，实现降本增效。

**[EN]** Current static rule-based routing cannot handle conversational diversity, causing many simple requests (e.g., greetings) to consume expensive remote LLM tokens.
**Objective**: Introduce concurrent "intent evaluators" based on local small models (e.g., Qwen-0.5B) to generate "intent vectors" for smart diversion: digest simple requests locally, and process complex ones remotely to reduce costs and increase efficiency.

### 1.2 核心特性描述 / Core Features Description
**[ZH]** 
- **多维意图生成**：并发分发给算子（上下文相关度、复杂度、长度），组合成意图向量。
- **平滑概率判决 (Logprobs)**：支持通过 `llm_logprob_api` 算子直接获取 Token 的对数概率并计算 Softmax 浮点分（0.0~1.0），实现长尾模糊路由。
- **抗上下文坍缩**：配置携带 `N-1` 轮历史对话，避免误判。
- **平滑降级**：严格超时控制，算子失败则静默降级至默认路由。
- **独立调试**：提供 `eval-cli` 工具，脱机调试算子的 prompt 与 `logit_bias`。

**[EN]**
- **Multi-dimensional Intent**: Concurrently distribute to evaluators (context relevance, complexity, length) to form intent vectors.
- **Smooth Probability Evaluation (Logprobs)**: Support `llm_logprob_api` evaluators to fetch log probabilities of Tokens and calculate Softmax float scores (0.0~1.0) for long-tail fuzzy routing.
- **Context-Aware**: Carry `N-1` rounds of history to avoid misjudgments.
- **Graceful Degradation**: Strict timeout control; falls back silently to default routing upon evaluator failure.
- **Standalone Debugging**: Provide `eval-cli` tool for offline debugging of evaluator prompts and `logit_bias`.

## 2. 架构设计 (Architecture)

### 2.1 系统交互流 / System Flow
**[ZH]** 拦截请求 -> 并发评估阶段 -> 向量聚合 -> 策略引擎匹配 (如 `antonmedv/expr`) -> 代理转发
**[EN]** Intercept Request -> Concurrent Evaluation Phase -> Vector Aggregation -> Strategy Engine Matching (e.g., `antonmedv/expr`) -> Proxy Forward

## 3. 详细技术方案 (Technical Design)

### 3.1 配置文件设计结构 / Configuration Structure
**[ZH]** / **[EN]**
```yaml
generative_routing:
  enabled: true
  global_timeout_ms: 100
  fallback_provider: "openai-remote"
  
  evaluators:
    # 算子 1: 硬分类器 / Evaluator 1: Hard Classifier
    - name: "complexity"
      type: "llm_api"
      endpoint: "http://localhost:11434/v1/chat/completions"
      model: "qwen-0.5b"
      history_rounds: 1
      timeout_ms: 60
      logit_bias: { "15": 100, "16": 100 } # 强迫二值 / Force binary
      prompt_template: "只允许输出0或1。{{.Current}}"
      
    # 算子 2: 概率平滑算子 / Evaluator 2: Probability Smooth Evaluator
    - name: "prob_complexity"
      type: "llm_logprob_api" # 【新增】通过 Top Logprobs 获取浮点分数 / [NEW] Fetch float scores via Top Logprobs
      endpoint: "http://localhost:11434/v1/chat/completions"
      model: "qwen-0.5b"
      logit_bias: { "15": 100, "16": 100 }
      
  resolution_strategy:
    type: "dynamic_expression"
    rules:
      - condition: "prob_complexity > 0.6 && complexity == 1"
        target_provider: "claude-3-5-sonnet"
    default_provider: "openai-remote"
```

### 3.4 策略引擎 (Strategy Engine & Vector Resolution)
**[ZH]** 策略解析将预编译字符串表达式 (AST) 使得在请求期间性能极高。支持 fail-safe，当意图向量缺失某个 key 时表达式能平滑落入 default_provider。
**[EN]** Strategy resolution precompiles string expressions (AST) for extreme performance during requests. Supports fail-safe: when the intent vector misses a key, expressions gracefully fall through to the default_provider.

## 4. 调试工具 (Standalone CLI)
**[ZH]** `go run cmd/eval-cli/main.go --config config.yaml --evaluator complexity --input mock_chat.json` 用于单步验证。
**[EN]** `go run cmd/eval-cli/main.go --config config.yaml --evaluator complexity --input mock_chat.json` used for step-by-step validation.

## 5. 里程碑划分 (Milestones)
**[ZH]** / **[EN]**
- Stage 1: Data & Config / 基础结构与配置
- Stage 2: Evaluator Implementations / 算子实现 (`builtin`, `llm_api`, `llm_logprob_api`)
- Stage 3: Standalone CLI / 独立调试 CLI
- Stage 4: Engine Integration / 路由引擎集成
- Stage 5: Strategy Expression / 策略解析与表达式
- Stage 6: Bilingual Documentation / 双语文档完善