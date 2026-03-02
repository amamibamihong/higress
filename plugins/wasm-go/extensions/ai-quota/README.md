---
title: AI 配额管理
keywords: [ AI网关, AI配额 ]
description: AI 配额管理插件配置参考
---

## 功能说明

`ai-quota` 插件实现给特定 consumer 根据分配固定的 quota 进行 quota 策略限流，同时支持 quota 管理能力，包括查询 quota 、刷新 quota、增减 quota。

`ai-quota` 插件需要配合 认证插件比如 `key-auth`、`jwt-auth` 等插件获取认证身份的 consumer 名称，同时需要配合 `ai-statistics` 插件获取 AI Token 统计信息。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`750`

## 配置说明

| 名称                 | 数据类型            | 填写要求                                 | 默认值 | 描述                                         |
|--------------------|-----------------|--------------------------------------| ---- |--------------------------------------------|
| `redis_key_prefix` | string          |  选填                                     |   chat_quota:   | qutoa redis key 前缀                         |
| `admin_consumer`   | string          | 必填                                   |      | 管理 quota 管理身份的 consumer 名称                 |
| `admin_path`       | string          | 选填                                   |   /quota   | 管理 quota 请求 path 前缀                        |
| `redis`            | object          | 是                                    |      | redis相关配置                                  |

`redis`中每一项的配置字段说明

| 配置项       | 类型   | 必填 | 默认值                                                     | 说明                                                                                         |
| ------------ | ------ | ---- | ---------------------------------------------------------- | ---------------------------                                                                  |
| service_name | string | 必填 | -                                                          | redis 服务名称，带服务类型的完整 FQDN 名称，例如 my-redis.dns、redis.my-ns.svc.cluster.local |
| service_port | int    | 否   | 服务类型为固定地址（static service）默认值为80，其他为6379 | 输入redis服务的服务端口                                                                      |
| username     | string | 否   | -                                                          | redis用户名                                                                                  |
| password     | string | 否   | -                                                          | redis密码                                                                                    |
| timeout      | int    | 否   | 1000                                                       | redis连接超时时间，单位毫秒                                                                  |
| database     | int    | 否   | 0                                                          | 使用的数据库id，例如配置为1，对应`SELECT 1`                                                  |


## 配置示例

### 识别请求参数 apikey，进行区别限流
```yaml
redis_key_prefix: "chat_quota:"
admin_consumer: consumer3
admin_path: /quota
redis:
  service_name: redis-service.default.svc.cluster.local
  service_port: 6379
  timeout: 2000
```


###  刷新 quota

如果当前请求 url 的后缀符合 admin_path，例如插件在 example.com/v1/chat/completions 这个路由上生效，那么更新 quota 可以通过
curl https://example.com/v1/chat/completions/quota/refresh -H "Authorization: Bearer credential3" -d "consumer=consumer1&quota=10000" 

Redis 中 key 为 chat_quota:consumer1 的值就会被刷新为 10000

### 查询 quota

查询特定用户的 quota 可以通过 curl https://example.com/v1/chat/completions/quota?consumer=consumer1 -H "Authorization: Bearer credential3"
将返回： {"quota": 10000, "consumer": "consumer1"}

### 增减 quota

增减特定用户的 quota 可以通过 curl https://example.com/v1/chat/completions/quota/delta -d "consumer=consumer1&value=100" -H "Authorization: Bearer credential3"
这样 Redis 中 Key 为 chat_quota:consumer1 的值就会增加100，可以支持负数，则减去对应值。

## 设计逻辑

### 架构概述

ai-quota 插件是一个基于 Proxy-WASM 的网关插件，用于对 AI 请求进行配额管理。该插件采用 **请求前检查 + 响应后扣减** 的设计模式，确保每个 consumer 的 token 使用量在配额范围内。

### 整体工作流程

```mermaid
graph LR
    A[客户端请求] --> B[认证插件]
    B --> C[设置x-mse-consumer请求头]
    C --> D[请求头阶段]
    D --> E{请求类型判断}
    E -->|聊天请求| F[查询配额]
    E -->|管理请求| G[执行管理操作]
    F --> H{配额检查}
    H -->|配额不足| I[返回403]
    H -->|配额充足| J[转发请求]
    J --> K[后端处理]
    K --> L[响应体阶段]
    L --> M[解析token使用量]
    M --> N[扣减配额]
    N --> O[返回响应]
    G --> O
```

