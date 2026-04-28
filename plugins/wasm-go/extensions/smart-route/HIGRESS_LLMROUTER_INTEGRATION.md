# 🚀 Higress + LLMRouter 智能路由深度集成方案 (v1.0)

## 一、 核心架构设计

本方案采用 **“两阶段决策逻辑”**：
1.  **逻辑阶段**：Higress 收到 `model: auto` 请求，调用 LLMRouter 服务，获取最终的 `供应商/模型` 对。
2.  **分发阶段**：根据 LLMRouter 返回的结果，重新打标并路由至真实的后端供应商。

---

## 二、 配置规范

### 1. Higress 基础环境配置
*   **Local Provider (内部递归源)**：
    *   预先创建一个名为 `local-gateway` 的 Provider，地址指向 Higress 自身的公网或内网入口。
    *   **超级 Consumer**：创建一个内部专用的 Consumer，设置为不计费模式（Internal-Super-User），用于插件内部发起递归请求。
*   **Provider 路由 (分发层)**：
    *   创建指向各个真实供应商（如 OpenAI, LiteLLM, NVIDIA）的路由。
    *   **匹配条件**：使用 HTTP Header `x-provider: {provider-id}` 进行精确匹配。
    *   **权限**：仅限内部超级 Consumer 或特定网段访问。

### 2. 智能路由配置 (接入层)
*   **智能模型名**：在 Higress 侧定义一个虚拟模型名，例如 `auto`。
*   **插件配置**：在匹配该虚拟模型的路由上启用 `smart-route` 插件：
    *   `llm_router_url`: `http://192.168.36.112:8080/v1/smart-route`
    *   `router_id`: `business_main_logic`
    *   `auth_token`: `(内部鉴权 Token)`

### 3. LLMRouter 业务配置 (决策层)
在 LLMRouter 服务端持久化的 `business_main_logic.yaml` 配置：

```yaml
# 文件名: business_main_logic.yaml
router:
  strategy: "chain" # 混合链式决策漏斗

  pipeline:
    - name: "fast_rules" # 第一层：极速规则拦截 (延迟 < 0.1ms)
      strategy: "rules"
      rules:
        # A. 内联关键词拦截
        - keywords: ["你好", "早上好", "hello"]
          model: "litellm/qwen-0.5b"

        # B. 外部大型词库引用 (支持动态热更新)
        - keywords_file:
            - "configs/rules/python.txt"
            - "configs/rules/java.txt"
          model: "litellm/Qwen3-Coder-Next"

    - name: "ai_judge" # 第二层：大模型智能裁判 (语义匹配)
      strategy: "llm"
      model: "Qwen3-Coder-Next" # 决策大脑
      base_url: "http://192.168.36.112:4000/v1"

  # 第三层：全链路未命中的兜底模型
  default_model: "litellm/Qwen3-8B"

llms:
  "litellm/qwen-0.5b":
    description: "Category: [Greeting, Chat]. 轻量快速，处理简单意图。"
  "litellm/Qwen3-Coder-Next":
    description: "Category: [Programming, Python, Code]. 编程专家，精通各种语言。"

api_keys:
  local: "sk-1234" # 用于调用裁判模型的密钥
```

---

## 三、 处理流程追踪

### 第一阶段：智能决策 (Smart Routing)
1.  **用户请求**：Client 发起请求，指定模型为 `model: auto`。
2.  **命中插件**：Higress 识别到 `auto` 模型，触发 `smart-route` 插件。
3.  **外部决策**：
    *   插件将用户的 `prompt` 提取，发送至 **LLMRouter API**。
    *   LLMRouter 按照 `Rules -> LLM Judge -> Default` 的漏斗逻辑进行决策。
    *   **返回结果**：例如返回 `litellm/Qwen3-Coder-Next`。
4.  **请求改写**：
    *   插件将返回结果拆分为：供应商 `litellm` 和模型 `Qwen3-Coder-Next`。
    *   **注入 Header**：添加 `x-provider: litellm`。
    *   **修改 Body**：将请求体中的模型名修改为 `Qwen3-Coder-Next`。
5.  **计费前置**：Higress 根据当前的 `Consumer` + `修改后的模型名` 进行计费预扣。

### 第二阶段：精细分发 (Provider Routing)
1.  **内部递归**：修改后的请求带上 `x-provider` 重新进入 Higress 路由引擎。
2.  **分发匹配**：路由引擎识别到 `x-provider: litellm` 标志。
3.  **后端透传**：请求被精准发送至 `192.168.36.112:4000` (LiteLLM 供应商) 提供的真实模型服务。
4.  **结果回传**：模型响应沿原路返回至客户端。
