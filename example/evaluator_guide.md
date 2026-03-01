# 智能化生成式路由算子配置与调试指南 / Generative Smart Routing Evaluator Configuration & Debugging Guide

**[ZH]** 本文档专门用于解释 `evaluator_config.yaml` 中关于意图判别算子（Evaluator）的本地配置方法，特别是大模型高级调优参数 `logit_bias` 的运作原理与实战用法。
**[EN]** This document specifically explains the local configuration methods for intent evaluators in `evaluator_config.yaml`, especially the operational principles and practical usages of the advanced LLM tuning parameter `logit_bias`.

## 1. 为什么需要 Logit Bias (逻辑回归偏置)？ / Why is Logit Bias Needed?

**[ZH]**
在我们的路由架构中，网关需要极高的速度和绝对稳定的输出格式（例如：**严格返回 `0` 或 `1`**），以便下游的策略引擎（Rules Engine）通过表达式（如 `complexity == 0`）进行数学判断。
尽管我们在 Prompt 中加上了“只允许输出0或1。不要有任何多余的字符”，但 LLM 的自回归生成特性意味着：
1. 它可能会输出 `"0"`
2. 它可能会输出 `" 0"` (带空格)
3. 它可能会输出 `"我认为是0"`
4. 开源小模型（如 0.5B）指令遵循能力较弱，经常“不听话”乱输出文字，导致整个路由网关解析 JSON 失败降级。

**解决方案：Logit Bias。**
Logit Bias 允许我们在模型生成（采样）前，直接从底层干预分词库 (Tokenizer) 中某个字词的生成概率。当我们将某个词的 Bias 设置为极端值（如 `100` 或 `+10`，取决于具体框架实现，OpenAI 协议上限是 100）时，相当于**强制**模型只能从这两个词里选一个作为回答，直接在物理层面封死了模型“废话”的可能。

**[EN]**
In our routing architecture, the gateway demands extremely high speed and absolutely stable output formats (e.g., **strictly returning `0` or `1`**) so the downstream Strategy Engine can perform mathematical evaluations via expressions (e.g., `complexity == 0`).
Even if we add "Only output 0 or 1. Do not use extra characters" in the Prompt, the autoregressive generation nature of LLMs means:
1. It might output `"0"`
2. It might output `" 0"` (with a space)
3. It might output `"I think it is 0"`
4. Open-source small models (e.g., 0.5B) have weaker instruction-following capabilities and often output garbage text, causing the entire gateway's JSON parsing to fail and degrade.

**Solution: Logit Bias.**
Logit Bias allows us to directly intervene in the generation probability of a specific token from the Tokenizer at the lowest level before the model generates (samples) it. By setting the Bias of a token to an extreme value (like `100` or `+10`, depending on the framework, OpenAI protocol cap is 100), we are essentially **forcing** the model to only choose between those specific tokens, physically blocking the possibility of generating nonsense.

---

## 2. 如何配置 Logit Bias？ / How to Configure Logit Bias?

**[ZH]** 在 YAML 的 `logit_bias` 字典中：
- **Key 是 Token ID（字符串格式）**：这不是字面上的 "0" 字母，而是对应的分词 ID。
- **Value 是偏置值**：设置为 100 意味着极度增加该词出现的概率。

**[EN]** In the YAML `logit_bias` dictionary:
- **Key is Token ID (String format)**: This is not the literal letter "0", but the corresponding tokenizer ID.
- **Value is bias value**: Setting it to 100 means extremely increasing the probability of that word appearing.

### 🚨 绝对核心痛点：不同模型的 Tokenizer 字典不一样！ / Critical Pain Point: Different Models have Different Tokenizers!

**[ZH]** 你不能直接无脑复制配置！如果你用的模型是 Qwen，字典里的 "0" 可能 ID 是 `15`；如果你用的是 Llama 3，字典里的 "0" ID 可能就是 `16`。如果填错了 ID，模型不仅不会按照你的预期强制输出数字，还会强制输出毫不相干的乱码（比如填错了 ID 可能强制输出了一个感叹号）。

**[EN]** You cannot just copy and paste configurations mindlessly! If you use the Qwen model, the ID for "0" might be `15`; if you use Llama 3, the ID for "0" might be `16`. If you provide the wrong ID, not only will the model fail to force output numbers as expected, but it will also force output irrelevant gibberish (e.g., providing the wrong ID might force an exclamation mark).

### 找到对应 Token ID 的方法 / Method to Find Corresponding Token IDs

**[ZH]** 最简单的方法是使用开源的 Python `tiktoken` 库，或者直接写一个短脚本通过该模型的官方 Tokenizer 查：
**[EN]** The easiest way is to use the open-source Python `tiktoken` library, or write a short script to check via the model's official Tokenizer:

```python
from transformers import AutoTokenizer

# 加载你作为算子使用的那个本地小模型 / Load the local small model you use as an evaluator
tokenizer = AutoTokenizer.from_pretrained("Qwen/Qwen2.5-0.5B-Instruct")

# 查找 "0" 和 "1" 在这套模型里的真实物理 ID / Find the real physical IDs for "0" and "1" in this model
tokens_0 = tokenizer.encode("0")
tokens_1 = tokenizer.encode("1")

print(f"'0' 的 Token ID 是 / Token ID for '0' is: {tokens_0}")
print(f"'1' 的 Token ID 是 / Token ID for '1' is: {tokens_1}")
```

