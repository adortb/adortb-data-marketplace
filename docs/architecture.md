# Architecture — adortb-data-marketplace

## 系统定位

受众数据市场，连接数据提供方（如 Experian、Nielsen）与广告主，通过标准化接口实现受众包上架、DSP 实时命中查询和月度分成结算。

## 整体架构图

```
数据提供方                 adortb-data-marketplace                广告主 / DSP
┌──────────┐           ┌──────────────────────────────────────┐  ┌──────────┐
│ Provider │──申请────>│                                      │  │          │
│          │──API Key──│  HTTP API Server                      │<─│ 搜索     │
│          │──上传用户─>│  ├── Provider Registry（接入）        │  │ 受众包   │
│          │           │  ├── Segment Catalog（管理）          │  │          │
└──────────┘           │  ├── Uploader（流式上传）             │<─│ 激活到   │
                       │  ├── Marketplace（搜索/上架）         │  │ 活动     │
                       │  ├── Activation（include/exclude）   │<─│ DSP      │
                       │  ├── Attribution（命中查询）          │  │ 实时查询 │
                       │  ├── UsageTracker（用量记录）         │  └──────────┘
                       │  └── Settlement（月度结算）           │
                       │                                      │
                       │  Redis（用户命中索引）                 │
                       │  PostgreSQL（元数据 + 用量 + 结算）   │
                       └──────────────────────────────────────┘
```

## 核心模块

### Provider Onboarding（`provider/`）

```
1. POST /v1/providers/apply
   → INSERT data_providers (status=pending, revshare_rate=0.70)

2. POST /v1/providers/:id/approve
   → UPDATE status = approved
   → apiKey = "dm_" + hex(rand 32 bytes)
   → UPDATE data_providers SET api_key = apiKey
   → 返回 apiKey（明文，仅此一次）

3. 可选：POST /v1/providers/:id/suspend → status = suspended
```

### Segment 生命周期（`segment/`）

```
                  提供方创建
                      │
              audience_segments (draft)
                      │
          POST /v1/segments/:id/approve
                      │
              audience_segments (approved)
                      │
       POST /v1/segments/:id/users/upload
           │
     ┌─────┴──────┐
     ▼             ▼
  Redis SET       PG segment_users
  segment:users:{id}   （持久化备份）
  TTL=30天
           │
    user_count 更新
```

### 用户列表上传（`segment/uploader.go`）

```
io.Reader（HTTP Body）
     │
buffio.Scanner（逐行读取）
     │ 每 500 条批量
     ├── Redis SADD segment:users:{segmentID} hashes...
     └── PG INSERT segment_users (UPSERT ON CONFLICT)
     │
返回 UploadResult{Total, Succeed, Failed}
```

### 定价策略（`segment/pricer.go`）

```
Pricer.GetStrategy(segment) → PricingStrategy
  ├── FlatFee > 0 → FlatFeeStrategy（固定月费，与 impressions 无关）
  └── CPMFee > 0  → CPMStrategy（fee = impressions / 1000 × cpmFee）
```

### DSP 实时命中查询（`activation/attribution.go`）

**关键性能路径**（高 QPS，O(1)）：
```
POST /v1/targeting/check
  {user_id_hash, segment_ids[]}
        │
        ▼
Redis Pipeline:
  for each segID in segment_ids:
      SISMEMBER segment:users:{segID} user_id_hash  → bool
        │
        ▼
匹配的 segment_ids 列表返回给 DSP
```

Pipeline 并行查询多个 Segment，单次 RTT 完成所有命中检测。

### 使用量追踪（`billing/usage_tracker.go`）

```
TrackBatch(segmentID, campaignID, impressions, totalFee)
        │
        ▼
UPSERT segment_usage
  ON CONFLICT (segment_id, campaign_id, date)
  DO UPDATE SET
    impressions += EXCLUDED.impressions,
    fees += EXCLUDED.fees
```

原子累加，高并发安全。

### 月度结算（`billing/settlement.go`）

```
RunMonthlySettlement(providerID, year, month)
  1. 获取 revshare_rate（providers.revshare_rate，默认 0.70）
  2. 查询该提供方旗下所有 segment_ids
  3. 查询 segment_usage WHERE date BETWEEN month_start AND month_end
  4. totalFees = Σ(record.Fees)
  5. providerEarning = totalFees × revshare_rate
  6. platformRevenue = totalFees - providerEarning
  7. 返回 SettlementReport
```

## 数据存储设计

### Redis（高频查询层）

```
key pattern:  segment:users:{segmentID}
type:         SET（天然去重）
TTL:          30 天（上传时自动续期）
查询：         SISMEMBER → O(1)
批量查询：     Pipeline（单次 RTT）
```

### PostgreSQL（持久化层）

```
data_providers      ─── 提供方主体（UNIQUE: name, api_key）
  │ 1:N
audience_segments   ─── 受众包（UNIQUE: provider_id + segment_id）
  │ 1:N
segment_users       ─── 用户列表 fallback（UNIQUE: segment_id + hash）
  │ 1:N
segment_activations ─── 活动激活（include/exclude）
  │
segment_usage       ─── 日用量（UNIQUE: segment_id + campaign_id + date）
```

## API 分层设计

| 层次 | 路径前缀 | 使用方 |
|------|---------|--------|
| 提供方接入 | `/v1/providers/` | 数据提供方 |
| 市场浏览 | `/v1/marketplace/` | 广告主 |
| 运行时查询 | `/v1/targeting/` | DSP（高频） |
| 平台管理 | `/v1/segments/:id/approve` | adortb 内部 |
| 收益查询 | `/v1/providers/:id/earnings` | 数据提供方 |

## 关键设计决策

1. **双重存储（Redis + PG）**：Redis 承载高 QPS 实时查询（O(1)），PG 作为持久化备份，TTL 保证冷数据自动清理
2. **流式上传**：使用 `bufio.Scanner` 逐行处理，500 条批量提交，支持亿级用户列表上传而不 OOM
3. **Pipeline 批量命中查询**：单次 RTB 请求可并行查询多个 Segment，一次 Redis RTT 完成所有匹配
4. **UPSERT 原子累加**：用量追踪高并发写入，通过 SQL UPSERT 避免分布式锁
5. **revshare 默认 0.70**：提供方拿 70%，平台拿 30%，费率可按提供方差异化配置

## 部署拓扑

```
┌────────────────────────────────────────────────┐
│  adortb-data-marketplace                        │
│  ├── PostgreSQL（元数据 + 用量 + 提供方信息）     │
│  ├── Redis（用户命中索引，30 天 TTL）             │
│  └── Prometheus                                │
└────────────────────────────────────────────────┘
```

DSP 通过 `/v1/targeting/check` 高频调用，建议部署在低延迟网络内（ideally same region 或 private VPC）。
