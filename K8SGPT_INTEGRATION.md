# K8sGPT 集成说明

本文档说明如何在 gwayne 项目中使用 K8sGPT 进行 Kubernetes 集群的智能诊断。

## 功能概述

已集成 K8sGPT 到 gwayne 项目，提供以下功能：

1. **AI 引擎管理**：支持配置和管理多个 AI 后端（OpenAI、Azure、Cohere、Amazon Bedrock、Google Gemini 等）
2. **智能诊断能力**：对 Kubernetes 集群资源进行智能分析和诊断
3. **资源诊断**：支持对 Node、Pod、Event 等资源进行诊断
4. **AI 生成说明**：使用 AI 生成自然语言的问题描述和解决方案

## API 接口

### 1. AI 引擎管理

#### 列出所有 AI 提供者
```
GET /api/v1/k8sgpt/ai/providers
```

#### 添加 AI 提供者
```
POST /api/v1/k8sgpt/ai/providers
Content-Type: application/json

{
  "name": "openai",
  "model": "gpt-4",
  "password": "your-api-key",
  "baseurl": "https://api.openai.com/v1",
  "temperature": 0.7
}
```

#### 删除 AI 提供者
```
DELETE /api/v1/k8sgpt/ai/providers/:name
```

#### 设置默认 AI 提供者
```
PUT /api/v1/k8sgpt/ai/providers/:name/default
```

#### 获取可用的 AI 后端列表
```
GET /api/v1/k8sgpt/ai/backends
```

### 2. 诊断接口

#### 通用诊断接口
```
POST /api/v1/k8sgpt/diagnose
Content-Type: application/json

{
  "cluster": "cluster-name",
  "namespace": "default",
  "resourceType": "Pod",
  "resourceName": "my-pod",
  "filters": ["Pod"],
  "explain": true,
  "backend": "openai",
  "language": "中文"
}
```

#### 诊断节点
```
GET /api/v1/k8sgpt/diagnose/node/:cluster/:name?explain=true
```

#### 诊断 Pod
```
GET /api/v1/k8sgpt/diagnose/pod/:cluster/:namespace/:name?explain=true
```

#### 诊断事件
```
GET /api/v1/k8sgpt/diagnose/event/:cluster/:namespace?explain=true
```

### 3. 资源页面诊断接口

#### Node 页面诊断
```
GET /api/v1/kubernetes/nodes/:name/clusters/:cluster/diagnose?explain=true
```

#### Pod 页面诊断
```
GET /api/v1/kubernetes/apps/:appid/pods/namespaces/:namespace/clusters/:cluster/diagnose?name=pod-name&explain=true
```

#### Event 页面诊断
```
GET /api/v1/kubernetes/events/namespaces/:namespace/clusters/:cluster/diagnose?explain=true
```

## 使用示例

### 1. 配置 AI 引擎

首先需要添加一个 AI 提供者：

```bash
curl -X POST http://localhost:8080/api/v1/k8sgpt/ai/providers \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "name": "openai",
    "model": "gpt-4",
    "password": "sk-your-api-key",
    "baseurl": "https://api.openai.com/v1"
  }'
```

### 2. 诊断节点

```bash
curl -X GET "http://localhost:8080/api/v1/k8sgpt/diagnose/node/my-cluster/my-node?explain=true" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

响应示例：
```json
{
  "data": {
    "status": "ProblemDetected",
    "problems": 1,
    "provider": "openai",
    "results": [
      {
        "kind": "Node",
        "name": "my-node",
        "errors": [
          "my-node has condition of type Ready, reason KubeletNotReady: container runtime network not ready"
        ],
        "details": "节点 my-node 处于未就绪状态。原因是容器运行时网络未就绪。\n\n解决方案：\n1. 检查容器运行时（如 Docker 或 containerd）的网络配置\n2. 检查 CNI 插件是否正确安装和配置\n3. 查看 kubelet 日志以获取更多信息：kubectl logs -n kube-system kubelet-xxx\n4. 重启 kubelet 服务：systemctl restart kubelet"
      }
    ]
  }
}
```

### 3. 诊断 Pod

```bash
curl -X GET "http://localhost:8080/api/v1/k8sgpt/diagnose/pod/my-cluster/default/my-pod?explain=true" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### 4. 诊断事件

```bash
curl -X GET "http://localhost:8080/api/v1/k8sgpt/diagnose/event/my-cluster/default?explain=true" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## 配置说明

### AI 提供者配置字段

- `name`: 提供者名称（如 openai、azureopenai、cohere 等）
- `model`: 使用的模型名称
- `password`: API 密钥（某些提供者需要）
- `baseurl`: API 基础 URL（可选）
- `temperature`: 温度参数（0-1，控制输出的随机性）
- `providerregion`: 提供者区域（某些云服务需要）
- 其他字段根据不同的 AI 提供者可能有所不同

### 支持的 AI 后端

- openai
- azureopenai
- cohere
- amazonbedrock
- amazonsagemaker
- google
- googlevertexai
- ollama
- localai
- huggingface
- customrest
- ibmwatsonxai

## 注意事项

1. **API 密钥安全**：AI 提供者的 API 密钥存储在配置文件中，请确保配置文件的安全访问权限
2. **API 配额**：使用 AI 解释功能会消耗 API 配额，请注意使用频率
3. **网络连接**：确保服务器能够访问所配置的 AI 服务端点
4. **集群访问**：诊断功能需要访问 Kubernetes 集群，确保 gwayne 有相应的集群访问权限

## 故障排查

### 1. AI 提供者配置失败

- 检查 API 密钥是否正确
- 检查网络连接是否正常
- 查看日志获取详细错误信息

### 2. 诊断失败

- 检查集群连接是否正常
- 检查是否有相应的集群访问权限
- 查看日志获取详细错误信息

### 3. AI 解释生成失败

- 检查 AI 提供者配置是否正确
- 检查 API 配额是否充足
- 检查网络连接是否正常

## 开发说明

### 项目结构

```
gwayne/
├── pkg/k8sgpt/
│   ├── ai_manager.go      # AI 引擎管理
│   └── diagnostic.go      # 诊断服务
├── controllers/k8sgpt/
│   ├── ai.go              # AI 引擎管理控制器
│   └── diagnostic.go      # 诊断控制器
└── routers/
    └── router_k8sgpt.go  # K8sGPT 路由配置
```

### 扩展功能

如需添加新的资源类型诊断，可以：

1. 在 `pkg/k8sgpt/diagnostic.go` 中添加新的诊断方法
2. 在相应的控制器中添加诊断端点
3. 在路由配置中添加相应的路由

## 参考文档

- [K8sGPT 官方文档](https://docs.k8sgpt.ai)
- [K8sGPT GitHub](https://github.com/k8sgpt-ai/k8sgpt)

