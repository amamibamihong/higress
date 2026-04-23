---
title: Smart Route
keywords: [higress, ai, routing, llm]
description: 基于外部 LLM_SMART_ROUTE_API 进行智能模型选择和路由
---

# 功能说明

`smart-route` 插件实现了与外部智能路由服务（如 `LLM_SMART_ROUTE_API`）集成的功能。它能够从用户请求中提取提示词（Prompt），并将其发送至远程决策服务。根据远程服务的建议，插件会动态修改请求体中的 `model` 字段，从而实现基于意图或策略的智能模型路由。

# 配置说明

| 名称 | 数据类型 | 填写要求 | 默认值 | 描述 |
| :--- | :--- | :--- | :--- | :--- |
| `routerID` | string | 必填 | - | 在路由服务中定义的业务 ID |
| `serviceName` | string | 必填 | - | 目标路由服务在 Higress 中对应的服务名称 |
| `servicePort` | int | 选填 | 80 | 目标路由服务的端口 |
| `serviceHost` | string | 选填 | 匹配 serviceName | 发起请求时的 Host 请求头（`:authority`） |
| `servicePath` | string | 选填 | /v1/smart-route | 智能路由接口的请求路径 |
| `requestJsonPath` | string | 选填 | `messages.@reverse.0.content` | 从原始请求体中提取 Prompt 的 JSON 路径 |
| `timeout` | int | 选填 | 5000 | 调用路由 API 的超时时间（毫秒） |

## 配置示例

```yaml
routerID: "platform_test"
serviceName: "llm-router-service.default.svc.cluster.local"
servicePort: 80
serviceHost: "llm-router.example.com"
servicePath: "/v1/smart-route"
timeout: 3000
```

# 工作原理

1. **提取提示词**：插件在 `ProcessRequestBody` 阶段运行，默认从 `messages` 数组的最后一条消息中提取 `content`。
2. **远程决策**：将提示词和 `routerID` 发送至配置的路由服务。
3. **模型替换**：解析 API 响应，获取 `selected_model`。
4. **请求修改**：使用新的模型名称替换请求体中的 `model` 参数。
5. **错误处理**：若 API 调用失败、超时或返回异常，插件将**立即响应 500 错误并终止流程**。

# 注意事项

- 必须确保 `serviceName` 及其对应的服务在 Higress 的服务列表中已正确配置。
- 默认的 `requestJsonPath` 适配标准的 OpenAI 聊天格式。
