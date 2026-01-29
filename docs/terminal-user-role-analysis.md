# 终端用户角色获取 - 代码分析与问题说明

## 一、用户角色从哪里来

### 1. 数据来源

- **角色** 由 `models.User.Admin` 决定：`Admin == true` 视为 `admin`，否则为 `user`。
- **User** 来自数据库表 `user`，`GetUserDetail(username)` 按用户名查完整用户（含 `Admin`）。

### 2. 两条路径

| 场景 | 用户信息从哪来 |
|------|----------------|
| **HTTP 接口（含创建终端）** | JWT 中间件：解析 token → `claims["aud"]` 得到用户名 → `GetUserDetail(username)` → `c.Set("User", user)` |
| **WebSocket 终端会话** | 不经过 JWT：从内存 `sessionUserMap[sessionId]` 取 User，而该映射只在 **创建终端** 的 POST 里写入 |

因此：**WebSocket 侧拿到的“用户角色”完全依赖“创建终端的那次 POST”是否成功把 User 写入当前进程的 sessionUserMap**。

---

## 二、创建终端时如何写入用户（POST）

**路由**：`POST /api/v1/kubernetes/apps/:appid/pods/:pod/terminal/namespaces/:namespace/clusters/:cluster`  
**处理**：`pod.Terminal()`，且该路由在 `appGroup` 下使用了 `middleware.JWTauth()`。

**逻辑**（`controllers/kubernetes/pod/terminal.go`）：

```go
userInterface, exists := c.Get("User")
if exists {
    if user, ok := userInterface.(*models.User); ok {
        sessionUserMap[sessionId] = user   // 只有这里写入了用户
        // ... TTL 清理
    } else {
        klog.Warningf("Terminal: user interface type assertion failed, type: %T", userInterface)
        // 未写入 sessionUserMap
    }
} else {
    klog.Warningf("Terminal: user not found in context")
    // 未写入 sessionUserMap
}
// 无论是否找到用户，都返回 200 和 sessionId
c.JSON(http.StatusOK, gin.H{"data": result})
```

要点：

- 只有 `c.Get("User")` 存在且类型断言为 `*models.User` 时才会写入 `sessionUserMap`。
- 若 `User` 不存在或类型断言失败，**仍会返回 200 和 sessionId**，不会拒绝创建终端。

---

## 三、WebSocket 连接时如何取用户

**路由**：`GET /ws/pods/exec/*param`，**未**使用 JWT 中间件（SockJS 连接通常不带 Authorization）。

**逻辑**（`handleTerminalSession`）：

1. 收第一条消息，要求 `Op == "bind"`，body 为 `TerminalResult`（含 `SessionId`、Token、Cluster、Namespace、Pod、Container 等）。
2. 校验 `checkShellToken(tr.Token, tr.Namespace, tr.Pod)`。
3. **从 sessionUserMap 取用户**：
   ```go
   if u, exists := sessionUserMap[tr.SessionId]; exists {
       user = u
   } else {
       user = nil   // 找不到则 user 为 nil
   }
   ```
4. 用该 `user` 创建 `TerminalSession`，后续命令校验时 `checkCommandPermission(t.user, ...)` 使用。

因此：**WebSocket 侧拿不到用户角色的根本原因，就是此时在本进程中查不到 `sessionUserMap[tr.SessionId]`**。

---

## 四、为什么会出现“获取不到用户角色”

### 1. 多实例 / 负载均衡（最可能）

- `sessionUserMap` 是**进程内内存**，每个实例各自一份。
- 流程：  
  - 创建终端：POST 被调度到 **实例 A** → 在 A 的 `sessionUserMap[sessionId]=user`。  
  - 连接终端：SockJS 被调度到 **实例 B** → B 的 `sessionUserMap` 里没有该 sessionId → `user == nil`。
- 结果：终端能连上（token 校验通过），但整条会话中 `t.user` 始终为 nil，角色被当成 `user`（`checkCommandPermission` 里对 nil 的处理）。

### 2. 创建终端时上下文中就没有 User

- 若 POST 请求未带有效 JWT，或 JWT 中间件未正确设置 `User`，则 `c.Get("User")` 不存在。
- 若某处把 `c.Set("User", xxx)` 设成了非 `*models.User` 类型，类型断言失败，也不会写入 sessionUserMap。
- 当前实现：**仍返回 200 和 sessionId**，前端会去连 WebSocket，此时本进程里本来就没写过该 sessionId → 仍然 `user == nil`。

### 3. 类型断言或 JWT 解析问题

- JWT 中间件：`username := claims["aud"].(string)`，若 JWT 里没有 `aud` 或类型不对会 **panic**，请求直接 500，不会到 Terminal()；若到得了 Terminal()，说明当时 User 已设置。
- 所以“有时能拿到角色、有时拿不到”更符合 **多实例** 或 **有时 POST 未带 token** 的情况，而不是偶发 panic。

### 4. checkCommandPermission 对 user == nil 的处理

```go
if user != nil {
    userName = user.Name
    isAdmin = user.Admin
    role = "admin" or "user"
} else {
    role = "user"
    userName = "unknown"
}
```

- 当 **user 为 nil**（WebSocket 侧从 sessionUserMap 取不到用户）时，**一律按普通用户 `user`** 处理。
- 表现：管理员在终端里也会被当成普通用户，受 user 角色的命令规则限制，出现“获取不到用户角色”的体感。

---

## 五、问题小结

| 问题 | 原因 | 表现 |
|------|------|------|
| 多实例下拿不到角色 | sessionUserMap 仅本进程可见，POST 与 WebSocket 可能落在不同实例 | 同一会话里角色“丢失”，一律按 user |
| 创建终端未强制要求 User | User 缺失或断言失败时仍返回 sessionId | 前端拿到 sessionId 并连接成功，但后端从未写入 user，始终 nil |
| JWT claims["aud"] 未做安全断言 | 缺少或类型错误会 panic | 请求 500，一般不会表现为“有时拿不到角色” |

---

## 六、已实施的修复

1. **JWT 中间件**（`middleware/jwt.go`）  
   - 对 `claims["aud"]` 做安全断言：存在、非 nil、类型为 string、非空，否则返回 401，避免 panic。

2. **创建终端时必须能拿到用户**（`controllers/kubernetes/pod/terminal.go`）  
   - 若 `c.Get("User")` 不存在或类型断言失败，**直接返回 401**，不返回 sessionId，避免“无用户会话”。

3. **终端 token 携带 username，多实例回退**（`controllers/kubernetes/pod/terminal.go`）  
   - `generateToken(namespace, pod, username)` 改为签发 **JWT**（HMAC-SHA256 + appKey），claims 含 `aud=username`、`namespace`、`pod`、`exp`。  
   - 新增 `getUsernameFromShellToken(token, namespace, pod)`：先按 JWT 校验并返回 `aud` 作为用户名；若不是 JWT 或解析失败，再按旧格式校验（不返回用户名）。  
   - `handleTerminalSession`：先校验 token 并得到 `tokenUsername`；再从 `sessionUserMap[sessionId]` 取 user；**若 user 为 nil 且 tokenUsername 非空，则 `GetUserDetail(tokenUsername)` 作为回退**，从而在多实例下也能拿到用户角色。

4. **旧 token 兼容**  
   - 仍支持旧格式 token（无 username）；此时仅能依赖 `sessionUserMap`，多实例下仍可能拿不到用户。新创建的终端一律使用 JWT token。