### 请求头阶段处理流程

```mermaid
graph TD
    A[开始] --> B[获取x-mse-consumer请求头]
    B --> C{consumer是否存在}
    C -->|否| D[返回401未认证]
    C -->|是| E[解析请求路径]
    E --> F{判断操作模式}
    F -->|/v1/chat/completions| G[聊天请求模式]
    F -->|/v1/chat/completions/quota| H[查询配额模式]
    F -->|/v1/chat/completions/quota/refresh| I[刷新配额模式]
    F -->|/v1/chat/completions/quota/delta| J[增减配额模式]
    F -->|其他路径| K[直接通过]
    G --> L[从Redis查询配额]
    L --> M{配额是否>0}
    M -->|否| N[返回403配额不足]
    M -->|是| O[继续处理请求]
    H --> P[检查是否为管理员consumer]
    I --> Q[缓冲请求体]
    J --> Q
    P --> R{是否为管理员}
    R -->|否| S[返回403无权限]
    R -->|是| T[执行查询操作]
```

**关键设计点：**
- **异步查询**：使用 Redis 异步查询，避免阻塞请求处理
- **提前拦截**：在请求转发前就检查配额，避免无效请求消耗后端资源
- **路径路由**：通过 URL 后缀区分不同操作类型

### 请求体阶段处理流程（管理请求）

```mermaid
graph TD
    A[开始] --> B{管理模式判断}
    B -->|刷新配额| C[解析请求体参数]
    B -->|增减配额| C
    C --> D[获取consumer和quota/value]
    D --> E[参数校验]
    E --> F{校验通过}
    F -->|否| G[返回400参数错误]
    F -->|是| H{操作类型}
    H -->|刷新| I[执行Redis SET操作]
    H -->|增减| J{value符号}
    J -->|正数| K[执行Redis INCRBY]
    J -->|负数| L[执行Redis DECRBY]
    I --> M[返回200成功]
    K --> M
    L --> M
```

### 响应体阶段处理流程（聊天请求）

```mermaid
graph TD
    A[响应体数据到达] --> B{是否流式响应结束}
    B -->|否| C[解析当前数据块]
    C --> D[提取token使用信息]
    D --> E[累计input和output token]
    E --> F[透传数据给客户端]
    B -->|是| G[计算总token数]
    G --> H[Redis DECRBY扣减配额]
    H --> I[完成响应]
```

**关键设计点：**
- **流式处理**：支持流式响应的实时 token 统计
- **精确扣减**：根据实际的 input + output token 总量进行扣减
- **依赖 ai-statistics**：依赖 ai-statistics 插件提供的 token 解析能力

### 操作模式定义

插件通过 URL 路径后缀识别不同操作：

| 操作类型 | URL 路径 | 描述 |
|---------|----------|------|
| 聊天请求 | `/v1/chat/completions` | AI 聊天请求，需要配额检查和扣减 |
| 查询配额 | `/v1/chat/completions/quota?consumer=xxx` | 查询指定 consumer 的配额 |
| 刷新配额 | `/v1/chat/completions/quota/refresh` | 刷新指定 consumer 的配额 |
| 增减配额 | `/v1/chat/completions/quota/delta` | 增减指定 consumer 的配额 |

### 数据模型

#### Redis Key 设计
- **格式**: `{redis_key_prefix}{consumer}`
- **示例**: `chat_quota:consumer1`
- **值类型**: 整数
- **单位**: token
- **含义**: 剩余可用配额

#### 配额检查逻辑
- 配额不存在：拒绝请求
- 配额 ≤ 0：拒绝请求
- 配额 > 0：允许请求，响应后扣减实际使用量

#### 配额扣除逻辑

配额扣除在响应体阶段进行，具体流程如下：

```mermaid
graph TD
    A[响应体数据到达] --> B{是否有token统计信息}
    B -->|否| C[透传数据]
    B -->|是| D[累计input token数量]
    D --> E[累计output token数量]
    E --> F{流式响应是否结束}
    F -->|否| C
    F -->|是| G[计算总token数]
    G --> H[总token = input + output]
    H --> I[执行Redis DECRBY操作]
    I --> J[配额 = 配额 - 总token]
    J --> K[完成响应]
```

