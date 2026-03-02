# Feature: AI Quota 插件预算和费率优化

## 一、功能需求描述

### 1.1 背景
当前 ai-quota 插件采用单一 token 配额管理方式，仅支持按用户预设 token 数量。这种模式存在以下限制：

1. **财务计费不灵活**：实际业务以金额为单位进行计费，token 配额无法直接对应费用
2. **模型差异难支持**：不同模型价格不同（如 GPT-4、Claude-3 等模型费率差异大），统一 token 配额无法区分
3. **动态调价困难**：费率调整需要重新计算所有用户配置
4. **双重控制缺失**：无法同时控制 token 用量和费用上限

为了解决这些问题，需要升级为同时支持 token 预算和费用预算的管理模式，并按模型配置费率。

### 1.2 目标
将 ai-quota 插件升级为支持：
- 按用户设置两种预算：token 预算和费用预算
- 按模型设置费率（输入费率、输出费率）
- 请求前验证剩余 token 和剩余费用，任一不足则拒绝
- 请求完成后同时扣除 token 和费用

### 1.3 核心功能需求

#### 1.3.1 双重预算管理
- 每个用户同时维护两个预算：
  - token 预算：剩余可用 token 数量（整数）
  - 费用预算：剩余可用金额（浮点数，单位：元，精确到分）
- 两种预算都需要：
  - 初始设置（refresh）
  - 查询（query）
  - 增减（delta）
  - 验证（请求前检查）

#### 1.3.2 按模型配置费率
- 支持按模型维度配置费率
- 费率包含：
  - 输入 token 费率（input_rate）：每输入 token 的价格
  - 输出 token 费率（output_rate）：每输出 token 的价格
- 费率单位：元/百万token（如 10 表示每百万 token 成本 10 元）
- 实际计费时转换为：每 token 价格 = 页面设置值 / 1,000,000
  - 如页面设置 10 元/百万token
  - 实际每 token 价格 = 10 / 1,000,000 = 0.00001 元/token
- 支持小费率：如 0.5 元/百万token，实际 = 0.0000005 元/token

#### 1.3.3 请求前验证
- 每次请求前检查用户的两种预算：
  - 剩余 token > 0
  - 剩余费用 > 0
- 任一条件不满足（<= 0）则拒绝请求，返回 403
- 拒绝响应说明原因（无 token 或无费用）

#### 1.3.4 请求后扣费
请求成功后执行：
1. 扣除 token：从用户 token 预算中减去实际使用的总 token
2. 扣除费用：从用户费用预算中减去计算的金额
   - 计算公式：费用 = input_token * input_rate + output_token * output_rate
   - 费率按模型获取（从请求中获取模型名称）
   - 费用精确到小数点后 6 位（百万分之一的精度）

#### 1.3.5 模型识别
- 从请求中提取供应商名称和模型名称
- 供应商识别：根据请求路由或请求头识别 AI 服务供应商（如 OpenAI、Claude、通义千问等）
- 模型识别：从请求体中提取模型名称（model 字段）
- 费率查询优先级：先按 `provider:/{model_name}` 查询，不存在则使用默认费率
- 支持常见供应商：OpenAI、Claude、通义千问、文心一言等

### 1.4 配置需求

#### 全局配置项：
- `redis_key_prefix`：Redis 键前缀
- `admin_consumer`：管理员用户名
- `admin_path`：管理接口路径
- `precision`：费用精度（默认为 6，支持精确到百万分之一）

#### Redis 数据结构：
- `{redis_key_prefix}token_budget:{consumer}`：用户的 token 预算（整数）
- `{redis_key_prefix}cost_budget:{consumer}`：用户的费用预算（浮点数）
- `{redis_key_prefix}rate:default`：默认费率配置（JSON字符串）
  - `input_rate`: 输入费率（元/百万token）
  - `output_rate`: 输出费率（元/百万token）
- `{redis_key_prefix}rate:model:{provider}:{model_name}`：模型费率配置（JSON字符串）
  - `provider`: 供应商名称（如 openai、claude 等）
  - `model_name`: 模型名称（如 gpt-4、claude-3-opus 等）
  - `input_rate`: 输入费率（元/百万token）
  - `output_rate`: 输出费率（元/百万token）

