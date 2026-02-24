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
  "language": "en",
  "entities": ["PERSON", "EMAIL_ADDRESS", "PHONE_NUMBER", ...],
  "correlation_id": "req-123456",
  "score_threshold": 0.5,
  "return_decision_process": true,
  "ad_hoc_recognizers": [...],
  "context": ["doctor", "patient", "hospital"],
  "allow_list": ["John", "Jane"],
  "allow_list_match": "exact",
  "regex_flags": 6
}
```

**请求字段说明：**
- `text`: 要分析的文本内容（必需）
- `language`: 文本语言代码（如 "en"、"zh"）（必需，默认为 "en"）
- `entities`: 要检测的实体类型列表（可选，如果未指定则检测所有支持的实体）
  - 支持的实体类型包括：PERSON、EMAIL_ADDRESS、PHONE_NUMBER、CREDIT_CARD、URL、IP_ADDRESS 等
- `correlation_id`: 跨调用ID用于日志追踪（可选）
- `score_threshold`: 最小置信度阈值，低于此值的实体将被过滤（可选，服务端处理）
- `return_decision_process`: 是否在响应中返回决策过程调试信息（可选，默认为 false）
  - 当设置为 true 时，响应中会包含 `analysis_explanation` 字段
- `ad_hoc_recognizers`: 自定义识别器列表（可选，用于定义特定的实体模式）
- `context`: 上下文关键词列表（可选，用于提高特定实体的检测准确度）
  - 例如：检测医疗场景中的姓名时，可以包含 "医生"、"患者"、"医院" 等关键词
- `allow_list`: 允许列表（可选），包含不应被识别为 PII 的值
- `allow_list_match`: 允许列表匹配方式（可选，默认为 "exact"）
  - 可选值："exact"（精确匹配）、"substring"（子字符串匹配）
- `regex_flags`: 正则表达式标志（可选，默认为 6，代表 DOTALL | MULTILINE | IGNORECASE）

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
  "analyzer_results": [
    {
      "entity_type": "PERSON",
      "start": 0,
      "end": 10,
      "score": 0.98,
      "analysis_explanation": null,
      "recognition_metadata": null
    },
    {
      "entity_type": "EMAIL_ADDRESS",
      "start": 20,
      "end": 35,
      "score": 0.95,
      "analysis_explanation": null,
      "recognition_metadata": null
    }
  ],
  "anonymizers": {
    "PERSON": {
      "type": "replace",
      "new_value": "<PERSON>"
    },
    "EMAIL_ADDRESS": {
      "type": "mask",
      "masking_char": "*",
      "chars_to_mask": 10,
      "from_end": false
    }
  }
}
```

**请求格式说明：**
- `text`: 需要匿名化的原始文本内容（必需）
- `analyzer_results`: 来自 Presidio Analyzer 的分析结果数组（必需）
  - 每个结果包含：
    - `entity_type`: 检测到的实体类型（如 "PERSON", "EMAIL_ADDRESS", "CREDIT_CARD" 等）
    - `start`: 实体在文本中的起始位置（从 0 开始）
    - `end`: 实体在文本中的结束位置（不包含）
    - `score`: 检测的置信分数（范围 0-1，值越高表示检测越可靠）
    - `analysis_explanation`: 分析解释信息（可选，通常为 null）
    - `recognition_metadata`: 识别元数据（可选，通常为 null）
- `anonymizers`: 匿名化操作配置对象（可选，使用默认操作符时不需要）
  - 键：实体类型名称（如 "PERSON", "EMAIL_ADDRESS"）或 "DEFAULT" 通用配置
  - 值：操作配置对象，包含：
    - `type`: 操作符类型（必需），支持以下操作符：
      - `replace`: 用指定值替换 PII
      - `mask`: 使用掩码字符替换指定数量的字符
      - `redact`: 完全删除 PII 实体
      - `encrypt`: 使用密钥加密 PII
      - `hash`: 使用哈希算法处理 PII
      - `decrypt`: 解密之前加密的 PII（用于反匿名化）

**支持的操作符类型及参数：**

1. **replace 操作符**：用指定值替换整个实体
```json
{
  "PERSON": {
    "type": "replace",
    "new_value": "<ANONYMIZED>"
  }
}
```
参数：
- `new_value`: 替换后的新值

2. **mask 操作符**：使用掩码字符替换指定数量的字符
```json
{
  "PHONE_NUMBER": {
    "type": "mask",
    "masking_char": "*",
    "chars_to_mask": 4,
    "from_end": true
  }
}
```
参数：
- `masking_char`: 掩码字符（默认: "*"）
- `chars_to_mask`: 要掩码的字符数（默认: 实体全长度）
- `from_end`: 是否从末尾开始掩码（默认: false）

**mask 操作符示例：**
- `chars_to_mask: 4, from_end: false`: "John Doe" → "**** Doe"
- `chars_to_mask: 4, from_end: true`: "John Doe" → "John ****"
- `chars_to_mask: 不指定`: "John Doe" → "********"

