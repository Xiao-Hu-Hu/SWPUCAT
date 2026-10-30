-- Update repository-managed official knowledge articles to match the current
-- code after migrations 039/040 changed report deadline/late handling and
-- converted 补签 into a standalone makeup record.
-- Official articles are revised by a new migration with a higher revision only;
-- do not rewrite 033.

-- 1) 打卡、签退与异常处理规则：明确补签(makeup)的字段与口径
UPDATE knowledge_items
SET name = '打卡、签退、补签与异常处理规则',
    description = $doc$# 打卡、签退、补签与异常处理规则

> 依据：当前打卡领域逻辑、接口权限与自动签退任务。适用于 SWPUCAT 内的工时记录。

## 正常流程

1. 开始工作时执行上班打卡，结束工作时执行下班打卡。
2. 系统只把同一天内按时间顺序配对成功的“上班—下班”记录计入工时。
3. 当天仍处于上班状态的记录会在日终任务中按 23:59:59 自动签退；成员仍应在实际结束工作时主动签退。
4. 助手的打卡写操作使用幂等请求，重复提交同一请求不会重复生成记录。

## 补签（makeup）

1. 补签不是伪造一对签到/签退，而是一条独立的补签记录：时间（time）为空，补偿工时存入 minutes，直接计入当日、本周和本月累计时长。
2. 补签分钟数限制为 1~480（最多 8 小时），日期不得晚于今天。
3. 历史正常配对的签到/签退记录其 minutes 为 0；只有补签记录才有非零 minutes。

## 异常说明

系统会标记重复上班打卡、没有对应上班记录的签退、时间先后无效、缺少签退以及未知记录类型。异常记录不会被助手自行猜测或补齐；需要更正时应联系有权限的队长或超级管理员核对并补签。

## 查询口径

本人历史记录和本人周/月累计只返回当前登录账号的数据。团队统计仅队长或超级管理员可用。$doc$,
    revision = revision + 1
WHERE source_key = 'official.checkin-policy';

-- 2) 月报周期、版本与提交流程：补充提交截止时间与逾期提交
UPDATE knowledge_items
SET description = $doc$# 月报周期、版本、提交流程与截止时间

> 依据：当前月报领域模型、截止与逾期逻辑以及权限规则。

1. 月报按 `YYYY-MM` 周期管理，周期状态分为开放（open）、关闭（closed）和归档（archived）。
2. 周期成员快照决定谁需要提交；豁免成员必须记录原因。
3. 报告先创建为草稿。每次编辑形成递增版本，旧版本保留，不能原地覆盖历史版本。
4. 只有正文非空的草稿版本可以提交；提交后可由有权限的管理者审核通过或退回；作者可在审核前撤回已提交版本，修改后重新提交。
5. 周期可配置提交截止时间（due_at）；超过截止时间后不再接受创建、保存或提审，编辑器被锁定。
6. 在报告窗口关闭后、截止时间前提交的报告会被标记为“逾期提交”（late_submission），该标记在审核阶段保留。
7. 成员只能读取自己的报告材料；团队材料只对队长或超级管理员开放。
8. 使用智能助手总结月报前，应先查询周期列表并选定准确周期；不得猜测内部周期 ID。

当前周期、截止时间和提交状态是动态数据，应以系统查询结果为准。$doc$,
    revision = revision + 1
WHERE source_key = 'official.monthly-report-rules';

-- Queue the revised descriptions so the retrieval index picks up the new text.
-- The periodic reconciler remains the fallback and replaces the empty
-- fingerprint with the current config.
INSERT INTO knowledge_index_jobs (
    knowledge_item_id, status, source_fingerprint, attempts,
    available_at, locked_at, last_error, created_at, updated_at
)
SELECT id, 'queued', '', 0, CURRENT_TIMESTAMP, NULL, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM knowledge_items
WHERE source_origin = 'official_catalog'
  AND source_key IN ('official.checkin-policy', 'official.monthly-report-rules')
ON CONFLICT (knowledge_item_id) DO UPDATE SET
    status = 'queued',
    source_fingerprint = '',
    attempts = 0,
    available_at = CURRENT_TIMESTAMP,
    locked_at = NULL,
    last_error = '',
    updated_at = CURRENT_TIMESTAMP;
