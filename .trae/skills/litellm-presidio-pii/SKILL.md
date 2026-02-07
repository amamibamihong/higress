---
name: litellm-presidio-pii
description: LiteLLM 中 Presidio PII 检测、屏蔽和配置的综合指南
license: MIT
---

# LiteLLM Presidio PII 技能

本技能提供关于 LiteLLM 中 Presidio PII（个人身份信息）检测、屏蔽和阻断功能的专业知识。

## 概述

Presidio 是微软开源的文本匿名化服务。LiteLLM 通过 Guardrail Hooks 集成 Presidio，提供自动化的 PII 检测和屏蔽功能。

## 核心组件

### 1. 内容提取

**从请求中提取：**
- 内容从请求的 `messages` 字段中提取
- 消息可能包含 system、user 和 assistant 角色
- 检查的关键字段：`messages[i].content` 或 `messages[i].string_content`
- 多模态内容：检查 `content` 列表中每个元素的 `text` 字段

**从响应中提取：**
- 内容从模型响应中提取
- 检查的字段：`choices[0].message.content` 或 `__content__`
- 响应可能包含多个选项，使用主要内容

**重要说明：** 
- **`tools` 参数不会被检测** - Presidio 目前只检测 `messages` 中的内容
- 如果需要在 tools 中检测 PII，需要额外的自定义处理

### 2. PII 检测流程

**检测请求：**
- **端点**：`analyze_text()` 方法调用 Presidio analyze
- **请求参数**：
  - `text`：要分析的文本
  - `presidio_config`：包含实体类型、评分阈值、语言
  - `request_data`：用于日志/错误处理的请求元数据

**分析请求结构：**
```json
{
  "text": "要分析的字符串内容",
  "entities": ["PERSON", "EMAIL_ADDRESS", "PHONE_NUMBER", ...],
  "language": "en",
  "correlation_id": "uuid",
  "score_threshold": 0.85
}
```

**分析响应：**
```json
[
  {
    "entity_type": "PERSON",
    "start": 0,
    "end": 10,
    "score": 0.98
  },
  {
    "entity_type": "EMAIL_ADDRESS",
    "start": 20,
    "end": 35,
    "score": 0.95
  }
]
```

**响应格式说明：**
- Presidio Analyzer API 直接返回一个数组，不是包装在 `results` 对象中
- 每个结果元素包含的字段：
  - `entity_type`: 检测到的实体类型（如 "PERSON", "EMAIL_ADDRESS", "CREDIT_CARD" 等）
  - `start`: 实体在文本中的起始位置（从 0 开始）
  - `end`: 实体在文本中的结束位置（不包含）
  - `score`: 检测的置信分数（范围 0-1，值越高表示检测越可靠）

**错误响应处理：**
- 如果 Presidio 返回错误，响应将是一个包含 `error` 键的字典：
  ```json
  {"error": "No text provided"}
  ```
- 此时代码会返回空数组 `[]` 并记录警告日志

**评分阈值过滤：**
- 检测结果通过 `filter_analyze_results_by_score()` 进行过滤
- 每个结果根据配置的阈值进行检查：
  - 全局阈值：`presidio_score_thresholds`（每个实体的默认值）
  - 每个实体阈值：`presidio_score_thresholds.PERSON` 等
- 低于阈值的结果在屏蔽前被丢弃

### 3. 检测后操作

#### PII 替换（屏蔽）

**屏蔽请求：**
- **端点**：`anonymize_text()` 方法调用 Presidio anonymize
- 使用分析结果确定要屏蔽的内容
- 支持不同的屏蔽模式：hash、asterisk、redaction

**匿名化请求结构：**
```json
{
  "text": "原始文本",
  "anonymizers": {
    "DEFAULT": {
      "type": "hash",
      "hash_type": "sha256"
    }
  },
  "anonymize_results": [
    {
      "start": 0,
      "end": 10,
      "entity_type": "PERSON"
    }
  ]
}
```

**匿名化响应：**
```json
{
  "text": "<包含 PI I已屏蔽的文本>",
  "items": [
    {
      "start": 0,
      "end": 10,
      "entity_type": "PERSON",
      "anonymizer": "hash"
    }
  ]
}
```

#### 请求阻断

**阻断机制：**
- 实体类型可以配置为动作 "BLOCK"
- 阻断检查在 `raise_exception_if_blocked_entities_detected()` 中进行
- 检测到阻断实体时抛出 `BlockedPiiEntityError`

**阻断实体检查：**
```python
# 检查每个检测到的实体
for entity in analyze_results:
    entity_type = entity["entity_type"]
    if entity_type in blocked_entity_types:
        raise BlockedPiiEntityError(
            message=f"检测到被阻断的实体: {entity_type}",
        )
```

**阻断动作：**
- 请求在到达 LLM 之前被拒绝
- 错误被记录并包含完整详情
- 用户收到关于被阻断内容的错误消息

### 4. Presidio PII 配置

**配置位置：**
- 全局：在 `litellm_config.yaml` 中的 `guardrails.presidio` 下
- 每个请求：通过 `check_pii()` 中的 `PresidioPerRequestConfig`

**实体类型配置：**
```yaml
guardrails:
  presidio:
    entities:
      PERSON:
        action: "MASK"  # 或 "BLOCK"
      EMAIL_ADDRESS:
        action: "MASK"
      PHONE_NUMBER:
        action: "MASK"
      CREDIT_CARD:
        action: "BLOCK"
```

**动作类型：**
- `MASK`：用占位符/哈希替换 PII
- `BLOCK`：完全拒绝请求

