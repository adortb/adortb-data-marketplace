-- 数据提供方
CREATE TABLE IF NOT EXISTS data_providers (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    company VARCHAR(255),
    contact_email VARCHAR(128),
    description TEXT,
    website VARCHAR(255),
    status VARCHAR(20) DEFAULT 'pending',  -- pending/approved/suspended
    revshare_rate DECIMAL(5,4) DEFAULT 0.70,  -- 数据商的分成（平台拿 30%）
    api_key VARCHAR(128) UNIQUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    approved_at TIMESTAMPTZ
);

-- 受众包
CREATE TABLE IF NOT EXISTS audience_segments (
    id BIGSERIAL PRIMARY KEY,
    provider_id BIGINT NOT NULL REFERENCES data_providers(id),
    segment_id VARCHAR(128) NOT NULL,      -- 数据商内部 ID
    name VARCHAR(255) NOT NULL,
    description TEXT,
    iab_taxonomy VARCHAR(30),              -- IAB Audience Taxonomy
    category VARCHAR(64),                   -- demographic/behavioral/intent/geo
    user_count BIGINT,                      -- 受众规模
    recency_days INT,                       -- 数据新鲜度
    cpm_fee DECIMAL(10,4) NOT NULL,         -- 每千次使用费用（USD）
    flat_fee DECIMAL(10,4),                 -- 或固定月费
    status VARCHAR(20) DEFAULT 'draft',
    approved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (provider_id, segment_id)
);

-- 受众包用户列表（Redis 存更好，这里建 PG 表仅作 fallback）
CREATE TABLE IF NOT EXISTS segment_users (
    id BIGSERIAL PRIMARY KEY,
    segment_id BIGINT REFERENCES audience_segments(id),
    user_id_hash VARCHAR(128) NOT NULL,     -- hashed user id
    added_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (segment_id, user_id_hash)
);
CREATE INDEX IF NOT EXISTS idx_seguser_hash ON segment_users(user_id_hash);

-- 活动激活
CREATE TABLE IF NOT EXISTS segment_activations (
    id BIGSERIAL PRIMARY KEY,
    campaign_id BIGINT NOT NULL,
    segment_id BIGINT NOT NULL REFERENCES audience_segments(id),
    operator VARCHAR(20) DEFAULT 'include',  -- include/exclude
    activated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 使用计数
CREATE TABLE IF NOT EXISTS segment_usage (
    id BIGSERIAL PRIMARY KEY,
    segment_id BIGINT NOT NULL,
    campaign_id BIGINT,
    date DATE NOT NULL,
    impressions BIGINT DEFAULT 0,
    fees DECIMAL(15,4) DEFAULT 0,
    UNIQUE (segment_id, campaign_id, date)
);
