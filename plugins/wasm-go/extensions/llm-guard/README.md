# LLM Guard 插件

LLM Guard 插件用于与 [LLM Guard](https://llmguard.io/) 服务集成，实现对 LLM 输入内容的实时安全检测和清洗。

## 功能特性

- **替换模式 (MASK)**: 调用 `/analyze/prompt` 清洗并替换内容
- **阻断模式 (BLOCK)**: 调用 `/scan/prompt` 检测风险并阻断请求
- **支持 OpenAI 格式**: 兼容 OpenAI 等主流 LLM API 格式

## 工作流程

### MASK 模式（替换）

```
请求 → /analyze/prompt
         ├─ is_valid: true → 替换内容 → 继续转发
         └─ is_valid: false → 继续转发（不做阻断）
```

### BLOCK 模式（阻断）

```
请求 → /scan/prompt
         ├─ is_valid: true → 继续转发
         └─ is_valid: false → 阻断请求
```

## 配置说明

### 基本配置

```yaml
serviceName: llm-guard-service  # LLM Guard 服务名称
servicePort: 8000               # LLM Guard 服务端口
serviceHost: llm-guard.default.svc.cluster.local  # LLM Guard 服务地址
```

### 运行模式配置

```yaml
# 默认操作：MASK（替换）、BLOCK（阻断）
# MASK 模式：调用 /analyze/prompt 替换内容
# BLOCK 模式：调用 /scan/prompt 阻断检测
defaultAction: MASK
```

### 检查范围配置

```yaml
checkRequest: true   # 是否检查请求
checkResponse: false # 是否检查响应（暂不支持）
scope: input         # 检查范围: input/output/both
```

### 内容提取配置

```yaml
# 请求体中提取内容的路径（OpenAI 格式）
requestJsonPath: "messages.@reverse.0.content"
```

### 阻断配置

```yaml
denyCode: 200       # 阻断时的 HTTP 状态码
denyMessage: "内容包含风险，已被拦截"  # 阻断时的提示信息
timeout: 5000       # LLM Guard 服务调用超时时间（毫秒）
```

### 端点配置

```yaml
# LLM Guard 端点配置
endpointAnalyzePrompt: "/analyze/prompt"  # 内容清洗端点
endpointScanPrompt: "/scan/prompt"        # 内容扫描端点
```

## 完整配置示例

```yaml
# LLM Guard 插件配置示例
serviceName: llm-guard-service
servicePort: 8000
serviceHost: llm-guard.default.svc.cluster.local

checkRequest: true
checkResponse: false
scope: input

# 运行模式：MASK（替换）或 BLOCK（阻断）
defaultAction: MASK

requestJsonPath: "messages.@reverse.0.content"

denyCode: 200
denyMessage: "内容包含风险，已被拦截"
timeout: 5000

endpointAnalyzePrompt: "/analyze/prompt"
endpointScanPrompt: "/scan/prompt"
```

## API 说明

### LLM Guard Analyze Prompt API

**请求格式**:
```json
{
  "prompt": "用户输入的提示词内容",
  "scanners_suppress": ["optional-scanner-to-suppress"]
}
```

**响应格式**:
```json
{
  "sanitized_prompt": "清洗后的提示词",
  "is_valid": true,
  "scanners": {
    "scanner_name": 0.95
  }
}
```

### LLM Guard Scan Prompt API

**请求格式**:
```json
{
  "prompt": "用户输入的提示词内容",
  "scanners_suppress": ["optional-scanner-to-suppress"]
}
```

**响应格式**:
```json
{
  "is_valid": false,
  "scanners": {
    "scanner_name": 0.95
  }
}
```

## 日志属性

插件会在日志中记录以下用户属性：

| 属性名 | 说明 |
|--------|------|
| `llm_guard_request_rt` | 请求处理耗时（毫秒） |
| `llm_guard_status` | 处理状态：`request sanitized`、`request deny`、`request pass` |
| `llm_guard_scanners` | 触发的扫描器名称列表 |

## 注意事项

1. **响应检查暂不支持**: 目前仅实现了请求内容的检查，响应内容的检查功能暂未实现
2. **超时设置**: 建议根据 LLM Guard 服务的实际响应时间调整 `timeout` 配置
3. **依赖服务**: 需要预先部署 LLM Guard 服务，并确保网络连通性

## 部署参考

### LLM Guard 服务部署

推荐使用 Helm 部署 LLM Guard:

```bash
helm repo add llm-guard https://laiyer-ai.github.io/llm-guard
helm install llm-guard llm-guard/llm-guard
```

### Higress 插件配置

在 Higress 控制台或通过 Kubernetes CRD 配置插件:

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: llm-guard
  namespace: higress-system
spec:
  defaultConfig:
    serviceName: llm-guard
    servicePort: 8000
    serviceHost: llm-guard.default.svc.cluster.local
    checkRequest: true
    checkResponse: false
    scope: input
    defaultAction: MASK
    requestJsonPath: "messages.@reverse.0.content"
    denyCode: 200
    denyMessage: "内容包含风险，已被拦截"
    timeout: 5000
```

## 相关链接

- [LLM Guard 官方文档](https://llm-guard.com/)
- [LLM Guard GitHub](https://github.com/laiyer/llm-guard)
- [Higress 官方文档](https://higress.io/)
