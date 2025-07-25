# Deployment和DaemonSet重启功能说明

## 概述

本功能实现了Kubernetes资源的重启操作，支持Deployment、DaemonSet和StatefulSet的重启。重启操作使用`k8s.io/kubectl/pkg/polymorphichelpers`包，这是Kubernetes官方kubectl命令使用的相同重启机制，相当于`kubectl rollout restart`命令的功能。

## API接口

### 重启工作负载（使用polymorphichelpers）

**接口地址：**
```
PUT /api/v1/apps/{appId}/_proxy/clusters/{cluster}/namespaces/{namespace}/{kind}/{name}/restart
```

**路径参数：**
- `appId`: 应用ID
- `cluster`: 集群名称
- `namespace`: 命名空间
- `kind`: 资源类型（deployments/daemonsets/statefulsets）
- `name`: 资源名称

**请求体（可选）：**
```json
{
  "force": true
}
```

**响应示例：**
```json
{
  "data": {
    "message": "deployments default/nginx-deployment restarted successfully",
    "timestamp": "2024-01-15T10:30:00Z",
    "success": true
  }
}
```

**支持的资源类型：**
- `deployments`: Deployment
- `daemonsets`: DaemonSet
- `statefulsets`: StatefulSet

## 实现原理

### 重启机制（polymorphichelpers）

重启操作通过以下步骤实现：

1. **获取Kubernetes客户端**：使用多集群客户端管理器获取目标集群的客户端
2. **资源类型映射**：将资源类型映射为对应的`schema.GroupVersionResource`
3. **使用polymorphichelpers**：调用`polymorphichelpers.ObjectRestarterFn`执行重启
4. **触发滚动更新**：polymorphichelpers内部处理Pod模板更新和滚动重启逻辑

### polymorphichelpers优势

1. **官方实现**：使用Kubernetes官方kubectl命令的相同重启逻辑
2. **标准化**：遵循Kubernetes最佳实践和标准
3. **兼容性**：与kubectl命令行为完全一致
4. **维护性**：由Kubernetes社区维护，随版本更新

### 资源映射

```go
switch kind {
case "deployments":
    resource = schema.GroupVersionResource{
        Group:    "apps",
        Version:  "v1",
        Resource: "deployments",
    }
case "statefulsets":
    resource = schema.GroupVersionResource{
        Group:    "apps",
        Version:  "v1",
        Resource: "statefulsets",
    }
case "daemonsets":
    resource = schema.GroupVersionResource{
        Group:    "apps",
        Version:  "v1",
        Resource: "daemonsets",
    }
}
```

## 使用示例

### cURL示例

**重启Deployment：**
```bash
curl -X PUT \
  "http://localhost:8080/api/v1/apps/1/_proxy/clusters/my-cluster/namespaces/default/deployments/nginx-deployment/restart" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"force": true}'
```

**重启DaemonSet：**
```bash
curl -X PUT \
  "http://localhost:8080/api/v1/apps/1/_proxy/clusters/my-cluster/namespaces/kube-system/daemonsets/calico-node/restart" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"force": false}'
```

**重启StatefulSet：**
```bash
curl -X PUT \
  "http://localhost:8080/api/v1/apps/1/_proxy/clusters/my-cluster/namespaces/default/statefulsets/mysql-statefulset/restart" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{"force": true}'
```

### JavaScript示例

```javascript
// 重启工作负载
async function restartWorkload(appId, cluster, namespace, kind, name, force = false) {
  const response = await fetch(
    `/api/v1/apps/${appId}/_proxy/clusters/${cluster}/namespaces/${namespace}/${kind}/${name}/restart`,
    {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`
      },
      body: JSON.stringify({ force })
    }
  );
  
  const result = await response.json();
  return result.data;
}

// 使用示例
restartWorkload(1, 'my-cluster', 'default', 'deployments', 'nginx-deployment', true)
  .then(data => {
    console.log('重启成功:', data.message);
  })
  .catch(error => {
    console.error('重启失败:', error);
  });
```

## 错误处理

### 常见错误

1. **资源不存在**
   ```json
   {
     "error": "deployments.apps \"not-exist\" not found"
   }
   ```

2. **权限不足**
   ```json
   {
     "error": "deployments.apps is forbidden: User \"user\" cannot patch resource \"deployments\" in API group \"apps\" in the namespace \"default\""
   }
   ```

3. **集群连接失败**
   ```json
   {
     "error": "Failed to get kubeClient"
   }
   ```

4. **不支持的资源类型**
   ```json
   {
     "error": "Unsupported resource type"
   }
   ```

### 错误码

- `200`: 重启成功
- `400`: 请求参数错误或资源类型不支持
- `401`: 认证失败
- `403`: 权限不足
- `404`: 资源不存在
- `500`: 服务器内部错误

## 注意事项

1. **权限要求**：用户需要对目标资源具有patch权限
2. **集群连接**：确保集群连接正常
3. **资源状态**：重启操作会触发滚动更新，可能影响服务可用性
4. **强制重启**：使用`force=true`时，会添加强制重启注解，可能导致更激进的更新策略
5. **并发控制**：建议避免同时重启多个相关资源
6. **polymorphichelpers依赖**：确保项目中包含`k8s.io/kubectl`依赖

## 测试

运行测试用例：

```bash
# 运行所有重启功能测试
go test ./controllers/kubernetes/proxy -v

# 运行polymorphichelpers重启功能测试
go test ./controllers/kubernetes/proxy -v -run TestRestartWorkload
```

## 扩展功能

### 支持更多资源类型

可以通过扩展资源映射来支持更多资源类型：

```go
case "cronjobs":
    resource = schema.GroupVersionResource{
        Group:    "batch",
        Version:  "v1",
        Resource: "cronjobs",
    }
```

### 添加重启状态查询

可以添加接口来查询重启状态：

```go
// 查询重启状态
func GetRestartStatus(c *gin.Context) {
    // 实现重启状态查询逻辑
}
```

### 批量重启功能

可以添加批量重启功能：

```go
// 批量重启
func BatchRestart(c *gin.Context) {
    // 实现批量重启逻辑
}
```

## 技术细节

### polymorphichelpers包

`polymorphichelpers`是Kubernetes官方提供的包，用于处理多态资源操作。它提供了：

1. **ObjectRestarterFn**：用于重启资源的函数
2. **ResourceLocation**：资源位置信息结构体
3. **标准化操作**：与kubectl命令行为一致

### 与原有实现的区别

1. **使用官方包**：使用`polymorphichelpers`而不是手动实现patch逻辑
2. **更可靠**：使用经过充分测试的官方实现
3. **更简洁**：代码更简洁，维护成本更低
4. **更标准**：遵循Kubernetes官方标准 