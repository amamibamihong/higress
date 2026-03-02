# Feature: AI Statistics 插件增强分布式追踪属性

## 一、功能需求描述

### 1.1 背景
在AI应用的分布式追踪场景中，除了基础的AI指标（如模型名称、token使用量、请求耗时等）之外，业务场景往往需要追踪更多的上下文信息。特别是会话标识（SessionID）和消费者标识（ConsumerKey）这两个字段，在以下场景中非常重要：

- **用户行为分析**：通过SessionID可以关联同一用户的多次请求，分析会话级别的行为模式
- **问题排查**：当发现异常时，可以通过SessionID快速定位相关的所有请求链路
- **消费统计**：通过ConsumerKey可以进行按消费者维度的调用统计和计费
- **安全审计**：可以追踪特定消费者的请求记录，用于安全审计和合规检查
- **性能优化**：分析不同ConsumerKey的请求性能差异，优化资源分配

当前 ai-statistics 插件已经能够提取 SessionID 和 ConsumerKey，但仅存储在日志和上下文中，并未输出到分布式追踪（Span）中。这导致在 APM 系统（如 ARMS、Skywalking 等）中无法通过这些字段进行过滤和检索。

### 1.2 目标
在 ai-statistics 插件中，将 SessionID 和 ConsumerKey 这两个关键字段输出到分布式追踪的 Span 属性中，使其在 APM 系统中可见和可查询。

### 1.3 核心功能需求

#### 1.3.1 ConsumerKey 追踪
- 从请求头 `x-mse-consumer` 中提取消费者标识
- 将 ConsumerKey 输出到 Span 属性中
- 使用常量 `ConsumerKey` 作为 Span attribute 的 key（值为 "consumer"）

#### 1.3.2 SessionID 追踪
- 按优先级从以下请求头中提取会话标识：
  - `x-openclaw-session-key`
  - `x-clawdbot-session-key`
  - `x-moltbot-session-key`
  - `x-agent-session`
  - 支持自定义请求头配置
- 将 SessionID 输出到 Span 属性中
- 使用常量 `SessionID` 作为 Span attribute 的 key（值为 "session_id"）

#### 1.3.3 Span 属性命名
- Span attribute key 使用代码中已有的常量，保持一致性
- 与现有的 ARMS 风格属性（如 `gen_ai.span.kind`、`gen_ai.model_name` 等）保持区分
- 便于后续维护和统一管理

### 1.4 技术实现要点

#### 1.4.1 提取时机
- ConsumerKey 和 SessionID 的提取在 `onHttpRequestHeaders` 阶段完成
- 提取成功后立即调用 `setSpanAttribute()` 将值写入 Span

#### 1.4.2 现有常量复用
- 使用已定义的常量 `ConsumerKey`（值为 "x-mse-consumer"）作为请求头提取
- 使用已定义的常量 `SessionID`（值为 "session_id"）作为会话标识
- Span attribute key 直接使用这些常量，避免新的常量定义

#### 1.4.3 条件设置
- ConsumerKey：仅当请求头中存在该字段且值非空时才设置
- SessionID：仅当成功提取到 session ID 且值非空时才设置

## 二、验收标准

### 2.1 功能验收

#### 验收标准2.1.1：ConsumerKey 追踪
- [ ] 能够从请求头 `x-mse-consumer` 中正确提取消费者标识
- [ ] 提取到的 ConsumerKey 值正确输出到 Span 属性中
- [ ] Span attribute key 为 "consumer"
- [ ] 当请求中不包含该头时，不影响正常处理
- [ ] 当请求中该头为空时，不设置 Span 属性

#### 验收标准2.1.2：SessionID 追踪
- [ ] 能够按优先级从多个请求头中提取会话标识
- [ ] 默认支持 `x-openclaw-session-key`、`x-clawdbot-session-key`、`x-moltbot-session-key`、`x-agent-session`
- [ ] 支持自定义请求头配置
- [ ] 提取到的 SessionID 值正确输出到 Span 属性中
- [ ] Span attribute key 为 "session_id"
- [ ] 当所有请求头都不包含 session ID 时，不影响正常处理
- [ ] 当提取到的值为空时，不设置 Span 属性

#### 验收标准2.1.3：Span 属性可见性
- [ ] APM 系统（如 ARMS、Skywalking）中能够查看到 "consumer" 属性
- [ ] APM 系统中能够查看到 "session_id" 属性
- [ ] 能够通过这些属性进行 trace 过滤和检索
- [ ] 这些属性与已有的 `gen_ai.*` 属性一起显示

