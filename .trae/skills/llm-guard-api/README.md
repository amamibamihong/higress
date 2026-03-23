# LLM Guard API Skill

## 简介

LLM Guard API 提供 AI 安全扫描服务，支持对 LLM 的输入（Prompt）和输出（Output）进行安全性检测和清理。

## 核心功能

| 功能 | 描述 |
|------|------|
| 输入扫描 | 检测用户输入的安全性 |
| 输出扫描 | 检测 LLM 响应的安全性 |
| Analyze 模式 | 分析并返回清理后的文本 |
| Scan 模式 | 快速扫描返回风险评分 |

## 快速开始

### 认证

所有 API 调用需要携带认证头：
```
Authorization: Bearer YOUR_AUTH_TOKEN
```

### 示例：输入分析

```bash
curl -X POST "http://localhost:8000/analyze/prompt" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
     -d '{"prompt": "Tell me a secret."}'
```

### 示例：输出扫描

```bash
curl -X POST "http://localhost:8000/scan/output" \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer YOUR_AUTH_TOKEN" \
     -d '{"prompt": "What is AI?", "output": "AI stands for Artificial Intelligence."}'
```

## API 端点列表

### 输入扫描
- `POST /analyze/prompt` - 分析并清理输入
- `POST /scan/prompt` - 快速扫描输入

### 输出扫描
- `POST /analyze/output` - 分析并清理输出
- `POST /scan/output` - 快速扫描输出

### 健康检查
- `GET /` - API 信息
- `GET /healthz` - 健康状态
- `GET /readyz` - 就绪状态
- `GET /metrics` - Prometheus 指标

## 详细文档

更多详细信息请参考 [SKILL.md](./SKILL.md)。