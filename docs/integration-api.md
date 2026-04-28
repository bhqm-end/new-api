# Integration API 使用文档

本文档面向外部业务服务端，例如生图网站后端。该 API 用于在 New API 侧完成用户绑定、API Key 管理和卡密兑换。

## 基础信息

- Base URL: `https://{new-api-domain}`
- 接口前缀: `/api/integration`
- 鉴权方式: `Authorization: Bearer <INTEGRATION_API_KEY>`
- Content-Type: `application/json`

`INTEGRATION_API_KEY` 需要在 New API 服务端配置环境变量，或在系统 option 中配置 `IntegrationApiKey`。该密钥只应保存在生图网站服务端，禁止下发到浏览器、小程序、App 客户端。

所有业务接口默认返回统一结构：

```json
{
  "success": true,
  "message": "",
  "data": {}
}
```

鉴权失败可能返回 HTTP `401` 或 `403`。业务参数错误通常返回 HTTP `200` 且 `success=false`，调用方必须同时判断 HTTP 状态码和响应体 `success` 字段。

## 推荐接入流程

1. 生图网站用户注册或首次使用时，调用 `POST /api/integration/users`，用 `external_user_id` 绑定或创建 New API 用户。
2. 创建成功后保存返回的 New API `id`，作为后续 `user_id` 使用。
3. 调用 `POST /api/integration/users/{user_id}/tokens` 创建 API Key。
4. 生图请求由生图网站服务端使用该 API Key 调用 New API relay 接口。
5. 用户购买卡密后，生图网站服务端调用 `POST /api/integration/users/{user_id}/redeem` 兑换额度。

## 1. 创建或绑定用户

按外部用户 ID 幂等创建 New API 用户。相同 `external_user_id` 重复调用不会创建重复用户，会返回已有用户。

```http
POST /api/integration/users
Authorization: Bearer <INTEGRATION_API_KEY>
Content-Type: application/json
```

请求体：

```json
{
  "external_user_id": "image_site_user_10001",
  "username": "optional_username",
  "display_name": "用户昵称",
  "email": "user@example.com",
  "group": "default"
}
```

字段说明：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `external_user_id` | 是 | 生图网站侧用户唯一 ID，最长 128 字符 |
| `username` | 否 | New API 用户名；不传时自动按 `external_user_id` 生成 |
| `display_name` | 否 | 显示名；不传时使用用户名，最长 20 字符 |
| `email` | 否 | 邮箱，最长 50 字符 |
| `group` | 否 | New API 用户分组；不传默认为 `default` |

