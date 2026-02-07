---
title: Presidio PPI 检测
keywords: [higress, PII, presidio]
description: 使用 Presidio 检测和屏蔽个人身份信息(PII)
---

## 功能说明
通过对接 Presidio 服务，检测和屏蔽 AI 应用请求和响应中的个人身份信息(PII)，保护用户隐私。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`300`

## 配置说明
| Name | Type | Requirement | Default | Description |
| ------------ | ------------ | ------------ | ------------ | ------------ |
| `analyzerServiceName` | string | required | - | Higress 中创建的服务名。DNS 类型服务用 `.dns` 结尾（如 `presidio-analyzer.dns`），固定地址类型服务用 `.static` 结尾（如 `presidio-analyzer.static`） |
| `analyzerServicePort` | int | required | - | Presidio Analyzer 服务的端口。固定地址类型服务默认为 80 |
| `analyzerServiceHost` | string | required | - | 真实的 Presidio Analyzer 服务地址（域名或 IP，用于 HTTP Host 头） |
| `analyzerPath` | string | optional | `/` | Presidio Analyzer API 路径 |
| `anonymizerServiceName` | string | required | - | Higress 中创建的服务名。DNS 类型服务用 `.dns` 结尾（如 `presidio-anonymizer.dns`），固定地址类型服务用 `.static` 结尾（如 `presidio-anonymizer.static`） |
| `anonymizerServicePort` | int | required | - | Presidio Anonymizer 服务的端口。固定地址类型服务默认为 80 |
| `anonymizerServiceHost` | string | required | - | 真实的 Presidio Anonymizer 服务地址（域名或 IP，用于 HTTP Host 头） |
| `anonymizerPath` | string | optional | `/` | Presidio Anonymizer API 路径 |
| `checkRequest` | bool | optional | false | 检测请求内容中的 PII |
| `checkResponse` | bool | optional | false | 检测响应内容中的 PII |
| `filterScope` | string | optional | `both` | 检测范围：`input`、`output` 或 `both` |
| `language` | string | optional | `zh` | 检测语言 |
| `defaultAction` | string | optional | `MASK` | 默认动作：`MASK`、`BLOCK` 或 `NONE` |
| `defaultScoreThreshold` | float | optional | `0.85` | 默认评分阈值 |
| `anonymizer` | string | optional | `hash` | 屏蔽方式：`hash`、`asterisk` 或 `redact` |
| `denyCode` | int | optional | `200` | 检测到阻断实体时的响应状态码 |
| `denyMessage` | string | optional | `很抱歉，内容包含敏感信息` | 检测到阻断实体时的响应内容 |
| `protocol` | string | optional | `openai` | 协议格式，非 openai 协议填 `original` |
| `timeout` | int | optional | `2000` | 调用 Presidio 服务的超时时间(ms) |
| `bufferLimit` | int | optional | `1000` | 流式响应缓冲区限制 |
| `entities` | array | optional | - | 实体类型配置列表 |
| `entities[].entityType` | string | required | - | 实体类型，如 `PERSON`、`EMAIL_ADDRESS` 等 |
| `entities[].action` | string | optional | `defaultAction` | 该实体的动作 |
| `entities[].scoreThreshold` | float | optional | `defaultScoreThreshold` | 该实体的评分阈值 |

### 动作类型说明
- `MASK`：用占位符/哈希替换 PII
- `BLOCK`：完全拒绝请求
- `NONE`：不处理该实体

### 屏蔽方式说明
- `hash`：使用 SHA256 哈希替换
- `asterisk`：使用星号替换
- `redact`：直接删除

### 检测范围说明
- `input`：仅检测用户 → 模型的内容（请求输入）
- `output`：仅仅检测模型 → 用户的内容（响应输出）
- `both`：检测两个方向（默认）

## 配置示例

### 前提条件
由于插件中需要调用 Presidio 服务，所以需要先在 Higress 中创建服务。支持两种类型的服务：

**DNS 类型服务（用于域名）：**
- Analyzer 服务名：`presidio-analyzer.dns`
- Analyzer 服务端口：`8080`
- Analyzer 服务域名：`presidio-analyzer.example.com`
- Anonymizer 服务名：`presidio-anonymizer.dns`
- Anonymizer 服务端口：`8080`
- Anonymizer 服务域名：`presidio-anonymizer.example.com`

