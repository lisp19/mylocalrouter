# 智能化生成式路由算子配置与调试指南

本文档专门用于解释 `evaluator_config.yaml` 中关于意图判别算子（Evaluator）的本地配置方法，特别是大模型高级调优参数 `logit_bias` 的运作原理与实战用法。

## 1. 为什么需要 Logit Bias (逻辑回归偏置)？

在我们的路由架构中，网关需要极高的速度和绝对稳定的输出格式（例如：**严格返回 `0` 或 `1`**），以便下游的策略引擎（Rules Engine）通过表达式（如 `complexity == 0`）进行数学判断。

尽管我们在 Prompt 中加上了“只允许输出0或1。不要有任何多余的字符”，但 LLM 的自回归生成特性意味着：
1. 它可能会输出 `"0"`
2. 它可能会输出 `" 0"` (带空格)
3. 它可能会输出 `"我认为是0"`
4. 开源小模型（如 0.5B）指令遵循能力较弱，经常“不听话”乱输出文字，导致整个路由网关解析 JSON 失败降级。

**解决方案：Logit Bias。**
Logit Bias 允许我们在模型生成（采样）前，直接从底层干预分词库 (Tokenizer) 中某个字词的生成概率。当我们将某个词的 Bias 设置为极端值（如 `100` 或 `+10`，取决于具体框架实现，OpenAI 协议上限是 100）时，相当于**强制**模型只能从这两个词里选一个作为回答，直接在物理层面封死了模型“废话”的可能。

---

## 2. 如何配置 Logit Bias？

在 YAML 的 `logit_bias` 字典中：
- **Key 是 Token ID（字符串格式）**：这不是字面上的 "0" 字母，而是对应的分词 ID。
- **Value 是偏置值**：设置为 100 意味着极度增加该词出现的概率。

### 🚨 绝对核心痛点：不同模型的 Tokenizer 字典不一样！

你不能直接无脑复制配置！如果你用的模型是 Qwen，字典里的 "0" 可能 ID 是 `15`；如果你用的是 Llama 3，字典里的 "0" ID 可能就是 `16`。如果填错了 ID，模型不仅不会按照你的预期强制输出数字，还会强制输出毫不相干的乱码（比如填错了 ID 可能强制输出了一个感叹号）。

### 找到对应 Token ID 的方法

最简单的方法是使用开源的 Python `tiktoken` 库，或者直接写一个短脚本通过该模型的官方 Tokenizer 查：

例如，如果您使用的是 Qwen 模型，可以编写一个非常简单的 Python 脚本：
```python
from transformers import AutoTokenizer

# 加载你作为算子使用的那个本地小模型
tokenizer = AutoTokenizer.from_pretrained("Qwen/Qwen2.5-0.5B-Instruct")

# 查找 "0" 和 "1" 在这套模型里的真实物理 ID
tokens_0 = tokenizer.encode("0")
tokens_1 = tokenizer.encode("1")

print(f"'0' 的 Token ID 是: {tokens_0}")
print(f"'1' 的 Token ID 是: {tokens_1}")
```

假设上述脚本打印出 `'0'` 的 ID 是 `15`，`'1'` 的 ID 是 `16`，相应的配置应当为：
```yaml
      logit_bias: 
        "15": 100
        "16": 100
```
*(注意：YAML 中要求 JSON 的 Key 必须是字符串，务必要给数字加引号)*

---

## 3. 使用 `eval-cli` 工具进行单步调优验证

我们为您提供了一个独立于主流水线的调试工具。在你真正把网关部署并切流之前，**必须**使用该工具验证你的 Prompt 和 Logit Bias 是否奏效。

1. **准备模拟数据** `mock_chat.json`:
   ```json
   {
     "messages": [
       {"role": "user", "content": "你好，在吗？"}
     ]
   }
   ```

2. **本地启动你的小模型后端**（例如以 Ollama 启动）：
   ```bash
   ollama run qwen2.5:0.5b
   ```

3. **运行独立调试工具**：
   ```bash
   go run cmd/eval-cli/main.go --config example/evaluator_config.yaml --evaluator complexity --input mock_chat.json
   ```

4. **观察输出**：
   如果配置完全正确，并且模型遵循了 Logit Bias 的限制，你应该看到它在 50~150ms 内稳定返回数字 `0` 并被成功解析。
   ```text
   === Evaluation Result ===
   Evaluator Dimension: complexity
   Score:               0
   Time Taken (TTFT):   78.4ms
   ```

## 4. 获取平滑的概率值 (0.0 ~ 1.0)
如果您希望模型不仅输出 `0` 或 `1`，而是希望得到类似 `0.85` 的概率平滑值（例如：0.85 意味着模型认为该问题有 85% 的概率是复杂任务），您可以使用 **`llm_logprob_api`** 这一高级算子类型。

**配置方法：**
在 `config.yaml` 中，将算子的 `type` 修改为 `llm_logprob_api`，同时**保留 `logit_bias`** 设置。

```yaml
    - name: "prob_complexity"
      type: "llm_logprob_api"    # 高级类型
      endpoint: "http://localhost:11434/v1/chat/completions"
      model: "qwen2.5:0.5b"
      logit_bias: 
        "15": 100
        "16": 100
```

**工作原理：**
该选项依然在底层强制模型只能选择 `0` 或 `1`，但系统不会直接返回该硬分类结果，而是通过 OpenAI 标准协议中的 `top_logprobs` 取回模型在生成这一个 Token 时，备选词汇表里 `0` 和 `1` 的原始对数概率。然后使用 Softmax 公式将其转换为精确的长尾浮点分数。
借此，您可以在 `resolution_strategy` 里写出更加柔性的表达式：
*例如：* `- condition: "prob_complexity > 0.6"`

## 5. 其它注意事项
* **Timeout 设置原则**：模型越大，出首字（TTFT）越慢。建议算子使用的模型参数量控制在 1.5B 以下，并且在 `config.yaml` 中严格设置 `timeout_ms: 60` 或 `100`。超时后网关会自动执行降级，跳过拦截直接去远端，**确保核心服务不断流**。