**扣除时机**：
- AI 响应完全结束后（endOfStream = true）
- 在 ai-statistics 插件统计到 token 使用量后

**扣除计算公式**：
```
扣除数量 = input_tokens + output_tokens
新配额 = 当前配额 - 扣除数量
```

**扣除操作**：
- 使用 Redis 的 `DECRBY` 命令原子性地减少配额
- 扣减操作在后台异步执行，不影响响应返回给客户端

**边界情况处理**：
- 如果配额不足扣减，Redis 会将配额减为负数
- 配额为负数时，后续请求会被配额检查逻辑拒绝
- 扣减失败不影响当前请求，但可能导致配额数据不准确

### 安全机制

1. **身份验证**：依赖认证插件（key-auth、jwt-auth 等）确保请求来自合法 consumer
2. **权限控制**：管理接口仅允许配置的 `admin_consumer` 访问
3. **参数校验**：对所有管理接口的参数进行严格校验

### 插件协同

| 插件 | 职责 | 执行顺序 |
|------|------|---------|
| key-auth / jwt-auth | 身份认证，设置 `x-mse-consumer` | 优先级最高 (300) |
| ai-quota | 配额管理，检查和扣减 | 优先级中 (280) |
| ai-statistics | Token 统计，解析响应体 | 优先级低 (250) |

### 性能优化

1. **异步 Redis 操作**：所有 Redis 操作都是异步的，不阻塞请求处理
2. **请求体按需读取**：仅管理请求需要读取请求体，聊天请求直接跳过
3. **流式响应处理**：实时解析响应，无需等待完整响应

### 扩展性考虑

1. **可配置的 Redis 前缀**：支持多租户隔离
2. **可配置的管理路径**：避免与业务路径冲突
3. **支持 Redis 集群**：支持大规模部署场景

## 限制与改进方向

### 待改进

#### Redis 持久化问题

**问题描述**

使用 Redis 存储配额数据存在持久化风险。在程序崩溃、突然断电等异常情况下，刚修改的配额数据可能还未持久化到磁盘就丢失了。

**具体影响场景**

1. **配额消耗记录丢失**
   - 影响范围：丢失最近期间未持久化的配额消耗记录
   - 可接受程度：这种情况相对可以接受
   - 原因：只是丢失了使用历史记录，不影响当前的配额控制功能，配额检查仍然正常工作

2. **配额调整操作失败**
   - 影响范围：在执行刷新（refresh）或增减（delta）配额操作后崩溃，导致调整失败
   - 后果：可能导致实际配额与预期不符
   - 风险：如果管理员刚为某个 consumer 刷新了大量配额后崩溃，该 consumer 可能仍被限制无法使用

**权衡考虑**

- **当前策略**：采用 Redis 默认持久化配置（通常为 RDB 快照 + AOF），在性能和数据安全之间平衡
- **强持久化方案**：可以调整为每条指令都落盘（AOF appendfsync=always），保证数据不丢失，但会导致 Redis 性能大幅下降
- **建议**：根据业务场景评估风险
  - 对于非关键业务，当前策略即可
  - 对于关键业务，可以考虑使用 appendfsync=everysec，并在应用层增加重试机制
  - 对于极高可靠性要求，需要考虑使用数据库等其他持久化方案

### 功能缺陷

#### 计费模式限制

**当前实现**

```
配额粒度：路由 + Consumer
配额类型：Token 总数（input + output）
```

- 支持通过 `redis_key_prefix` 配置实现按路由 + Consumer 维度设置配额
- 只支持简单的 token 总数配额控制，不区分请求和响应
- 所有请求使用统一的计费标准

**实际业务需求**

在实际 AI 服务中，计费模式更加复杂和多样化：

1. **输入输出计费差异**
   - 大部分模型厂商的输入和输出价格不同
     - 有的模型输入输出价格相同
     - 有的模型输出价格比输入更高（如 GPT-4）
     - 有的模型只对输出计费，或者只对输入计费
   