**[ZH]** 假设上述脚本打印出 `'0'` 的 ID 是 `15`，`'1'` 的 ID 是 `16`，相应的配置应当为：
**[EN]** Assuming the script above prints that the ID for `'0'` is `15` and `'1'` is `16`, the corresponding configuration should be:
```yaml
      logit_bias: 
        "15": 100
        "16": 100
```
*(注意：YAML 中要求 JSON 的 Key 必须是字符串，务必要给数字加引号 / Note: YAML requires JSON Keys to be strings, make sure to add quotes to numbers)*

---

## 3. 使用 `eval-cli` 工具进行单步调优验证 / Using `eval-cli` for Step-by-Step Tuning Validation

**[ZH]** 我们为您提供了一个独立于主流水线的调试工具。在你真正把网关部署并切流之前，**必须**使用该工具验证你的 Prompt 和 Logit Bias 是否奏效。
**[EN]** We provide a standalone debugging tool independent of the main pipeline. Before you actually deploy the gateway and switch traffic, you **must** use this tool to verify if your Prompt and Logit Bias are effective.

1. **准备模拟数据 / Prepare mock data** `mock_chat.json`:
   ```json
   {
     "messages": [
       {"role": "user", "content": "你好，在吗？ / Hello, are you there?"}
     ]
   }
   ```

2. **本地启动你的小模型后端 / Start your local small model backend** (e.g. Ollama):
   ```bash
   ollama run qwen2.5:0.5b
   ```

3. **运行独立调试工具 / Run standalone debug tool**:
   ```bash
   go run cmd/eval-cli/main.go --config example/evaluator_config.yaml --evaluator complexity --input mock_chat.json
   ```

4. **观察输出 / Observe Output**:
   **[ZH]** 如果配置完全正确，并且模型遵循了 Logit Bias 的限制，你应该看到它在 50~150ms 内稳定返回数字 `0` 并被成功解析。
   **[EN]** If perfectly configured and the model follows Logit Bias constraints, you should see it stably return the number `0` within 50~150ms and successfully parse it.
   ```text
   === Evaluation Result ===
   Evaluator Dimension: complexity
   Score:               0
   Time Taken (TTFT):   78.4ms
   ```

## 4. 获取平滑的概率值 (0.0 ~ 1.0) / Getting Smooth Probability Values (0.0 ~ 1.0)

**[ZH]** 如果您希望模型不仅输出 `0` 或 `1`，而是希望得到类似 `0.85` 的概率平滑值（例如：0.85 意味着模型认为该问题有 85% 的概率是复杂任务），您可以使用 **`llm_logprob_api`** 这一高级算子类型。
**[EN]** If you want the model to output a smooth probability score like `0.85` (e.g., 0.85 means the model believes there is an 85% chance this is a complex task) instead of a hard `0` or `1`, you can use the advanced evaluator type **`llm_logprob_api`**.

**[ZH]** **配置方法：** 在 `config.yaml` 中，将算子的 `type` 修改为 `llm_logprob_api`，同时**保留 `logit_bias`** 设置。
**[EN]** **Configuration Method:** In `config.yaml`, change the evaluator `type` to `llm_logprob_api`, while **retaining the `logit_bias`** configuration.

```yaml
    - name: "prob_complexity"
      type: "llm_logprob_api"    # 高级类型 / Advanced Type
      endpoint: "http://localhost:11434/v1/chat/completions"
      model: "qwen2.5:0.5b"
      logit_bias: 
        "15": 100
        "16": 100
```

**[ZH]** **工作原理：** 该选项依然在底层强制模型只能选择 `0` 或 `1`，但系统不会直接返回该硬分类结果，而是通过 OpenAI 标准协议中的 `top_logprobs` 取回模型在生成这一个 Token 时，备选词汇表里 `0` 和 `1` 的原始对数概率。然后使用 Softmax 公式将其转换为精确的长尾浮点分数。
借此，您可以在 `resolution_strategy` 里写出更加柔性的表达式（例如：`- condition: "prob_complexity > 0.6"`）。
**[EN]** **How it works:** This option still strictly forces the model to only select `0` or `1` at the lowest level. However, the system does not directly return this hard classification result. Instead, it retrieves the raw log probabilities of `0` and `1` from the alternate vocabulary when generating this single Token via the `top_logprobs` field in standard OpenAI protocol. It then uses the Softmax formula to convert this into a precise floating-point score.
With this, you can write more flexible expressions in `resolution_strategy` (e.g.: `- condition: "prob_complexity > 0.6"`).

## 5. 其它注意事项 / Other Considerations

**[ZH]** **Timeout 设置原则**：模型越大，出首字（TTFT）越慢。建议算子使用的模型参数量控制在 1.5B 以下，并且在 `config.yaml` 中严格设置 `timeout_ms: 60` 或 `100`。超时后网关会自动执行降级，跳过拦截直接去远端，**确保核心服务不断流**。
**[EN]** **Timeout Configuration Principle**: The larger the model, the slower the Time To First Token (TTFT). It is recommended to keep the parameter size of the model used by the evaluator under 1.5B, and strictly set `timeout_ms: 60` or `100` in `config.yaml`. Upon timeout, the gateway will automatically degrade and bypass the interception, routing directly to the remote, **ensuring uninterrupted core service**.
