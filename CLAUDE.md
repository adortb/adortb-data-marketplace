# CLAUDE.md — adortb-data-marketplace

## 项目概述

受众数据市场。语言 Go 1.25.3，监听端口 `:8115`（可配置）。

## 目录结构

```
cmd/data-marketplace/main.go
internal/
  provider/
    registry.go         # 提供方 CRUD（状态：pending→approved→suspended）
    onboarding.go       # 审批 + 颁发 API Key（dm_ 前缀 + 64 字符 hex）
  segment/
    catalog.go          # 受众包 CRUD（状态：draft→approved/rejected）
    uploader.go         # 流式上传（Scanner + 批量 500 条）
    pricer.go           # 定价策略（CPM / FlatFee）
  activation/
    activator.go        # 活动激活管理（include / exclude 算子）
    attribution.go      # Redis Pipeline 命中查询（O(1)）
  billing/
    usage_tracker.go    # 按天聚合使用记录（UPSERT 幂等）
    settlement.go       # 月度结算（totalFees × revshare_rate）
  marketplace/
    listing.go          # 上架管理
    search.go           # 受众包搜索（按类别/最小规模/状态过滤）
  api/server.go         # 所有路由注册
  metrics/metrics.go
migrations/001_data_marketplace.up.sql
```

## Provider Onboarding 流程

```
1. POST /v1/providers/apply
   → 创建 data_providers 记录，status=pending，revshare_rate 默认 0.70

2. POST /v1/providers/:id/approve
   → status = approved
   → 生成 API Key：dm_ + hex(rand 32 bytes)（64 字符）
   → 返回 api_key（仅此一次明文展示）

3. 提供方使用 API Key 创建/管理受众包

4. POST /v1/providers/:id/suspend（可选）
   → status = suspended，暂停所有操作
```

## Segment 生命周期

```
创建（draft）→ 平台审批（POST /v1/segments/:id/approve）→ approved
                                                        ↓
                                               上传用户列表（NDJSON 流）
                                                        ↓
                                               广告主激活到活动
```

**受众包分类**：`demographic` / `behavioral` / `intent` / `geo`

## 用户列表上传

```go
// uploader.go：流式处理，每 500 条批量提交
// 格式：每行一个 JSON 对象
// {"user_id_hash":"sha256_hash"}
```

**双重存储**：
- Redis：`SADD segment:users:{segmentID} hash…`，TTL 30 天（高频查询）
- PostgreSQL：`segment_users` 表（UPSERT 持久化）

## 定价策略

```go
// 优先级：FlatFee > CPM
type CPMStrategy:      fee = impressions / 1000 × cpmFee
type FlatFeeStrategy:  fee = flatFee（固定月费，忽略 impressions）
```

## 结算流程

```
月度结算：RunMonthlySettlement(providerID, year, month)
  1. 获取提供方分成比例（revshare_rate）
  2. 查询旗下所有受众包的使用记录（from~to）
  3. 聚合 totalFees = Σ(record.Fees)
  4. providerEarning = totalFees × revshare_rate
  5. platformRevenue = totalFees - providerEarning
```

**使用记录幂等写入**：
```sql
INSERT INTO segment_usage (segment_id, campaign_id, date, impressions, fees)
ON CONFLICT (segment_id, campaign_id, date)
DO UPDATE SET
  impressions = segment_usage.impressions + EXCLUDED.impressions,
  fees = segment_usage.fees + EXCLUDED.fees
```

## 高频 DSP 查询（targeting/check）

```go
// attribution.go：Redis Pipeline 并行查询
pipe := redis.Pipeline()
for _, segID := range segmentIDs {
    pipe.SIsMember(ctx, "segment:users:{segID}", userHash)
}
pipe.Exec(ctx)
// 返回命中的 segment ID 列表
```

- 每次查询 O(1)（Redis SET），Pipeline 批量，适合高 QPS RTB 场景

## 关键约定

- API Key 仅在 `Approve` 响应时明文返回一次，数据库存储明文（非哈希）用于请求认证
- `revshare_rate` 默认 0.70，表示提供方获得 70%，平台保留 30%
- 广告主只能搜索/查看 `status=approved` 的受众包
- 命中查询优先走 Redis，Redis 无数据时 fallback 到 PG（`segment_users` 表）

## 数据库

核心表：`data_providers` / `audience_segments` / `segment_users` / `segment_activations` / `segment_usage`

```bash
psql $DATABASE_URL < migrations/001_data_marketplace.up.sql
```