3. **redact 操作符**：完全删除 PII 实体
```json
{
  "PERSON": {
    "type": "redact"
  }
}
```
参数：无

**redact 操作符示例：**
- "My name is John Doe" → "My name is "

4. **encrypt 操作符**：使用密钥加密 PII（用于隐私保护）
```json
{
  "CREDIT_CARD": {
    "type": "encrypt",
    "key": "WmZq4t7w!z%C&F)J"
  }
}
```
参数：
- `key`: 加密密钥（必需）

**注意：** 加密是可逆的，需要安全地保存密钥以便后续解密。

5. **hash 操作符**：使用哈希算法处理 PII（不可逆）
```json
{
  "PERSON": {
    "type": "hash"
  }
}
```
参数：
- `hash_type`: 哈希类型（可选，如 "sha256"）

**注意：** 哈希是不可逆的，无法恢复原始值。

6. **decrypt 操作符**：解密之前加密的 PII（用于反匿名化）
```json
{
  "DEFAULT": {
    "type": "decrypt",
    "key": "WmZq4t7w!z%C&F)J"
  }
}
```
参数：
- `key`: 解密密钥（必需）

**DEFAULT 通用配置：**
可以使用 `DEFAULT` 键为所有未指定配置的实体类型设置默认操作符：
```json
{
  "anonymizers": {
    "DEFAULT": {
      "type": "replace",
      "new_value": "<REDACTED>"
    },
    "PERSON": {
      "type": "mask",
      "masking_char": "*"
    }
  }
}
```

**注意：**
- 如果未提供 `anonymizers` 配置，Presidio 将使用服务端配置的默认 anonymizer
- `analyzer_results` 字段名是固定的，不要与 `anonymizer_results` 混淆
- `decrypt` 操作符通常用于反匿名化场景，需要在反匿名化请求中提供 `deanonymizers` 配置

**匿名化响应：**
```json
{
  "text": "<包含 PII 已屏蔽的文本>",
  "items": [
    {
      "start": 0,
      "end": 10,
      "text": "<PERSON>",
      "operator": "replace",
      "entity_type": "PERSON"
    },
    {
      "start": 20,
      "end": 35,
      "text": "<EMAIL_ADDRESS>",
      "operator": "replace",
      "entity_type": "EMAIL_ADDRESS"
    }
  ]
}
```

**响应格式说明：**
- `text`: 匿名化后的文本内容
- `items`: 匿名化操作的详细信息数组
  - 每个操作包含：
    - `start`: 原始文本中替换的起始位置
    - `end`: 原始文本中替换的结束位置
    - `text`: 替换后的占位符文本
    - `operator`: 操作类型（如 "replace"）
    - `entity_type`: 被替换的实体类型（可选）

#### 反匿名化（Deanonymization）

**反匿名化请求结构：**
```json
{
  "text": "My name is S184CMt9Drj7QaKQ21JTrpYzghnboTF9pn/neN8JME0=",
  "deanonymizers": {
    "PERSON": {
      "type": "decrypt",
      "key": "WmZq4t7w!z%C&F)J"
    }
  },
  "anonymizer_results": [
    {
      "start": 11,
      "end": 55,
      "entity_type": "PERSON"
    }
  ]
}
```

**请求格式说明：**
- `text`: 需要反匿名化的文本（必需，通常包含之前加密的 PII）
- `deanonymizers`: 反匿名化操作配置对象（必需）
  - 键：实体类型名称（如 "PERSON", "EMAIL_ADDRESS"）或 "DEFAULT" 通用配置
  - 值：操作配置对象，包含：
    - `type`: 操作符类型（必需），通常为 "decrypt" 用于解密
    - `key`: 解密密钥（必需，与加密时使用的密钥相同）
- `anonymizer_results`: 来自之前匿名化操作的结果数组（必需）
  - 每个结果包含：
    - `start`: 匿名化文本中实体的起始位置
    - `end`: 匿名化文本中实体的结束位置
    - `entity_type`: 实体类型
    - `text`: 匿名化后的文本值（可选）

**反匿名化响应：**
```json
{
  "text": "My name is John Doe",
  "items": [
    {
      "start": 11,
      "end": 19,
      "entity_type": "PERSON",
      "operator": "decrypt",
      "text": "John Doe"
    }
  ]
}
```

**反匿名化使用场景：**
- 在需要原始数据的下游系统中恢复加密的 PII
- 用于审计和合规性检查
- 在某些业务流程中需要解密之前匿名化的数据

**安全注意事项：**
- 加密密钥必须安全存储和管理
- 反匿名化操作应只在受控环境中进行
- 考虑使用密钥管理系统（KMS）来管理加密密钥
- 记录所有反匿名化操作以进行审计

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

**示例 5：使用 mask 操作符部分屏蔽电话号码**
```json
{
  "text": "我的电话号码是 13812345678",
  "analyzer_results": [
    {
      "entity_type": "PHONE_NUMBER",
      "start": 7,
      "end": 19,
      "score": 0.95
    }
  ],
  "anonymizers": {
    "PHONE_NUMBER": {
      "type": "mask",
      "masking_char": "*",
      "chars_to_mask": 4,
      "from_end": true
    }
  }
}
// 结果: "我的电话号码是 1381234****"
```

