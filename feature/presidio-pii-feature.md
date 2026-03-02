# Feature: Presidio PII 检测与保护插件

## 一、功能需求描述

### 1.1 背景
随着AI应用的普及，用户在与AI模型交互时可能会输入包含个人身份信息（PII）的内容，如姓名、邮箱、电话、银行卡号等。同时，AI模型的响应也可能包含敏感的PII信息。为了保护用户隐私，需要实现一个可配置的PII检测和屏蔽插件。

### 1.2 目标
开发一个基于Presidio的Wasm插件，实现对AI应用请求和响应中PII的自动检测和屏蔽，支持多种处理策略，并提供灵活的配置选项。

### 1.3 核心功能需求

#### 1.3.1 PII检测功能
- 支持对接Presidio Analyzer服务，对文本进行PII检测
- 支持多种PII实体类型（PERSON、EMAIL_ADDRESS、PHONE_NUMBER、CREDIT_CARD等）
- 支持配置检测的语言（中文、英文等）
- 支持配置实体级别的置信度阈值（scoreThreshold）
- 支持全局默认置信度阈值配置

#### 1.3.2 PII处理策略
- **MASK（屏蔽）**：使用指定的匿名化方式替换PII
  - 支持默认mask操作（<ENTITY_TYPE>格式）
  - 支持实体级别的自定义匿名化配置：
    - mask：使用指定字符掩码
    - replace：使用自定义文本替换
    - redact：完全删除PII
    - encrypt：加密PII
    - hash：哈希PII
- **BLOCK（阻断）**：直接拒绝包含敏感PII的请求/响应
- **NONE（忽略）**：不处理该类型的PII

#### 1.3.3 双向检测
- 支持检测用户→模型的请求输入（request）
- 支持检测模型→用户的响应输出（response）
- 支持同时检测两个方向（both）

#### 1.3.4 流式响应处理
- 支持流式响应的PII检测和屏蔽
- 支持缓冲多个chunk后进行批量检测
- 支持配置缓冲区大小限制（bufferLimit）
- 检测到PII后进行匿名化并替换流式响应内容

#### 1.3.5 协议兼容性
- 默认支持OpenAI兼容协议
- 支持自定义JSON路径配置，可适配非OpenAI协议
- 默认配置路径：
  - 请求：`messages.@reverse.0.content`
  - 响应：`choices.0.message.content`
  - 流式响应：`choices.0.delta.content`

#### 1.3.6 服务发现
- 支持DNS类型服务（域名访问）
- 支持固定地址类型服务（IP访问）
- 支持配置analyzer和anonymizer两个独立服务

#### 1.3.7 可观测性
- 提供监控指标：
  - `presidio_pii_request_deny`：请求阻断数量
  - `presidio_pii_response_deny`：响应阻断数量
- 提供日志属性：
  - `presidio_pii_request_rt`：请求检测耗时
  - `presidio_pii_response_rt`：响应检测耗时
  - `presidio_pii_status`：PII检测状态
  - `presidio_pii_masked_count`：屏蔽的实体数量
  - `presidio_pii_blocked_entities`：被阻断的实体类型列表

### 1.4 配置需求

#### 基础配置项：
- `analyzerServiceName`：Analyzer服务名
- `analyzerServicePort`：Analyzer服务端口
- `analyzerServiceHost`：Analyzer服务地址（域名或IP）
- `analyzerPath`：Analyzer API路径
- `anonymizerServiceName`：Anonymizer服务名
- `anonymizerServicePort`：Anonymizer服务端口
- `anonymizerServiceHost`：Anonymizer服务地址（域名或IP）
- `anonymizerPath`：Anonymizer API路径

#### 功能配置项：
- `checkRequest`：是否检测请求
- `checkResponse`：是否检测响应
- `filterScope`：检测范围（input/output/both）
- `language`：检测语言
- `defaultAction`：默认操作（MASK/BLOCK/NONE）
- `defaultScoreThreshold`：默认置信度阈值

#### 高级配置项：
- `requestContentJsonPath`：请求内容JSON路径
- `responseContentJsonPath`：响应内容JSON路径
- `responseStreamContentJsonPath`：流式响应内容JSON路径
- `protocol`：协议类型（openai/original）
- `timeout`：调用Presidio服务的超时时间
- `bufferLimit`：流式响应缓冲限制
- `denyCode`：阻断响应状态码
- `denyMessage`：阻断响应消息

#### 实体配置项：
- `entities`：实体类型配置列表
- `entities[].entityType`：实体类型
- `entities[].action`：该实体的操作类型
- `entities[].scoreThreshold`：该实体的置信度阈值
- `entities[].anonymizer`：实体级别的匿名化配置

## 二、验收标准

### 2.1 功能验收