**评分阈值：**
```yaml
guardrails:
  presidio:
    presidio_score_thresholds:
      PERSON: 0.85
      EMAIL_ADDRESS: 0.90
      PHONE_NUMBER: 0.80
      # 全局默认值
      DEFAULT: 0.75
```

**语言配置：**
```yaml
guardrails:
  presidio:
    language: "en"  # 默认语言
```

**检测范围配置（presidio_filter_scope）：**
```yaml
guardrails:
  presidio:
    presidio_filter_scope: "both"  # "input", "output", 或 "both"
```

- `input`：仅检测用户 → 模型的内容（messages 输入）
- `output`：仅检测模型 → 用户的内容（响应输出）
- `both`（默认）：检测两个方向

**每个请求覆盖：**
```python
check_pii(
    text=text,
    output_parse_pii=True,
    presidio_config=PresidioPerRequestConfig(
        entities=["PERSON", "EMAIL"],
        score_thresholds={"PERSON": 0.9},
        language="zh"
    ),
    request_data=request_data
)
```

### 5. 检测范围限制和扩展

**当前检测范围：**（`presidio.py:564` `async_pre_call_hook`）
- ✅ `messages[].content` - 消息内容字符串
- ✅ `messages[].string_content` - 替代字段
- ✅ 多模态 `messages[].content[].text` - list 元素的文本
- ❌ `tools` - 工具定义参数（不会检测）
- ❌ `tool_calls` - 工具调用参数（不会检测）
- ❌ 请求 metadata - 元信息（不会检测）

**不检测 tools 参数的示例：**
```json
{
  "messages": [{"role": "user", "content": "Help me"}],
  "tools": [{
    "function": {
      "name": "search_user",
      "description": "Search by email john.doe@example.com"
    }
  }]
}
```
上述请求中 `description` 的邮箱不会被检测。

**扩展检测到 tools：**
如果在 tools 中检测 PII，需要以下方式之一：
1. 在请求发送前手动预处理 tools 参数
2. 自定义 guardrail hook 检查 tools
3. 使用自定义 Python 脚本调用 Presidio 检测 tools

### 6. 提高检测准确率（增强词）

**自定义识别器：**
定义自定义模式以提高特定实体检测准确率：

```python
# 在 ad_hoc_recognizer 配置中
{
  "recognizers": [
    {
      "name": "ZipCodeRecognizer",
      "supported_entity": "ZIP_CODE",
      "patterns": [
        {
          "regexp": "\\b\\d{5}(-\\d{4})?\\b",
          "score": 0.95
        }
      ],
      "supported_language": "en"
    }
  ]
}
```

**上下文关键词：**
识别器可以包含上下文约束以提高准确率：

```python
{
  "recognizers": [
    {
      "name": "IDNumberRecognizer",
      "supported_entity": "ID_NUMBER",
      "patterns": [
        {
          "regexp": "\\b\\d{9}\\b",
          "score": 0.85
        }
      ],
      "context": [
        "ID",
        "ID number",
        "identification"
      ]
    }
  ]
}
```

**置信度分数调优：**
- 更高的阈值减少误报但可能遗漏一些 PII
- 更低的阈值捕获更多 PII 但增加误报率
- 根据用例调整每个实体的阈值

## 使用示例

**示例 1：基本 PII 屏蔽**
```python
# 检测并屏蔽用户消息中的 PII
original_text = "联系 John Doe，邮箱 john.doe@example.com"
masked_text = await presidio_hook.check_pii(
    text=original_text,
    output_parse_pii=False,
    presidio_config=None,
    request_data={}
)
# 结果: "联系 <PERSON>，邮箱 <EMAIL_ADDRESS>"
```

**示例 2：阻断敏感数据**
```python
# 配置阻断信用卡
guardrails:
  presidio:
    entities:
      CREDIT_CARD:
        action: "BLOCK"

# 如果检测到信用卡，请求将被拒绝
```

**示例 3：仅检测输出**
```python
# 配置仅检测模型输出
guardrails:
  presidio:
    presidio_filter_scope: "output"
```

**示例 4：自定义识别器**
```python
# 使用 ad-hoc 识别器检测自定义实体
with open("presidio_ad_hoc_recognizers.json") as f:
    recognizers = json.load(f)

check_pii(
    text=text,
    presidio_config=PresidioPerRequestConfig(
        ad_hoc_recognizers=recognizers
    ),
    ...
)
```

## 最佳实践

1. **从默认阈值开始** 并根据误报/漏报率进行调整
2. **使用每个请求的语言配置** 处理多语言内容
3. **记录被阻断的实体** 以了解捕获的内容
4. **使用真实数据测试** 部署到生产环境之前
5. **考虑隐私要求** - 根据敏感度选择屏蔽或阻断
6. **注意 tools 参数** - 如果 tools 包含敏感信息，需要额外处理

## 错误处理

**常见错误：**
- `BlockedPiiEntityError`：请求包含被阻断的实体类型
- Presidio 服务不可用：回退到原始文本或拒绝
- 配置无效：检查实体类型和阈值

**错误响应：**
```python
{
  "error": "检测到被阻断的实体: CREDIT_CARD",
  "details": {
    "entity_type": "CREDIT_CARD",
    "text": "...",
    "position": {"start": 10, "end": 20}
  }
}
```

## 资源

- Presidio 文档: https://microsoft.github.io/presidio/
- LiteLLM Guardrails: `/litellm/proxy/guardrails/`
- 示例 ad-hoc 识别器: `/litellm/proxy/hooks/example_presidio_ad_hoc_recognizer.json`
- PII 文档: `/docs/my-website/docs/proxy/guardrails/pii_masking_v2.md`
