# mylocalrouter vNext: 智能生成式路由 (Generative Smart Routing) 需求与技术设计文档

本文档旨在为 mylocalrouter 的下一代“智能生成式路由”特性提供完整的需求定义、架构设计、技术实现细节以及为 Vibe Coding 准备的阶段性任务拆分。

## 1. 需求设计说明 (PRD)

### 1.1 核心痛点与目标
当前的基于静态规则（配置策略、简单表达式）的路由机制无法有效应对自然语言对话的多样性和复杂性，导致大量简单请求（如寒暄、短确认）依然消耗昂贵的远程大模型（如 Claude/Gemini）Token 和时间。

**目标**：引入基于本地小参数模型（如 Qwen3-0.6B）的多路并发“意图判别算子”，通过生成多维度的“意图向量”来进行智能路由分流，实现极简请求本地消化，复杂请求远程处理，达到极致的降本增效。

### 1.2 核心特性描述
- **多维意图生成与向量化匹配**：系统将客户端请求并发分发给多个意图判别算子（如：上下文相关度、任务复杂度、内容长度）。各算子输出标量（0或1，或0~1），组合成意图向量，由策略引擎决定路由去向。
- **抗上下文坍缩 (Context-Aware)**：判别算子在评估时，不仅接收最后一条消息，还可配置携带 `N-1` 轮历史对话，避免脱离语境的误判。
- **非阻塞与平滑降级 (Graceful Degradation)**：算子的执行必须是高并发、严格超时的。若任何算子超时或解析失败，系统静默降级，回退至默认的 Provider 路由逻辑，确保核心代理链路 100% 可用。
- **独立算子调试体系**：提供脱离主网关进程的脚本化工具，支持对单个算子的提示词、Logit Bias 和解析逻辑进行独立验证和调优。

## 2. 架构设计 (Architecture)

### 2.1 系统交互流
1. **拦截请求**：网关接收标准 OpenAI Chat Completions 请求。
2. **并发评估 (Evaluation Phase)**：基于配置，使用 `errgroup` 并行调用多个算子（内置算子或本地 LLM 算子）。
3. **向量聚合**：收集所有算子的返回值，生成意图向量（例：`{"complexity": 0, "context_rel": 1, "length": 0}`）。
4. **策略匹配 (Strategy Engine)**：将向量输入策略引擎，命中规则（如“全为0则使用本地模型”），决定最终的 target provider。
5. **代理转发**：将原始请求无损转发至目标 Provider 并流式返回响应。

### 2.2 部署与并发最佳实践 (Best Practices)
- **模型参数共享**：若多个判别算子调用同一物理设备上的同一基础小模型，需在底层推理引擎（如 Ollama）中开启共享，避免显存抖动。
- **KV Cache 与 Swap**：携带历史上下文并发请求会加重显存负担。部署本地算子时，强烈建议开启 vLLM 的 PagedAttention 及显存-内存 Swap 优化。

## 3. 详细技术方案 (Technical Design)

### 3.1 核心接口抽象 (Go Interfaces)
设计高度抽象的算子接口，确保主流程对算子内部实现“零感知”。

```go
package evaluator

import "context"

// EvaluationResult 存储单个维度的评分
type EvaluationResult struct {
    Dimension string
    Score     float64 // 0.0 ~ 1.0 或二值
}

// Evaluator 定义了所有意图判别算子必须实现的接口
type Evaluator interface {
    // Name 返回算子的唯一标识
    Name() string
    // Evaluate 执行判别逻辑。messages 为携带了 N-1 历史的上下文。
    Evaluate(ctx context.Context, messages []Message) (*EvaluationResult, error)
}
```

### 3.2 配置文件设计结构
无侵入式配置，在现有的 config.yaml 中增加路由控制块。

```yaml
generative_routing:
  enabled: true
  global_timeout_ms: 100 # 全局并发超时
  fallback_provider: "openai-remote" # 降级默认目标
  
  evaluators:
    - name: "complexity"
      type: "llm_api"
      endpoint: "http://localhost:11434/v1/chat/completions"
      model: "qwen-0.6b"
      history_rounds: 1 # 携带1轮历史
      timeout_ms: 60
      # 强迫输出二值，15和16代表特定tokenizer下的 0 和 1
      logit_bias: 
        "15": 100
        "16": 100
      prompt_template: "你是一个无情的二值分类器。判断最后一条消息是否是一个极其简单的日常寒暄... 只允许输出0或1。上下文：{{.History}} 当前：{{.Current}}"
      
    - name: "length_check"
      type: "builtin" # 内置规则算子，无需走网络
      threshold: 50
```

### 3.3 并发引擎伪代码 (Router Core)