**示例 6：使用 replace 操作符自定义替换值**
```json
{
  "text": "请发送邮件到 john.doe@example.com",
  "analyzer_results": [
    {
      "entity_type": "EMAIL_ADDRESS",
      "start": 8,
      "end": 28,
      "score": 0.98
    }
  ],
  "anonymizers": {
    "EMAIL_ADDRESS": {
      "type": "replace",
      "new_value": "[邮箱已隐藏]"
    }
  }
}
// 结果: "请发送邮件到 [邮箱已隐藏]"
```

**示例 7：使用 redact 操作符完全删除 PII**
```json
{
  "text": "我的身份证号是 123456789012345678",
  "analyzer_results": [
    {
      "entity_type": "ID_NUMBER",
      "start": 7,
      "end": 23,
      "score": 0.92
    }
  ],
  "anonymizers": {
    "ID_NUMBER": {
      "type": "redact"
    }
  }
}
// 结果: "我的身份证号是"
```

**示例 8：使用 encrypt 操作符加密敏感数据**
```json
{
  "text": "信用卡号是 4111111111111111",
  "analyzer_results": [
    {
      "entity_type": "CREDIT_CARD",
      "start": 6,
      "end": 22,
      "score": 0.99
    }
  ],
  "anonymizers": {
    "CREDIT_CARD": {
      "type": "encrypt",
      "key": "my-secret-key-123"
    }
  }
}
// 结果: "信用卡号是 U2FsdGVkX1..."
```

**示例 9：使用 DEFAULT 配置应用通用操作符**
```json
{
  "text": "联系张三，电话13800000000，邮箱zhang@example.com",
  "analyzer_results": [
    {"entity_type": "PERSON", "start": 2, "end": 4, "score": 0.85},
    {"entity_type": "PHONE_NUMBER", "start": 6, "end": 17, "score": 0.92},
    {"entity_type": "EMAIL_ADDRESS", "start": 18, "end": 35, "score": 0.98}
  ],
  "anonymizers": {
    "DEFAULT": {
      "type": "replace",
      "new_value": "[已屏蔽]"
    },
    "EMAIL_ADDRESS": {
      "type": "mask",
      "masking_char": "*",
      "from_end": true
    }
  }
}
// 结果: "联系[已屏蔽]，电话[已屏蔽]，邮箱zhang***********"
```

**示例 10：反匿名化恢复加密数据**
```json
// 匿名化请求
{
  "text": "信用卡号是 4111111111111111",
  "analyzer_results": [{"entity_type": "CREDIT_CARD", "start": 6, "end": 22, "score": 0.99}],
  "anonymizers": {"CREDIT_CARD": {"type": "encrypt", "key": "my-secret-key-123"}}
}

// 反匿名化请求
{
  "text": "信用卡号是 U2FsdGVkX1...",
  "anonymizer_results": [{"start": 6, "end": 20, "entity_type": "CREDIT_CARD"}],
  "deanonymizers": {"CREDIT_CARD": {"type": "decrypt", "key": "my-secret-key-123"}}
}

// 结果: "信用卡号是 4111111111111111"
```

## 最佳实践

1. **从默认阈值开始** 并根据误报/漏报率进行调整
2. **使用每个请求的语言配置** 处理多语言内容
3. **记录被阻断的实体** 以了解捕获的内容
4. **使用真实数据测试** 部署到生产环境之前
5. **考虑隐私要求** - 根据敏感度选择屏蔽或阻断
6. **注意 tools 参数** - 如果 tools 包含敏感信息，需要额外处理
7. **选择合适的匿名化操作符**：
   - `replace`：适合需要保留占位符的场景，易于调试
   - `mask`：适合需要保留部分信息的场景（如保留电话号码区号）
   - `redact`：适合需要完全移除敏感数据的场景（如日志脱敏）
   - `hash`：适合需要不可逆脱敏的场景（如生成匿名 ID）
   - `encrypt`：适合需要后续恢复的场景（如数据分析）
8. **谨慎使用加密和反匿名化**：
   - 加密密钥必须使用 KMS 或其他安全机制管理
   - 避免在不必要的情况下进行反匿名化
   - 记录所有反匿名化操作以进行审计
9. **使用 DEFAULT 配置简化管理**：
   - 为大多数实体设置默认操作符
   - 仅对特殊实体单独配置
   - 减少配置复杂度和维护成本
10. **测试匿名化效果**：
    - 验证匿名化后文本的可读性
    - 确保匿名化不会破坏业务逻辑
    - 测试各种实体类型的处理效果
11. **监控匿名化性能**：
    - 匿名化操作可能会增加请求延迟
    - 监控 PII 检测和匿名化的处理时间
    - 对于高吞吐量场景考虑性能优化

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