**固定地址类型服务（用于 IP）：**
- Analyzer 服务名：`presidio-analyzer.static`
- Analyzer 服务地址：`192.168.1.100:8080`
- Anonymizer 服务名：`presidio-anonymizer.static`
- Anonymizer 服务地址：`192.168.1.101:8080`

### 使用域名的基本配置：检测并屏蔽请求中的 PII

```yaml
analyzerServiceName: presidio-analyzer.dns
analyzerServicePort: 8080
analyzerServiceHost: presidio-analyzer.example.com
analyzerPath: "/analyze"

anonymizerServiceName: presidio-anonymizer.dns
anonymizerServicePort: 8080
anonymizerServiceHost: presidio-anonymizer.example.com
anonymizerPath: "/anonymize"

checkRequest: true
entities:
  - entityType: PERSON
    action: MASK
    scoreThreshold: 0.85
  - entityType: EMAIL_ADDRESS
    action: MASK
    scoreThreshold: 0.90
  - entityType: PHONE_NUMBER
    action: MASK
    scoreThreshold: 0.80
```

### 检测并阻断敏感数据

```yaml
analyzerServiceName: presidio-analyzer.dns
analyzerServicePort: 8080
analyzerServiceHost: presidio-analyzer.example.com
analyzerPath: "/analyze"

anonymizerServiceName: presidio-anonymizer.dns
anonymizerServicePort: 8080
anonymizerServiceHost: presidio-anonymizer.example.com
anonymizerPath: "/anonymize"

checkRequest: true
entities:
  - entityType: PERSON
    action: MASK
    scoreThreshold: 0.85
  - entityType: EMAIL_ADDRESS
    action: MASK
    scoreThreshold: 0.90
  - entityType: CREDIT_CARD
    action: BLOCK
    scoreThreshold: 0.95
```

### 使用 IP 地址配置：检测并屏蔽请求中的 PII

```yaml
analyzerServiceName: presidio-analyzer.static
analyzerServicePort: 80
analyzerServiceHost: 192.168.1.100
analyzerPath: "/analyze"

anonymizerServiceName: presidio-anonymizer.static
anonymizerServicePort: 80
anonymizerServiceHost: 192.168.1.101
anonymizerPath: "/anonymize"

checkRequest: true
entities:
  - entityType: PERSON
    action: MASK
    scoreThreshold: 0.85
  - entityType: EMAIL_ADDRESS
    action: MASK
    scoreThreshold: 0.90
  - entityType: PHONE_NUMBER
    action: MASK
    scoreThreshold: 0.80
```

### 检测输入与输出

```yaml
analyzerServiceName: presidio-analyzer.dns
analyzerServicePort: 8080
analyzerServiceHost: presidio-analyzer.example.com
analyzerPath: "/analyze"

anonymizerServiceName: presidio-anonymizer.dns
anonymizerServicePort: 8080
anonymizerServiceHost: presidio-anonymizer.example.com
anonymizerPath: "/anonymize"

checkRequest: true
checkResponse: true
filterScope: both
entities:
  - entityType: PERSON
    action: MASK
  - entityType: EMAIL_ADDRESS
    action: MASK
```

### 仅检测输出

```yaml
analyzerServiceName: presidio-analyzer.dns
analyzerServicePort: 8080
analyzerServiceHost: presidio-analyzer.example.com
analyzerPath: "/analyze"

anonymizerServiceName: presidio-anonymizer.dns
anonymizerServicePort: 8080
anonymizerServiceHost: presidio-anonymizer.example.com
anonymizerPath: "/anonymize"

checkResponse: true
filterScope: output
entities:
  - entityType: PERSON
    action: MASK
  - entityType: EMAIL_ADDRESS
    action: MASK
```

### 使用星号屏蔽

```yaml
analyzerServiceName: presidio-analyzer.dns
analyzerServicePort: 8080
analyzerServiceHost: presidio-analyzer.example.com
analyzerPath: "/analyze"

anonymizerServiceName: presidio-anonymizer.dns
anonymizerServicePort: 8080
anonymizerServiceHost: presidio-anonymizer.example.com
anonymizerPath: "/anonymize"

checkRequest: true
checkResponse: true
anonymizer: asterisk
entities:
  - entityType: PERSON
    action: MASK
  - entityType: EMAIL_ADDRESS
    action: MASK
```