```go
// EvaluateAll 并发执行所有配置的算子
func EvaluateAll(ctx context.Context, req *ChatRequest, evals []Evaluator) map[string]float64 {
    ctx, cancel := context.WithTimeout(ctx, globalTimeout)
    defer cancel()

    var g errgroup.Group
    var mu sync.Mutex
    results := make(map[string]float64)

    for _, ev := range evals {
        ev := ev // capture variable
        g.Go(func() error {
            // 截取需要的 N-1 轮上下文
            slicedMsgs := buildContext(req.Messages, ev.HistoryRounds())
            res, err := ev.Evaluate(ctx, slicedMsgs)
            if err != nil {
                // 记录日志，但不阻断整体（容错降级）
                log.Warnf("Evaluator %s failed: %v", ev.Name(), err)
                return nil 
            }
            mu.Lock()
            results[ev.Name()] = res.Score
            mu.Unlock()
            return nil
        })
    }
    g.Wait() // 忽略内部被吞掉的error，保证尽力而为返回
    return results
}
```

### 3.4 策略引擎与意图向量解析 (Strategy Engine & Vector Resolution)
在并发算子全部返回结果（或因超时返回部分结果）后，系统会生成一个多维意图向量（在 Go 中表现为 `map[string]float64`）。
策略引擎（Strategy Engine）负责接收该向量，并输出目标 Provider 的名称。

#### 3.4.1 核心接口设计
策略解析必须高度抽象，主流程仅关心输入向量和输出 Provider 字符串。

```go
package strategy

// Resolver 定义了意图向量解析策略的通用接口
type Resolver interface {
    // Name 策略的唯一标识名
    Name() string
    // Resolve 输入意图向量，返回决定使用的 Provider Name。若无法决断，返回空字符串交由 Fallback 处理。
    Resolve(vector map[string]float64) string
}
```

#### 3.4.2 硬编码的内置解析策略 (Built-in Strategies)
为了保证系统开箱即用，代码库中将预置几个硬编码的经典策略实现：

- `strict_local_first` (严格本地优先策略):
  - 逻辑： 仅当向量中 `complexity == 0.0` 且 `context_rel == 0.0`（既不复杂也不强依赖上下文），且 `length < threshold` 时，才返回 `local_provider`；只要有任何一个维度不满足或缺失，立刻返回 `remote_provider`。

- `weighted_scoring` (加权阈值打分策略):
  - 逻辑： 为各个维度配置权重（如：复杂度权重 0.6，上下文权重 0.4）。计算加权总分 `Score = sum(Weight_i * Value_i)`，若 `Score > Threshold`，则认为任务偏难，路由至远端。

#### 3.4.3 动态配置化表达式解析 (Dynamic Expression Routing)
这是策略解析的杀手锏功能。用户可以在 config.yaml 中使用类似 if 逻辑表达式的语法定义路由规则。

工程实现建议：在 Go 中，不建议从零手写 AST 解析器，推荐引入轻量且安全的表达式评估库（如 `github.com/antonmedv/expr` 或 `Knetic/govaluate`）。它们能将字符串形态的逻辑表达式安全地编译为可执行的字节码或 AST，性能极高，不会对代理转发产生肉眼可见的延迟。

配置化结构设计 (config.yaml 补充)：

```yaml
generative_routing:
  # ... (前面的 evaluators 配置) ...
  
  resolution_strategy:
    type: "dynamic_expression" # 可选: strict_local_first, weighted_scoring, dynamic_expression
    
    # 动态表达式规则列表（自上而下匹配，命中即返回）
    rules:
      - condition: "complexity == 0 && length_check < 50"
        target_provider: "local-qwen-0.5b"
      - condition: "complexity == 1 || context_rel == 1"
        target_provider: "claude-3-5-sonnet"
    
    # 所有规则均未命中，或解析发生错误时的最终保底路由
    default_provider: "openai-remote"
```

#### 3.4.4 策略引擎分发伪代码

```go
// StrategyEngine 管理策略的加载与执行
type Engine struct {
    resolver Resolver
    fallback string
}

func (e *Engine) Route(vector map[string]float64) string {
    if e.resolver == nil {
        return e.fallback
    }
    
    // 调用选定的解析器（内置或动态表达式解析器）进行判决
    target := e.resolver.Resolve(vector)
    if target == "" {
        log.Warnf("Strategy resolution yielded no target, falling back to %s", e.fallback)
        return e.fallback
    }
    return target
}

// ExpressionResolver 动态表达式策略的具体实现
type ExpressionResolver struct {
    rules []CompiledRule // 预编译好的表达式 AST 列表，避免每次请求重复解析字符串
}

func (er *ExpressionResolver) Resolve(vector map[string]float64) string {
    // antonmedv/expr 等库支持直接传入 map 作为运行上下文 (env)
    for _, rule := range er.rules {
        matched, err := rule.Program.Run(vector)
        if err == nil && matched.(bool) {
            return rule.TargetProvider
        }
    }
    return ""
}
```