### 2.2 兼容性验收

#### 验收标准2.2.1：向后兼容
- [ ] 不影响现有的 Span 属性输出
- [ ] 不影响现有的日志输出
- [ ] 不影响现有的请求处理逻辑
- [ ] 不造成性能显著下降

#### 验收标准2.2.2：配置兼容
- [ ] 无需额外配置即可启用该功能
- [ ] SessionID 的自定义请求头配置保持原有机制
- [ ] 现有的 ai-statistics 配置依然生效

### 2.3 性能验收

#### 验收标准2.3.1：性能影响
- [ ] 添加的 Span 属性设置对请求延迟影响可忽略（< 1ms）
- [ ] 不增加额外的网络调用
- [ ] 不增加额外的内存占用

### 2.4 代码质量验收

#### 验收标准2.4.1：代码规范
- [ ] 使用现有的常量定义，未引入冗余常量
- [ ] 代码风格与现有代码保持一致
- [ ] 遵循 Go 语言编码规范

## 三、技术实现要点

### 3.1 改动说明

#### 3.1.1 修改文件
- `/Users/zhangshang/git/higress/plugins/wasm-go/extensions/ai-statistics/main.go`

#### 3.1.2 代码改动
在 `onHttpRequestHeaders` 函数中：

```go
// ConsumerKey 提取和 Span 属性设置
if consumer, _ := proxywasm.GetHttpRequestHeader(ConsumerKey); consumer != "" {
    ctx.SetContext(ConsumerKey, consumer)
    setSpanAttribute(ConsumerKey, consumer)  // 新增
}

// SessionID 提取和 Span 属性设置
sessionId := extractSessionId(config.sessionIdHeader)
if sessionId != "" {
    ctx.SetUserAttribute(SessionID, sessionId)
    setSpanAttribute(SessionID, sessionId)  // 新增
}
```

### 3.2 技术栈
- Wasm 框架：higress-wasm-go-plugin
- 追踪集成：proxy-wasm-go-sdk
- 属性设置：通过 `proxywasm.SetProperty` 设置 filter state，由 Envoy 上报到追踪系统

### 3.3 实现机制

#### 3.3.1 `setSpanAttribute` 函数
该函数将 key-value 对设置到 filter state 中，格式为：
```go
traceSpanTag := wrapper.TraceSpanTagPrefix + key
```

Envoy 会自动将这些属性包含在分布式追踪的 Span 中。

#### 3.3.2 `extractSessionId` 函数
该函数按以下优先级提取 SessionID：
1. 如果配置了自定义头，优先使用自定义头
2. 否则按顺序尝试默认头：`x-openclaw-session-key` → `x-clawdbot-session-key` → `x-moltbot-session-key` → `x-agent-session`

## 四、已知限制和注意事项

1. **常量复用**：Span attribute key 使用的常量（`ConsumerKey`、`SessionID`）也用于其他场景（如请求头名、日志属性名），使用时需要理解上下文

2. **头字段值**：请求头值的正确性和合法性由调用方保障，插件不进行额外校验

3. **追踪依赖**：该功能依赖 Envoy 的分布式追踪配置，需要确保已正确配置 TracingProvider（OpenTelemetry、Zipkin 或 Skywalking）

4. **属性可见性**：Span 属性只有在追踪采样命中时才会被收集，取决于采样率的配置

5. **无额外配置**：该功能无需额外配置，只要请求头中包含相应字段即可自动生效

## 五、后续优化建议

1. 支持配置是否启用这两个字段的 Span 属性输出
2. 支持自定义 Span attribute key，以适配不同的追踪系统和语义约定
3. 添加更多的 session 上下文信息到 Span（如 user_id、tenant_id 等）
4. 支持 Span attribute 的值脱敏，避免敏感信息泄露
5. 添加相关的 Prometheus 指标，统计包含这些属性的请求比例

## 六、实施进度

### 已完成
- [x] ConsumerKey 的 Span 属性输出实现
- [x] SessionID 的 Span 属性输出实现
- [x] 使用现有常量，避免冗余定义
- [x] 代码实现和验证

### 待完成
- [ ] 功能测试验证（在 APM 系统中验证属性可见性）
- [ ] 性能测试验证（验证无性能影响）
- [ ] 文档更新（更新插件 README 说明该功能）
