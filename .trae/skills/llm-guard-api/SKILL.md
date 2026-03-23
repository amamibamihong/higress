# LLM Guard API 技能文档

## 概述

LLM Guard API 提供 AI 安全扫描服务，主要针对 LLM（大语言模型）的输入（Prompt）和输出（Output）进行安全性检测和清理。

---

## 核心概念

### 操作类型

- **`analyze` (分析)**: 接收文本，执行扫描，并返回一个"清理"过（sanitized）的、安全的文本版本以及扫描结果
- **`scan` (扫描)**: 接收文本，并行执行扫描，快速返回文本是否有效以及各扫描器的风险评分，但**不**返回清理后的文本

### 扫描范围

- **输入扫描 (Prompt Scanning)**: 对用户输入给 LLM 的内容进行安全检测
- **输出扫描 (Output Scanning)**: 对 LLM 生成的响应内容进行安全检测

---

## 认证方式

所有核心 API 接口都需要认证。根据 `config/scanners.yml` 的默认配置，认证方式为 `http_bearer`。

**请求头示例:**
```
Authorization: Bearer YOUR_AUTH_TOKEN
```

---

## API 接口

### 技能一：输入扫描 (Prompt Scanning)

#### 1.1 分析输入 (Analyze Prompt)

- **Endpoint**: `POST /analyze/prompt`
- **描述**: 分析一个 prompt，返回经过清理（sanitized）的 prompt 和扫描器的结果。如果任何扫描器判定输入无效，`is_valid` 将为 `false`。

**请求体:**
```json
{
  "prompt": "用户的原始输入文本",
  "scanners_suppress": ["Toxicity"]
}
```
- `prompt` (str, 必需): 需要分析的输入文本
- `scanners_suppress` (List[str], 可选): 需要临时禁用的扫描器名称列表

**成功响应体:**
```json
{
  "sanitized_prompt": "清理后的安全输入文本",
  "is_valid": true,
  "scanners": {
    "Language": 0.0,
    "Toxicity": 0.1
  }
}
```

**示例 (`curl`):**
```sh
curl -X POST "http://localhost:8000/analyze/prompt" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
     -d '{"prompt": "Tell me a secret."}'
```

#### 1.2 扫描输入 (Scan Prompt)

- **Endpoint**: `POST /scan/prompt`
- **描述**: 并行扫描一个 prompt，快速返回其是否有效及风险评分。

**请求体:**
```json
{
  "prompt": "用户的原始输入文本",
  "scanners_suppress": ["Toxicity"]
}
```

**成功响应体:**
```json
{
  "is_valid": true,
  "scanners": {
    "Language": 0.0,
    "Toxicity": 0.1
  }
}
```

**示例 (`curl`):**
```sh
curl -X POST "http://localhost:8000/scan/prompt" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
     -d '{"prompt": "Hello, how are you?"}'
```

---

### 技能二：输出扫描 (Output Scanning)

#### 2.1 分析输出 (Analyze Output)

- **Endpoint**: `POST /analyze/output`
- **描述**: 分析 LLM 的输出，返回经过清理（sanitized）的输出和扫描器的结果。

**请求体:**
```json
{
  "prompt": "用户的原始输入文本",
  "output": "LLM 生成的原始输出文本",
  "scanners_suppress": ["NoRefusal"]
}
```
- `prompt` (str, 必需): 对应的用户输入（用于上下文关联，如 `Relevance` 扫描器）
- `output` (str, 必需): 需要分析的 LLM 输出

**成功响应体:**
```json
{
  "sanitized_output": "清理后的安全输出文本",
  "is_valid": true,
  "scanners": {
    "Relevance": 0.1,
    "Toxicity": 0.05
  }
}
```

**示例 (`curl`):**
```sh
curl -X POST "http://localhost:8000/analyze/output" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
     -d '{"prompt": "What is the capital of France?", "output": "The capital of France is Paris."}'
```

#### 2.2 扫描输出 (Scan Output)

- **Endpoint**: `POST /scan/output`
- **描述**: 并行扫描 LLM 的输出，快速返回其是否有效及风险评分。

**请求体:**
```json
{
  "prompt": "用户的原始输入文本",
  "output": "LLM 生成的原始输出文本"
}
```

**成功响应体:**
```json
{
  "is_valid": true,
  "scanners": {
    "Relevance": 0.1,
    "Toxicity": 0.05
  }
}
```

**示例 (`curl`):**
```sh
curl -X POST "http://localhost:8000/scan/output" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
     -d '{"prompt": "What is the capital of France?", "output": "The capital of France is Paris."}'
```

---

### 辅助技能

- `GET /`: 返回 API 名称，可用于简单的连通性测试
- `GET /healthz`: 健康检查端点，返回 `{"status": "alive"}`
- `GET /readyz`: 就绪检查端点，返回 `{"status": "ready"}`
- `GET /metrics`: (如果启用 Prometheus) 返回 Prometheus 格式的监控指标

---

## 使用场景

### 场景一：输入安全检查
在对用户输入进行处理前，使用 `analyze/prompt` 或 `scan/prompt` 进行安全检查，防止恶意输入攻击。

### 场景二：输出内容审核
在将 LLM 生成的响应返回给用户前，使用 `analyze/output` 或 `scan/output` 进行内容审核，确保输出内容安全合规。

### 场景三：全链路安全防护
结合输入扫描和输出扫描，构建从用户输入到 LLM 响应的完整安全防护链路。

---

## 错误处理

API 在出错时会返回相应的 HTTP 状态码和错误信息：

- `400 Bad Request`: 请求参数错误
- `401 Unauthorized`: 认证失败
- `500 Internal Server Error`: 服务器内部错误

错误响应示例：
```json
{
  "error": "Invalid request body",
  "details": "Missing required field: prompt"
}
```

---

## 总结

LLM Guard API 提供了完整的 AI 安全扫描能力，通过灵活的配置和丰富的 API 接口，帮助开发者轻松构建安全的 AI 应用。核心要点包括：

1. **双向扫描**：同时支持输入和输出扫描
2. **灵活操作**：提供 `analyze`（分析+清理）和 `scan`（仅扫描）两种操作模式
3. **可扩展**：支持自定义扫描器和配置
4. **标准化**：遵循 RESTful API 设计原则

更多详细信息请参考官方文档和示例代码。