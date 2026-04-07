# Middleware - Auth Header 规范

用于网关统一鉴权后，将用户身份安全传递给下游微服务；同时支持服务与服务之间的轻量校验。

## 设计目标

- 统一 header 规范，保证跨服务解析一致性
- 使用 HMAC 签名防篡改，避免信任链断裂
- 轻量易用，可选空用户（内部调用）
- 可配置版本、Issuer、时间窗与密钥轮换

## Header 规范（v1）

| Header | 必填 | 说明 |
| --- | --- | --- |
| `X-AIS-Auth-V` | 是 | 版本号，固定为 `1` |
| `X-AIS-Auth-Iss` | 是 | 发行方/服务名（gateway 或服务名） |
| `X-AIS-Auth-Ts` | 是 | Unix 秒级时间戳 |
| `X-AIS-Auth-Nonce` | 推荐 | 随机 nonce，避免重放 |
| `X-AIS-Auth-User` | 可选 | base64url(JSON UserInfo) |
| `X-AIS-Auth-Sign` | 是 | HMAC-SHA256 签名（hex） |

签名 payload（使用 `|` 拼接）：

```
v|iss|ts|nonce|user
```

## UserInfo 结构

```json
{
  "user_id": "u123",
  "tenant_id": "t1",
  "dept_id": "d001",
  "username": "alice",
  "roles": ["admin"],
  "permissions": ["order.read"],
  "extra": {"region": "华东"}
}
```

### 租户隔离设计

**第一层：集团隔离（tenant_id）**
- 除以下表外，所有**租户域业务表**必须包含 `tenant_id` 作为隔离键
- 确保所有查询默认带 `tenant_id = ?` 条件
- 例外表：
  - `upms_tenants`：租户实体本身（以 `id` 表示租户）
  - `upms_users` / `upms_password_history`：全局用户域（跨租户）

**第二层：门店隔离（dept_id）**
- `dept_id` 代表集团下的门店/机构
- 所有业务数据默认按 `tenant_id + dept_id` 双重隔离
- 确保数据字段级隔离（Column-based）

## 网关注入（示例）

```go
signer := middleware.NewAuthHeaderSigner(&middleware.AuthHeaderSignerConfig{
    Enabled: true,
    Secret:  "your-shared-secret",
    Issuer:  "gateway",
})

headers, _ := signer.BuildHeaders(&middleware.UserInfo{
    UserID:   "u123",
    TenantID: "t1",
    Roles:    []string{"admin"},
})

req, _ := http.NewRequest(http.MethodGet, "http://svc/api", nil)
middleware.WriteAuthHeaders(req.Header, headers)
```

## 下游服务校验（示例）

```go
verifier := middleware.NewAuthHeaderVerifier(&middleware.AuthHeaderVerifierConfig{
    Enabled:        true,
    Secret:         "your-shared-secret",
    AllowedIssuers: []string{"gateway"},
    MaxAge:         5 * time.Minute,
}, log)

app.Use(verifier.Authenticate())

app.Get("/api", func(c fiber.Ctx) error {
    user, _ := middleware.UserFromContext(c)
    return c.JSON(user)
})
```

## 服务与服务之间

- 使用同一套 header + 签名机制，`Issuer` 设置为调用方服务名。
- 无用户上下文时，可配置 `AllowEmptyUser: true`。
- 可通过 `Secrets` 配置多 Issuer 密钥，支持轮换。

## 安全与约束建议

- 强制使用 TLS 保护 header 不被窃听。
- `MaxAge` 建议 1~5 分钟，结合 `AllowedClockSkew` 处理时钟漂移。
- 建议开启 `Nonce`，并在业务侧引入去重存储（如 Redis）增强防重放。
- 密钥定期轮换，使用 `Secrets` 灰度过渡。