### 配置非 openai 协议

```yaml
analyzerServiceName: presidio-analyzer.dns
analyzerServicePort: 8080
analyzerServiceHost: presidio-analyzer.example.com
analyzerPath: "/analyze"

anonymizerServiceName: presidio-anonymizer.dns
anonymizerServicePort: 8080
anonymizerServiceHost: presidio-anonymizer.example.com
anonymizerPath: "/anonymize"

checkRequest: true
checkResponse: true
requestContentJsonPath: "input.prompt"
responseContentJsonPath: "output.text"
denyCode: 200
denyMessage: "很抱歉，内容包含敏感信息"
protocol: original
entities:
  - entityType: PERSON
    action: MASK
```

## 可观测

### Metric
presidio-pii 插件提供了以下监控指标：
- `presidio_pii_request_deny`: 请求 PII 检测失败请求数
- `presidio_pii_response_deny`: 响应 PII 检测失败请求数

### Log Attributes
presidio-pii 插件会在日志中添加以下属性：
- `presidio_pii_request_rt`: 请求 PII 检测耗时(ms)
- `presidio_pii_response_rt`: 响应 PII 检测耗时(ms)
- `presidio_pii_status`: PII 检测状态（`request pass`、`request deny`、`request masked`、`response pass`、`response deny`、`response masked`）
- `presidio_pii_masked_count`: 屏蔽的实体数量
- `presidio_pii_blocked_entities`: 被阻断的实体类型列表

## 请求示例

```bash
curl http://localhost/v1/chat/completions \
-H "Content-Type: application/json" \
-d '{
  "model": "gpt-4o-mini",
  "messages": [
    {
      "role": "user",
      "content": "请联系 John Doe，邮箱是 john.doe@example.com"
    }
  ]
}'
```

如果配置了 PII 屏蔽，请求内容会被修改为：
```json
{
  "model": "gpt-4o-mini",
  "messages": [
    {
      "role": "user",
      "content": "请联系 <PERSON>，邮箱是 <EMAIL_ADDRESS>"
    }
  ]
}
```

如果配置了 PII 阻断，网关将返回：
```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "model": "from-presidio-pii",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "很抱歉，内容包含敏感信息"
      },
      "finish_reason": "stop"
    }
  ]
}
```

## Presidio 服务端点要求

插件需要 Presidio 服务提供以下端点：

### 1. Analyzer 端点
- 路径：`{analyzerPath}`
- 方法：POST
- 请求体：
```json
{
  "text": "要分析的文本",
  "entities": ["PERSON", "EMAIL_ADDRESS"],
  "language": "zh",
  "scoreThreshold": 0.85
}
```
- 响应体：
```json
{
  "results": [
    {
      "entity_type": "PERSON",
      "start": 0,
      "end": 10,
      "score": 0.98
    }
  ]
}
```

### 2. Anonymizer 端点
- 路径：`{anonymizerPath}`
- 方法：POST
- 请求体：
```json
{
  "text": "原始文本",
  "anonymize_results": [
    {
      "start": 0,
      "end": 10,
      "entity_type": "PERSON"
    }
  ],
  "anonymizers": {
    "DEFAULT": {
      "type": "hash",
      "hash_type": "sha256"
    }
  }
}
```
- 响应体：
```json
{
  "text": "<包含 PII 已屏蔽的文本>",
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

## 支持的实体类型

常见的 Presidio 实体类型包括：
- `PERSON`：人名
- `EMAIL_ADDRESS`：邮箱地址
- `PHONE_NUMBER`：电话号码
- `CREDIT_CARD`：信用卡号
- `IBAN_CODE`：国际银行账号
- `IP_ADDRESS`：IP 地址
- `URL`：URL
- `US_SSN`：美国社会保障号
- `UK_NHS`：英国国民健康服务号
- `DATE_TIME`：日期时间
- `LOCATION`：地理位置

更多实体类型请参考 [Presidio 文档](https://microsoft.github.io/presidio/supported_entities/)。