#### API 接口参数：
- 设置预算（refresh）：
  - `token_budget`：token 预算（可选）
  - `cost_budget`：费用预算（可选）
- 查询预算（query）：
  - 返回 `token_budget` 和 `cost_budget` 两个字段
- 增减预算（delta）：
  - `token_budget_delta`：token 增减量（可选，正数表示增加，负数表示减少）
  - `cost_budget_delta`：费用增减量（可选，正数表示增加，负数表示减少）
- 设置费率（setrate）：
  - `provider`：供应商名称（可选，不填表示设置默认费率）
  - `model`：模型名（可选，不填表示设置默认费率）
  - `input_rate`：输入费率（元/百万token）
  - `output_rate`：输出费率（元/百万token）
  - 注意：`provider` 和 `model` 都不填时设置默认费率，都填时设置指定供应商的模型费率
- 查询费率（getrate）：
  - `provider`：供应商名称（可选，不填表示查询默认费率）
  - `model`：模型名（可选，不填表示查询默认费率）
  - 返回 `provider`、`model`、`input_rate`、`output_rate` 字段

## 二、验收标准

### 2.1 功能验收

#### 验收标准2.1.1：双重预算管理
- [ ] 支持为用户设置 token 预算和费用预算
- [ ] 支持查询用户的剩余 token 和费用
- [ ] 支持增减用户的 token 和费用预算
- [ ] 两种预算数据在 Redis 中正确存储和读取

#### 验收标准2.1.2：按模型费率配置
- [ ] 支持配置多种模型的费率
- [ ] 费率单位为元/百万token
- [ ] 费率支持小数（如 0.5、10.5 等）
- [ ] 支持分别配置输入和输出费率
- [ ] 从请求中正确识别模型名称
- [ ] 正确加载对应模型的费率

#### 验收标准2.1.3：请求前验证
- [ ] 请求前检查剩余 token > 0，否则拒绝
- [ ] 请求前检查剩余费用 > 0，否则拒绝
- [ ] 任一预算不足时返回 403 和明确的错误信息
- [ ] 两种预算充足时允许请求通过

#### 验收标准2.1.4：请求后扣费
- [ ] 请求成功后正确扣除使用的 token
- [ ] 请求成功后正确扣除计算的金额
- [ ] 费用计算：input_token * (input_rate/1000000) + output_token * (output_rate/1000000)
- [ ] 扣费前后预算数据正确
- [ ] 扣费操作使用 Redis 保证原子性

#### 验收标准2.1.5：精确计费
- [ ] 支持小费率（如 0.5 元/百万token = 0.0000005 元/token）
- [ ] 费用精度支持到小数点后 6 位
- [ ] 费用计算采用四舍五入，保持精度
- [ ] 边界测试：最小费率、最大费率、大量 token

### 2.2 接口验收

#### 验收标准2.2.1：设置预算（refresh）
- [ ] 支持设置 token_budget 参数
- [ ] 支持设置 cost_budget 参数
- [ ] 支持同时设置两种预算
- [ ] 参数验证正确（非负数）
- [ ] 返回成功响应

#### 验收标准2.2.2：查询预算（query）
- [ ] 支持查询单个用户的预算
- [ ] 返回 token_budget 和 cost_budget 两个字段
- [ ] 金额保持正确的精度
- [ ] 用户不存在时返回 0

#### 验收标准2.2.3：增减预算（delta）
- [ ] 支持对 token_budget 增减
- [ ] 支持对 cost_budget 增减
- [ ] 支持同时增减两种预算
- [ ] 负值表示减少，正值表示增加
- [ ] 参数验证正确

#### 验收标准2.2.4：设置和查询费率（setrate/getrate）
- [ ] 支持设置默认费率
- [ ] 支持设置模型费率
- [ ] 支持查询默认费率
- [ ] 支持查询模型费率
- [ ] 费率正确存储在 Redis 中
- [ ] 费率正确从 Redis 中读取

### 2.3 模型支持验收

