# adortb-data-marketplace

受众数据市场（第九期）。连接数据提供方与广告主，支持受众包上架、DSP 实时查询命中、月度结算。

## 功能概览

- 数据提供方接入审核（申请 → 审批 → 颁发 API Key）
- 受众包管理（创建 → 审批 → 上架市场）
- 用户列表流式上传（Redis SET + PG fallback，30 天 TTL）
- CPM 和固定月费两种定价策略
- DSP 实时命中查询（Redis Pipeline，O(1)）
- 月度结算（分成比例默认 70% 给提供方）
- Prometheus 监控

## 快速启动

```bash
export DATABASE_URL="postgres://user:pass@localhost/adortb_data_marketplace"
export REDIS_ADDR="localhost:6379"
export PORT="8115"

go run ./cmd/data-marketplace
```

## API 端点

**数据提供方侧**
```
POST   /v1/providers/apply                 # 申请成为提供方
POST   /v1/providers/:id/approve           # 审批并颁发 API Key
POST   /v1/providers/:id/segments          # 创建受众包
POST   /v1/segments/:id/users/upload       # 上传用户列表（NDJSON 流）
GET    /v1/providers/:id/earnings          # 查看收益明细
```

**广告主侧**
```
GET    /v1/marketplace/segments            # 搜索受众包
GET    /v1/segments/:id                   # 受众包详情
POST   /v1/campaigns/:campaign_id/segments # 激活受众包到活动
GET    /v1/campaigns/:campaign_id/segments/usage  # 查看消耗
```

**平台管理**
```
POST   /v1/segments/:id/approve            # 审批受众包
```

**DSP 运行时**
```
POST   /v1/targeting/check                 # 检查用户命中（高频接口）
```

## 用户上传格式

每行一个 JSON 对象（NDJSON）：

```
{"user_id_hash":"sha256_of_user_id_1"}
{"user_id_hash":"sha256_of_user_id_2"}
```

## 命中查询示例

```json
POST /v1/targeting/check
{
  "user_id_hash": "abc123...",
  "segment_ids": [10, 20, 30]
}

Response:
{
  "matched_segment_ids": [10, 20]
}
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DATABASE_URL` | `postgres://localhost/adortb_data_marketplace` | PostgreSQL 连接串 |
| `REDIS_ADDR` | `localhost:6379` | Redis 地址 |
| `PORT` | `8115` | 监听端口 |

## 技术栈

- Go 1.25.3
- PostgreSQL + Redis
- Prometheus
