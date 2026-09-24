-- 补签改为一条独立的 makeup 记录：time 为空，minutes 存入补偿工时，直接计入周总时长，
-- 不再伪造一对签到/签退记录。历史 in/out 记录的 minutes 为 0。
ALTER TABLE checkin_records ADD COLUMN IF NOT EXISTS minutes BIGINT NOT NULL DEFAULT 0;