## 4. 算子插件独立调试工具链 (Standalone Tool)
为了在不启动完整网关的情况下调试 LLM 算子的 Prompt、Logit Bias 以及解析逻辑，需开发一个独立的 CLI 工具。

**设计方案**：
创建一个新的 entrypoint：`cmd/eval-cli/main.go`。

**使用方式**：
```bash
# 传入算子配置文件和模拟对话记录进行单步调试
go run cmd/eval-cli/main.go --config config.yaml --evaluator complexity --input mock_chat.json
```

**工具核心逻辑**：
1. 解析 YAML 中对应 evaluator 的配置块。
2. 读取 `mock_chat.json` 并根据 `history_rounds` 构建上下文。
3. 渲染 `prompt_template`。
4. 携带 `logit_bias` 发起 HTTP 请求到本地小模型。
5. 打印原始 Response 耗时 (TTFT) 及解析后的 Score，便于直接调整模型侧参数。

## 5. 任务拆分与 Vibe Coding Stage 划分
建议直接将以下 Stage 输入给 AI 编程助手进行逐步构建，确保每一步都有坚实的基础。

### Stage 1: 基础数据结构与配置解析 (Data & Config)
- **任务**：
  - 定义 `Evaluator` 接口、`EvaluationResult` 结构体。
  - 修改配置解析逻辑，使其能够读取 `config.yaml` 中新增的 `generative_routing` 和多态的 `evaluators` 数组。
- **验证**：能成功序列化和反序列化包含多种算子类型的 YAML。

### Stage 2: 核心算子实现 (Evaluator Implementations)
- **任务**：
  - 实现 `BuiltinLengthEvaluator`（基于字符/Token 长度）。
  - 实现 `LLMAPIEvaluator`，核心是构建标准的 OpenAI 请求，注入设定好的 Logit Bias，处理超时 context，并解析返回的单 Token 结果转为 Float64。
- **验证**：编写单元测试，Mock 一个 HTTP Server 验证 `LLMAPIEvaluator` 的解析逻辑。

### Stage 3: 独立调试 CLI 开发 (Standalone CLI)
- **任务**：在 `cmd/eval-cli` 下搭建脚手架，实现根据命令行参数加载特定 `Evaluator`，读取本地 JSON 文件并发起调用的逻辑。
- **验证**：使用本地真实部署的 Qwen-0.6B 模型，运行 CLI 验证 Prompt 命中率和 Logit Bias 强制输出的效果。

### Stage 4: 路由引擎与并发调度集成 (Engine Integration)
- **任务**：
  - 在主处理 Pipeline 中引入 `EvaluateAll` 并发逻辑。
  - 实现降级策略：一旦 `EvaluateAll` 返回的特征向量不完整，直接走 `fallback_provider`；若完整，则使用策略匹配逻辑选择目标。
- **验证**：主服务启动后，发送多并发测试请求，通过打点日志观察 Evaluator 阶段是否稳定在指定的 `global_timeout_ms` 内返回。

### Stage 5: 策略解析器引擎开发与配置化集成
- **任务**：
  - 引入抽象接口 `Resolver`，实现硬编码策略 `strict_local_first`。
  - 引入 `antonmedv/expr` 或同类表达式解析库，实现 `ExpressionResolver`。
  - 在初始化阶段（Init），遍历读取配置文件中的 rules，将条件字符串预编译为执行程序（Program），以保证请求期间的极致性能。
- **验证**：编写单元测试，构造多个 `map[string]float64` 意图向量，验证动态配置的逻辑表达式（包含 `&&, ||, <, >, ==` 等操作符）是否能被正确解析并输出预期的 Provider 名称。若遇到缺失变量（如某个算子超时未返回对应 key），验证表达式求值是否能平滑 fail-safe 并且落入下一条 rule 或 fallback。

## 6. 验证标准 (Acceptance Criteria)
- **功能正确性**：发送简单的“你好”，系统应能通过独立 CLI 验证输出 1（简单），并被主路由转发至本地 Provider；发送复杂长代码，输出 0，转发至远程 Provider。
- **超时容错性**：人为在本地 LLM API 服务上制造 500ms 延迟，网关应在设定的（如 100ms）超时后自动断开算子连接，无报错且无延迟感知地走 `fallback_provider` 继续提供服务。
- **并发与资源**：在 50 QPS 的并发下，mylocalrouter 进程自身的内存泄漏率应为 0，Goroutine 数量应保持稳定，不出现无限挂起的协程。
- **无侵入性**：原有纯配置的静态路由在不开启 `generative_routing` 时，表现行为、性能应与上个版本完全一致。