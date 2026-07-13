---
parent: map.md
type: research
status: closed
assignee: agent
resolution: prompt-based
---

## Question

How should the system detect the customer's language from their first message and ensure all subsequent replies in the same session use that language?

## Resolution

**Language detection is prompt-based — no external library needed.**

### Root cause analysis

通过检查 OpenClaw session 文件（JSONL），发现三个导致语言混乱的原因：

1. **中文偏置**: `build-agent-prompt.js` 生成的 AGENTS.md 全部为中文指令。身份保密规则 #7 硬编码了中文回复："我是客服小助手，有什么可以帮您的？"——即使客户全程用英语，身份类问题也会突然切换到中文。

2. **语言规则太弱**: 规则 #5 "用客户的语言回复（中文问→中文答，英文问→英文答）" 过于简单，LLM 执行不严格。

3. **架构约束**: 管理后台不在消息路由路径中（消息流是 WhatsApp → OpenClaw Gateway → agent），无法在消息层面做预处理。语言检测必须通过 prompt 实现。

### 决定：三层 Prompt 强化方案

#### 层1：去中文偏置 — 规则 #7 改为语言中立
```
旧: 如果有人问你是谁 → 只回答："我是客服小助手，有什么可以帮您的？"
新: When asked about your identity, reply in the customer's language that you are a customer service
    assistant. Never use a fixed phrase in a specific language.
    Examples: EN customer → "I'm your customer service assistant, how can I help?"
              ZH customer → "我是客服小助手，有什么可以帮您的？"
              JA customer → "カスタマーサービスアシスタントです。お手伝いしましょうか？"
```

#### 层2：强化语言规则 — 规则 #5 重写
```
旧: 用客户的语言回复（中文问→中文答，英文问→英文答）
新: LANGUAGE RULE (highest priority after safety):
    1. Detect the customer's language from their FIRST message in the conversation.
    2. Reply in THAT language for the ENTIRE conversation.
    3. Never mix languages in a single reply.
    4. Never switch languages unless the customer explicitly switches first.
    5. If the first message is ambiguous (e.g., just "hi"), default to the language of
       subsequent messages. If still ambiguous, use English.
```

#### 层3：新增语言锚定规则
```
### 0. 语言锚定（最高优先级）
在整个对话过程中，你必须使用与客户第一条消息相同的语言回复。
这是硬性规则，优先级等同安全限制。
- 客户用英语 → 全程英语
- 客户用日语 → 全程日语
- 客户用德语 → 全程德语
不允许中英混合、不允许中途切换语言。
```

### 实施要点

- 修改文件: `src/build-agent-prompt.js`
- 不需要新依赖
- 不需要修改 server.js 或 openclaw-bridge.js
- 语言检测由 LLM 原生能力完成（DeepSeek V4 支持多语言）
- 需要将所有硬编码的中文回复改为多语言示例格式

### 验证标准

1. 英语客户发 "hello" → 回复全部为英语，无中文字符
2. 日语客户发 "こんにちは" → 回复全部为日语
3. 德语客户发 "Hallo" → 回复全部为德语
4. 身份类问题（"你是谁" / "What is your name"）→ 回复语言与客户一致
