# LiteLLM Presidio PII 技能

这是一个关于 LiteLLM 中 Presidio PII（个人身份信息）检测、屏蔽和配置的综合指南 Skill。

## 文件说明

- `SKILL.md` - Skill 定义文件，包含完整的 Presidio PII 功能说明
- `litellm-presidio_pii.zip` - 打包好的 Skill ZIP 文件
- `example_presidio_pii_skill.py` - 示例代码，演示如何创建和使用此 Skill

## Skill 内容概览

1. **内容提取** - 如何从请求和响应中提取需要检测的文本
2. **PII 检测流程** - 如何发起检测请求、处理响应
3. **检测后操作** - 如何替换 PII 和阻断请求
4. **Presidio PII 配置** - 实体类型、动作、置信度阈值等配置
5. **提高检测准确率** - 使用自定义识别器和上下文关键词

## 使用方法

### 创建 Skill

将 Skill 上传到 LiteLLM 数据库：

```python
from litellm import create_skill

skill = create_skill(
    display_title="LiteLLM Presidio PII 指南",
    files=[open("litellm-presidio_pii.zip", "rb")],
    custom_llm_provider="litellm_proxy",
)

print(f"已创建 skill: {skill.id}")  # 保存这个 skill_id
```

### 使用 Skill

在任何 LLM 提供商上使用此 Skill：

```python
import litellm

response = litellm.completion(
    model="gpt-4o-mini",  # 支持任何提供商
    messages=[{"role": "user", "content": "如何配置 Presidio 阻断信用卡号?"}],
    container={
        "skills": [
            {"type": "custom", "skill_id": "litellm:skill_abc123"}  # 替换为实际 skill_id
        ]
    }
)
```

### 通过 Proxy 使用

启动 LiteLLM Proxy 后，通过 API 创建 Skill：

```bash
curl "http://0.0.0.0:4000/v1/skills" \
  -X POST \
  -H "X-Api-Key: sk-1234" \
  -F "display_title=Presidio PII 指南" \
  -F "files[]=@litellm-presidio_pii.zip"
```

## 主要技术细节

### 检测流程

1. **提取内容**: 从 `messages` 字段提取文本
2. **分析请求**: 调用 Presidio analyze 接口
3. **过滤结果**: 根据置信度阈值过滤检测项
4. **阻断检查**: 检查是否有阻断类型的实体
5. **替换处理**: 调用 Presidio anonymize 接口进行替换

### 配置示例

```yaml
guardrails:
  presidio:
    entities:
      PERSON:
        action: "MASK"
      EMAIL_ADDRESS:
        action: "MASK"
      PHONE_NUMBER:
        action: "MASK"
      CREDIT_CARD:
        action: "BLOCK"
    presidio_score_thresholds:
      PERSON: 0.85
      EMAIL_ADDRESS: 0.90
      DEFAULT: 0.75
    language: "en"
```

### 提高准确率

使用自定义识别器（ad-hoc recognizers）提高特定实体检测准确率：

```python
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
      ]
    }
  ]
}
```

## 相关文件

- 实现文件: `/litellm/proxy/guardrails/guardrail_hooks/presidio.py:712`
- 测试文件: `/tests/test_litellm/proxy/guardrails/guardrail_hooks/test_presidio.py`
- 文档: `/docs/my-website/docs/proxy/guardrails/pii_masking_v2.md`
- 示例识别器: `/litellm/proxy/hooks/example_presidio_ad_hoc_recognizer.json`