#### 验收标准2.1.1：PII检测
- [ ] 能够正确检测文本中的多种PII类型（PERSON、EMAIL_ADDRESS、PHONE_NUMBER等）
- [ ] 检测结果包含实体类型、位置、置信度等信息
- [ ] 支持配置实体级别的置信度阈值
- [ ] 支持全局默认置信度阈值
- [ ] 支持配置检测语言（中文、英文）

#### 验收标准2.1.2：PII屏蔽
- [ ] 能够正确对检测到的PII进行MASK操作
- [ ] 默认使用<ENTITY_TYPE>格式进行屏蔽
- [ ] 支持实体级别的自定义匿名化配置
- [ ] 支持多种匿名化方式（mask、replace、redact、encrypt、hash）

#### 验收标准2.1.3：PII阻断
- [ ] 能够正确对检测到的敏感PII进行BLOCK操作
- [ ] 阻断时返回配置的响应状态码和消息
- [ ] 阻断请求后，原始请求不会转发到后端
- [ ] 阻断响应后，原始响应不会返回给客户端

#### 验收标准2.1.4：双向检测
- [ ] 支持仅检测请求（filterScope=input）
- [ ] 支持仅检测响应（filterScope=output）
- [ ] 支持同时检测请求和响应（filterScope=both）
- [ ] 请求检测在请求发送到后端前完成
- [ ] 响应检测在响应返回给客户端前完成

#### 验收标准2.1.5：流式响应处理
- [ ] 能够正确处理流式响应（SSE格式）
- [ ] 能够正确解析流式响应中的content字段
- [ ] 支持缓冲多个chunk后进行批量检测
- [ ] 检测到PII后能够正确进行匿名化
- [ ] 能够正确将匿名化后的内容分布到流式响应中
- [ ] 支持配置缓冲区大小限制（bufferLimit）

#### 验收标准2.1.6：协议兼容性
- [ ] 默认支持OpenAI协议格式
- [ ] 支持自定义JSON路径配置
- [ ] 能够正确解析OpenAI请求格式
- [ ] 能够正确解析OpenAI响应格式
- [ ] 能够正确解析OpenAI流式响应格式

#### 验收标准2.1.7：服务发现
- [ ] 支持DNS类型服务配置
- [ ] 支持固定地址类型服务配置
- [ ] 能够正确调用analyzer服务
- [ ] 能够正确调用anonymizer服务
- [ ] 支持配置服务端口和Host头

### 2.2 性能验收

#### 验收标准2.2.1：响应延迟
- [ ] 非流式响应的PII检测延迟在合理范围内
- [ ] 流式响应的处理延迟在合理范围内

#### 验收标准2.2.2：并发处理
- [ ] 支持多个请求并发处理
- [ ] 每个请求的处理相互独立
- [ ] 不会因为单个请求的处理阻塞其他请求

#### 验收标准2.2.3：资源使用
- [ ] 内存使用在合理范围内
- [ ] 流式响应的缓冲区大小可控（通过bufferLimit配置）
- [ ] 长时间运行不会出现内存泄漏

### 2.3 可靠性验收

#### 验收标准2.3.1：错误处理
- [ ] Presidio服务调用失败时有适当的错误处理
- [ ] 超时情况下有合理的处理策略
- [ ] JSON解析失败时有适当的错误日志
- [ ] 配置错误时能够给出明确的错误提示

#### 验收标准2.3.2：边界情况
- [ ] 空文本能够正常处理
- [ ] 不包含PII的文本能够正常通过
- [ ] PII位于文本开头/结尾时能够正确识别
- [ ] 多个PII相邻时能够正确识别

#### 验收标准2.3.3：数据完整性
- [ ] 非PII内容不会被误修改
- [ ] JSON结构不会被破坏
- [ ] 请求/响应的其他字段不会被修改
- [ ] 上下文信息（如消息ID、时间戳等）保持不变

### 2.4 可观测性验收

#### 验收标准2.4.1：监控指标
- [ ] `presidio_pii_request_deny`指标正确统计请求阻断数量
- [ ] `presidio_pii_response_deny`指标正确统计响应阻断数量
- [ ] 指标数据能够被Prometheus等监控系统采集

#### 验收标准2.4.2：日志记录
- [ ] PII检测耗时正确记录到日志
- [ ] PII检测状态正确记录到日志
- [ ] 屏蔽的实体数量正确记录到日志
- [ ] 被阻断的实体类型正确记录到日志
- [ ] 错误情况有详细的错误日志

### 2.5 配置验收

#### 验收标准2.5.1：配置解析
- [ ] 支持YAML格式配置
- [ ] 配置解析错误时给出明确的错误提示
- [ ] 必填配置项缺失时能够检测并报错
- [ ] 配置项类型错误时能够检测并报错

#### 验收标准2.5.2：配置验证
- [ ] analyzer服务配置能够正确验证
- [ ] anonymizer服务配置能够正确验证
- [ ] 实体类型配置能够正确验证
- [ ] 操作类型配置能够正确验证