#### 验收标准2.3.1：模型识别
- [ ] 从 OpenAI 请求中正确提取 model 字段
- [ ] 支持常见模型识别（gpt-4, gpt-3.5-turbo 等）
- [ ] 支持自定义模型识别
- [ ] 模型未配置费率时使用默认费率

#### 验收标准2.3.2：费率加载
- [ ] 根据模型名称正确加载费率
- [ ] 费率不存在时使用默认费率（可配置）
- [ ] 费率解析正确（字符串转浮点数）

### 2.4 精度验收

#### 验收标准2.4.1：费用精度
- [ ] 费用计算保留 6 位小数
- [ ] 费用存储保持精度
- [ ] 费用返回保持精度
- [ ] 大额交易精度正确

#### 验收标准2.4.2：边界测试
- [ ] token 预算刚好用完时拒绝请求
- [ ] 费用预算刚好用完时拒绝请求
- [ ] 预算不足无法扣费时拒绝请求
- [ ] 最小费率计费正确
- [ ] 最大费率计费正确

### 2.5 性能验收

#### 验收标准2.5.1：响应时间
- [ ] 请求前验证延迟 < 5ms
- [ ] 请求后扣费延迟对用户体验无影响
- [ ] Redis 查询性能符合预期

### 2.6 并发验收

#### 验收标准2.6.1：并发扣费
- [ ] 同一用户的多个并发请求不会导致超支
- [ ] 使用 Redis 事务或 Lua 脚本保证原子性
- [ ] 并发场景下预算数据一致性正确

## 三、配置示例

### 3.1 插件配置示例

```yaml
redis:
  addr: redis.default.svc.cluster.local:6379
  password: ""
  db: 0

redis_key_prefix: "ai_quota:"
admin_consumer: "admin"
admin_path: "/ai/quota"
precision: 6
```

### 3.2 API 调用示例

#### 设置预算（refresh）
```
POST /ai/quota/refresh
consumer=example_user&token_budget=1000000&cost_budget=100.0
```

#### 查询预算（query）
```
GET /ai/quota/query?consumer=example_user
```

响应示例：
```json
{
  "consumer": "example_user",
  "token_budget": 985000,
  "cost_budget": 98.765432
}
```

#### 增减预算（delta）
```
POST /ai/quota/delta
consumer=example_user&token_budget_delta=50000&cost_budget_delta=10.0
```

#### 设置默认费率（setrate）
```
POST /ai/quota/setrate
model=default&input_rate=10.0&output_rate=30.0
```

#### 设置模型费率（setrate）
```
POST /ai/quota/setrate
model=gpt-4&input_rate=30.0&output_rate=60.0
```

#### 查询默认费率（getrate）
```
GET /ai/quota/getrate?model=default
```

响应示例：
```json
{
  "model": "default",
  "input_rate": 10.0,
  "output_rate": 30.0
}
```

#### 查询模型费率（getrate）
```
GET /ai/quota/getrate?model=gpt-4
```

响应示例：
```json
{
  "model": "gpt-4",
  "input_rate": 30.0,
  "output_rate": 60.0
}
```

## 四、技术实现要点

### 4.1 文件范围
- 修改文件：`/Users/zhangshang/git/higress/plugins/wasm-go/extensions/ai-quota/main.go`

### 4.2 主要改动
1. 修改请求前验证逻辑，同时检查 token 和费用预算
2. 修改请求后扣费逻辑，同时扣除 token 和费用
3. 修改管理接口（refresh/query/delta），支持双重预算操作
4. 新增管理接口（setrate/getrate），支持费率设置和查询
5. 实现模型识别和费率加载逻辑（从 Redis 读取）
6. 实现费用计算和精度处理

### 4.3 注意事项
1. 费率存储在 Redis 中，全局统一管理
2. 费率单位为元/百万token，计算时需除以 1,000,000
3. 费用计算需保证精度，避免浮点数误差
4. 并发扣费需使用 Redis 事务或 Lua 脚本保证原子性
5. 模型未配置费率时使用默认费率
6. 向后兼容：原有接口如果未设置费用预算，仍可正常工作（仅检查 token 预算）