2. **多种计费维度**
   - `cache_read_input_token`：缓存读取的输入 token（如 Claude 的缓存功能）
   - `input_cost_per_image`：图片输入按张计费（如 GPT-4 Vision）
   - `input_cost_per_audio`：音频输入按时长或大小计费（如 Whisper）
   - `input_cost_per_token_above_200k_tokens`：阶梯式计价（长文本输入超过阈值后价格变化）
   - `tool_use_tokens`：工具调用 token
   - 当前设计只支持简单的 input_token + output_token，无法处理这些复杂场景

3. **按模型差异化定价**

   - 不同模型的算力资源消耗差异巨大
   - 同样消耗 1000 tokens，GPT-4 的成本可能比 GPT-3.5 高 10 倍以上
   - 如果一个 consumer 的配额同时用于多个模型，无法公平反映真实成本
   - 无法针对不同模型设置不同的配额策略

   **场景举例**：
   ```
   Consumer A 配额：100,000 tokens
   
   场景1：使用 GPT-3.5
   - 实际成本：$0.20
   
   场景2：使用 GPT-4
   - 实际成本：$2.00
   
   相同的 token 消耗，成本相差 10 倍
   ```

**改进方案建议**

针对计费模式限制问题，提供以下两种改进方案：

#### 方案一：预算制（按金额计费）

**核心思路**

每个 Consumer 设置一个总预算（金额），不同模型配置各自的计价规则，使用时根据模型计价规则扣除金额。

**数据模型**

```yaml
Consumer 配置：
  budget: 1000.00              # 预算金额（美元/人民币等）
  currency: "CNY"             # 货币单位

模型计价配置：
  gpt-4o:
    input_price_per_1m: 10    # 输入价格：每 100 万 tokens 10 元
    output_price_per_1m: 30   # 输出价格：每 100 万 tokens 30 元
    cache_read_price_per_1m: 1 # 缓存读取价格
    image_price_per_image: 0.1 # 图片计价

  gpt-3.5-turbo:
    input_price_per_1m: 1
    output_price_per_1m: 2

  glm-4.7:
    input_price_per_1m: 5
    output_price_per_1m: 15
```

**扣除逻辑**

```
扣除金额 = (input_tokens × input_price_per_1m ÷ 1000000)
        + (output_tokens × output_price_per_1m ÷ 1000000)
        + 其他计费项

剩余预算 = 当前预算 - 扣除金额
```

Redis Key 设计：
```
consumer_budget:{consumer}        # 剩余预算金额
consumer_model_history:{consumer} # 使用历史记录（可选）
```

**适用场景**

- 多模型统一预算管理
- 按实际成本控制
- 需要精确的成本核算
- 支持复杂计费规则

**优势**

- 灵活支持各模型的不同计价策略
- 直接反映真实成本
- 易于与财务系统集成

**挑战**

- 需要维护各模型的计价配置
- 价格调整时需要同步更新配置
- 货币汇率处理（如涉及多币种）

#### 方案二：模型配额制（按模型分配）

**核心思路**

为每个模型设置独立的配额，不同模型之间配额相互独立。Consumer 使用某个模型时，只扣除该模型的配额，不影响其他模型。

**数据模型**

```yaml
Consumer 配置：
  model_quotas:
    glm-4.7:
      input_quota: 1000000    # 100 万 input tokens
      output_quota: 500000    # 50 万 output tokens

    glm-4.7-flash:
      input_quota: 10000000   # 1000 万 input tokens
      output_quota: 5000000   # 500 万 output tokens

    gpt-4o:
      input_quota: 100000
      output_quota: 50000
```

Redis Key 设计：

```
quota:{consumer}:{model}:input    # 某个模型的 input 配额
quota:{consumer}:{model}:output   # 某个模型的 output 配额
```

**检查和扣除逻辑**

```
请求到达模型 Model_X：
1. 从 Redis 获取 Model_X 的配额
2. 检查 input 配额和 output 配额
3. 任一不足则拒绝请求
4. 请求完成后，扣除对应的 input 和 output 配额
```

**适用场景**

- 不同模型有不同的限流策略
- 免费模型配额多，付费模型配额少
- 按产品线或业务类型分配模型资源
- 模型供应商的直接对接场景