成功响应：

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 12,
    "external_user_id": "image_site_user_10001",
    "username": "ext_4c530bd28f2c9c20",
    "display_name": "用户昵称",
    "email": "user@example.com",
    "group": "default",
    "quota": 0,
    "created": true,
    "created_at": 1710000000,
    "last_login_at": 0
  }
}
```

`created=true` 表示本次新建用户，`created=false` 表示返回已有绑定用户。

## 2. 创建用户 API Key

为指定 New API 用户创建可直接用于 relay 的完整 API Key。

```http
POST /api/integration/users/{user_id}/tokens
Authorization: Bearer <INTEGRATION_API_KEY>
Content-Type: application/json
```

请求体：

```json
{
  "name": "image-site-default",
  "group": "default",
  "expired_time": -1,
  "allow_ips": "",
  "model_limits_enabled": false,
  "model_limits": "",
  "cross_group_retry": false
}
```

字段说明：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `name` | 否 | API Key 名称；不传默认为 `image-site-default`，最长 50 字符 |
| `group` | 是 | token 使用分组，例如 `default` |
| `expired_time` | 否 | 过期 Unix 时间戳；`-1` 表示永不过期，默认 `-1` |
| `allow_ips` | 否 | IP 白名单，格式沿用 New API token 配置 |
| `model_limits_enabled` | 否 | 是否启用模型限制 |
| `model_limits` | 否 | 模型限制配置，格式沿用 New API token 配置 |
| `cross_group_retry` | 否 | 是否启用跨分组重试，主要用于 `auto` 分组 |

成功响应：

```json
{
  "success": true,
  "message": "",
  "data": {
    "id": 35,
    "user_id": 12,
    "name": "image-site-default",
    "key": "sk-xxxxxxxxxxxxxxxx",
    "masked_key": "sk-x**********xxxx",
    "status": 1,
    "group": "default",
    "created_time": 1710000000,
    "accessed_time": 1710000000,
    "expired_time": -1,
    "remain_quota": 0,
    "used_quota": 0,
    "unlimited_quota": true,
    "model_limits_enabled": false,
    "model_limits": "",
    "cross_group_retry": false
  }
}
```

说明：

- 返回的 `key` 是完整 API Key，只会通过 integration 接口返回，请由生图网站服务端加密或安全保存。
- integration 创建的 token 默认 `unlimited_quota=true`、`remain_quota=0`，实际消费扣用户余额。
- `group` 必须是该用户可用且系统启用的分组，否则创建失败。

## 3. 查询用户 API Key

查询指定用户的 API Key。可按分组过滤。

```http
GET /api/integration/users/{user_id}/tokens?group=default
Authorization: Bearer <INTEGRATION_API_KEY>
```

Query 参数：

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `group` | 否 | 指定 token 分组；不传时返回该用户所有 token |

成功响应：

```json
{
  "success": true,
  "message": "",
  "data": [
    {
      "id": 35,
      "user_id": 12,
      "name": "image-site-default",
      "key": "sk-xxxxxxxxxxxxxxxx",
      "masked_key": "sk-x**********xxxx",
      "status": 1,
      "group": "default",
      "created_time": 1710000000,
      "accessed_time": 1710000000,
      "expired_time": -1,
      "remain_quota": 0,
      "used_quota": 120,
      "unlimited_quota": true,
      "model_limits_enabled": false,
      "model_limits": "",
      "cross_group_retry": false
    }
  ]
}
```

## 4. 兑换卡密

按 New API 用户 ID 兑换卡密，将卡密额度充值到该用户余额。

```http
POST /api/integration/users/{user_id}/redeem
Authorization: Bearer <INTEGRATION_API_KEY>
Content-Type: application/json
```

请求体：

```json
{
  "key": "redemption_card_key"
}
```

成功响应：

```json
{
  "success": true,
  "message": "",
  "data": {
    "quota": 100000,
    "user_quota": 250000
  }
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `quota` | 本次卡密充值额度 |
| `user_quota` | 兑换后用户余额 |

安全语义：

- 只有有效 `user_id` 才会消耗卡密。
- 如果 `user_id` 不存在、卡密无效、卡密已使用或卡密过期，兑换失败且不会充值。
- 无效用户兑换失败不会把卡密标记为已使用。

## 调用 relay 生图接口

integration 接口创建出的 `key` 可以直接作为 New API relay API Key 使用。示例：

```http
POST /v1/images/generations
Authorization: Bearer <USER_API_KEY>
Content-Type: application/json
```

请求体按 New API 已支持的上游模型协议传入，例如：

```json
{
  "model": "gpt-image-1",
  "prompt": "a cute cat sitting on a desk",
  "size": "1024x1024"
}
```

该请求会按现有 New API 计费和渠道规则扣减 token 所属用户的余额。

## cURL 示例

```bash
BASE_URL="https://new-api.example.com"
INTEGRATION_API_KEY="your-integration-api-key"

curl -sS "$BASE_URL/api/integration/users" \
  -H "Authorization: Bearer $INTEGRATION_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "image_site_user_10001",
    "display_name": "用户昵称",
    "group": "default"
  }'
```

```bash
curl -sS "$BASE_URL/api/integration/users/12/tokens" \
  -H "Authorization: Bearer $INTEGRATION_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "image-site-default",
    "group": "default",
    "expired_time": -1
  }'
```

```bash
curl -sS "$BASE_URL/api/integration/users/12/tokens?group=default" \
  -H "Authorization: Bearer $INTEGRATION_API_KEY"
```

```bash
curl -sS "$BASE_URL/api/integration/users/12/redeem" \
  -H "Authorization: Bearer $INTEGRATION_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "key": "redemption_card_key"
  }'
```

## 错误处理建议

- HTTP `401`: 未提供 Bearer token，或 `INTEGRATION_API_KEY` 不正确。
- HTTP `403`: New API 未配置 `INTEGRATION_API_KEY`。
- `success=false`: 业务失败，读取 `message` 展示或记录。
- 对创建用户接口，建议生图网站始终以 `external_user_id` 调用，不依赖用户名唯一性。
- 对创建 token 接口，建议生图网站保存返回的 `key`；如丢失可调用查询接口重新获取。
- 对兑换接口，建议按卡密维度做幂等保护，避免用户重复提交导致第二次返回已使用。