### 2.6 文档验收

#### 验收标准2.6.1：用户文档
- [ ] README包含完整的功能说明
- [ ] README包含配置项说明
- [ ] README包含配置示例
- [ ] README包含实体类型列表
- [ ] README包含Presidio服务端点要求说明

#### 验收标准2.6.2：示例配置
- [ ] example-config.yaml包含详细的配置示例
- [ ] 配置示例包含中文注释
- [ ] 配置示例覆盖常见使用场景
- [ ] 配置示例包含高级配置说明

### 2.7 测试验收

#### 验收标准2.7.1：单元测试
- [ ] 配置解析有完整的单元测试
- [ ] 工具函数有完整的单元测试
- [ ] 单元测试覆盖率 > 80%

#### 验收标准2.7.2：集成测试
- [ ] 有完整的端到端测试场景
- [ ] 测试覆盖主要功能路径
- [ ] 测试覆盖边界情况

## 三、技术实现要点

### 3.1 技术栈
- 编程语言：Go
- Wasm框架：higress-wasm-go-plugin
- PII检测：Presidio Analyzer
- PII匿名化：Presidio Anonymizer
- JSON处理：gjson/sjson

### 3.2 关键技术难点

#### 3.2.1 流式响应处理
- 难点：流式输出逐词、逐字的特点导致PII可能被分割到多个chunk中
- 解决方案：缓冲多个chunk后进行批量检测，检测到PII后进行匿名化并替换内容

#### 3.2.2 JSON内容替换
- 难点：需要精确定位和替换JSON中的特定字段，同时保持JSON结构完整性
- 解决方案：使用sjson库进行精确的字段替换

#### 3.2.3 匿名化后长度变化
- 难点：匿名化后的文本长度与原文不同，导致流式响应chunk分布困难
- 解决方案：按照原始chunk的长度分布匿名化后的文本

### 3.3 架构设计

#### 插件架构：
```
main.go (插件入口)
  ├── onHttpRequestHeaders
  ├── onHttpRequestBody
  ├── onHttpResponseHeaders
  ├── onHttpResponseBody
  └── onHttpStreamingResponseBody

handler/handler.go (业务逻辑)
  ├── HandleRequestBody
  ├── HandleResponseBody
  ├── HandleStreamingResponseBody
  ├── sendDenyResponse
  ├── sendStreamingDenyResponse
  ├── handleAnonymize
  ├── handleStreamingAnonymize
  ├── getBlockedEntities
  └── getMaskedEntities

config/config.go (配置和类型定义)
  ├── PresidioPIIConfig
  ├── AnalyzeRequest/Response
  ├── AnonymizeRequest/Response
  └── PIIEntity
```

## 四、已知限制和注意事项

1. **流式响应的处理限制**：由于流式输出逐词、逐字的特点，PII检测可能在部分chunk中不准确，匿名化后的文本长度变化也可能导致内容分布不完美

2. **性能影响**：PII检测需要调用外部服务，会增加请求/响应的延迟，建议根据实际业务需求配置合理的超时时间

3. **内存使用**：流式响应需要缓冲chunk，大响应可能占用较多内存，建议通过bufferLimit配置合理限制

4. **检测准确性**：PII检测的准确性取决于Presidio的识别器和语言模型，某些情况下可能出现误检或漏检

5. **服务依赖**：插件依赖外部的Presidio Analyzer和Anonymizer服务，需要确保这些服务的可用性

## 五、后续优化建议

1. 支持本地缓存PII检测结果，减少对Presidio服务的调用
2. 支持自定义识别器和语言模型
3. 支持更复杂的匿名化策略，如部分掩码、模糊化等
4. 支持基于用户/租户的个性化配置
5. 支持PII检测结果的审计和追溯
6. 优化流式响应的处理性能，减少延迟
7. 支持更多的协议格式和消息结构

## 六、实施进度

### 已完成
- [x] 插件基础架构搭建
- [x] PII检测功能实现（对接Presidio Analyzer）
- [x] PII屏蔽功能实现（对接Presidio Anonymizer）
- [x] PII阻断功能实现
- [x] 双向检测支持（请求/响应）
- [x] OpenAI协议兼容
- [x] 非流式响应处理
- [x] 流式响应处理
- [x] 实体级别配置支持
- [x] 服务发现（DNS/固定地址）
- [x] 监控指标支持
- [x] 日志属性支持
- [x] 配置解析和验证
- [x] 示例配置文件
- [x] 用户文档（README）

### 待完成
- [ ] 单元测试完善（覆盖率 > 80%）
- [ ] 集成测试
- [ ] 性能测试和优化
- [ ] 边界情况测试
- [ ] 错误处理测试
- [ ] 文档完善
