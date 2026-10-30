-- Establish a stable, versioned identity for repository-managed knowledge.
-- User-contributed links and files keep their existing lifecycle; official
-- articles are updated only by a new migration with a higher revision.
ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS source_origin VARCHAR(24) NOT NULL DEFAULT 'user';

ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS source_key VARCHAR(128);

ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS revision INTEGER NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_knowledge_items_source_origin'
          AND conrelid = 'knowledge_items'::regclass
    ) THEN
        ALTER TABLE knowledge_items
            ADD CONSTRAINT chk_knowledge_items_source_origin
            CHECK (source_origin IN ('user', 'official_catalog'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_knowledge_items_revision'
          AND conrelid = 'knowledge_items'::regclass
    ) THEN
        ALTER TABLE knowledge_items
            ADD CONSTRAINT chk_knowledge_items_revision
            CHECK (revision > 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_knowledge_items_official_key'
          AND conrelid = 'knowledge_items'::regclass
    ) THEN
        ALTER TABLE knowledge_items
            ADD CONSTRAINT chk_knowledge_items_official_key
            CHECK (
                source_origin <> 'official_catalog'
                OR NULLIF(BTRIM(source_key), '') IS NOT NULL
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_knowledge_items_type'
          AND conrelid = 'knowledge_items'::regclass
    ) THEN
        ALTER TABLE knowledge_items
            ADD CONSTRAINT chk_knowledge_items_type
            CHECK (type IN ('link', 'file', 'article'));
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_knowledge_items_source_key
    ON knowledge_items(source_key)
    WHERE source_key IS NOT NULL AND BTRIM(source_key) <> '';

CREATE INDEX IF NOT EXISTS idx_knowledge_items_source_origin
    ON knowledge_items(source_origin);

INSERT INTO knowledge_categories (name, is_system, created_at)
SELECT seed.name, TRUE, CURRENT_TIMESTAMP
FROM (VALUES
    ('工作室制度'),
    ('项目与开发'),
    ('常见问题')
) AS seed(name)
WHERE NOT EXISTS (
    SELECT 1 FROM knowledge_categories existing
    WHERE LOWER(existing.name) = LOWER(seed.name)
);

WITH actor AS (
    SELECT
        COALESCE((SELECT id FROM users WHERE role = 'super_admin' ORDER BY id LIMIT 1), 0) AS id,
        COALESCE((SELECT nickname FROM users WHERE role = 'super_admin' ORDER BY id LIMIT 1), '系统知识库') AS name
), catalog(source_key, revision, name, category_name, body) AS (
    VALUES
    (
        'official.checkin-policy',
        1,
        '打卡、签退与异常处理规则',
        '工作室制度',
        $doc$# 打卡、签退与异常处理规则

> 依据：当前打卡领域逻辑、接口权限与自动签退任务。适用于 SWPUCAT 内的工时记录。

## 正常流程

1. 开始工作时执行上班打卡，结束工作时执行下班打卡。
2. 系统只把同一天内按时间顺序配对成功的“上班—下班”记录计入工时。
3. 当天仍处于上班状态的记录会在日终任务中按 23:59:59 自动签退；成员仍应在实际结束工作时主动签退。
4. 助手的打卡写操作使用幂等请求，重复提交同一请求不会重复生成记录。

## 异常说明

系统会标记重复上班打卡、没有对应上班记录的签退、时间先后无效、缺少签退以及未知记录类型。异常记录不会被助手自行猜测或补齐；需要更正时应联系有权限的队长或超级管理员核对并补签。

## 查询口径

本人历史记录和本人周/月累计只返回当前登录账号的数据。团队统计仅队长或超级管理员可用。$doc$
    ),
    (
        'official.work-hour-requirements',
        1,
        '每周工时要求与达标口径',
        '工作室制度',
        $doc$# 每周工时要求与达标口径

> 依据：当前工时要求配置与个人工时汇总逻辑。

- 工时要求按年级配置，年级由成员学号中的入学年份结合当前日期推导。
- 达标判断只针对本周，从周一累计到查询当天；本月累计不套用每周目标。
- 系统管理员可调整各年级要求，因此具体小时数必须查询当前配置，不能引用旧截图或让模型自行推算。
- 未配置对应年级、学号无法推导年级或记录存在异常时，系统会明确返回未知原因，不把未知状态当作未达标。
- 达标、尚差分钟数和小时数由后端确定性计算；助手只转述该结果。

可向智能助手询问“我本周工作了多久、还差多少”或“各年级本周要求是多少”。$doc$
    ),
    (
        'official.monthly-report-rules',
        1,
        '月报周期、版本与提交流程',
        '工作室制度',
        $doc$# 月报周期、版本与提交流程

> 依据：当前月报领域模型与权限规则。

1. 月报按 `YYYY-MM` 周期管理，周期状态分为开放、关闭和归档。
2. 周期成员快照决定谁需要提交；豁免成员必须记录原因。
3. 报告先创建为草稿。每次编辑形成递增版本，旧版本保留，不能原地覆盖历史版本。
4. 只有正文非空的草稿版本可以提交；提交后可由有权限的管理者审核通过或退回。
5. 成员只能读取自己的报告材料；团队材料只对队长或超级管理员开放。
6. 使用智能助手总结月报前，应先查询周期列表并选定准确周期；不得猜测内部周期 ID。

当前周期、截止时间和提交状态是动态数据，应以系统查询结果为准。$doc$
    ),
    (
        'official.project-workflow',
        1,
        '项目变更与交付流程',
        '项目与开发',
        $doc$# 项目变更与交付流程

> 依据：本仓库的分层结构、迁移器、Docker Compose 部署方式和现有测试命令。

## 推荐顺序

1. 明确需求、权限边界和验收条件，先检查工作区与现有实现。
2. 将改动拆成可独立验证、可独立回滚的步骤；不要混入无关修改。
3. 业务规则放在领域层或应用层，HTTP 层负责鉴权、校验和协议映射，基础设施层负责数据库及外部服务。
4. 数据库结构只通过新的、前向 SQL 迁移演进；已应用迁移受校验和保护，不得修改旧文件。
5. 每一步运行相关 Go 测试；影响前端时运行 `npm run build`，提交前运行 `git diff --check`。
6. 部署前校验环境变量和 Compose 配置，再构建并启动服务；迁移服务成功后主服务才会启动。

密钥、真实账号和生产数据不得写入提交、日志或知识正文。$doc$
    ),
    (
        'official.development-standards',
        1,
        '开发、测试与数据库变更规范',
        '项目与开发',
        $doc$# 开发、测试与数据库变更规范

> 依据：README、Go/Vue 项目结构以及 `cmd/migrate` 的校验行为。

- 后端采用领域、应用、基础设施、接口四层；新增能力应依赖领域接口，而不是从 Handler 直接访问数据库。
- 所有用户身份和角色来自服务端认证上下文，不采信聊天文本、请求体或前端隐藏字段中的身份声明。
- 数据库迁移必须可在新库和已有库上前向执行；迁移文件应用后不得改写，否则校验和检查会拒绝启动。
- 新增业务分支至少覆盖成功、输入边界、权限拒绝、空结果和失败回滚测试。
- Go 代码提交前运行 `go test -buildvcs=false ./...`；前端改动运行 `npm run build`。
- 外部模型、邮件、数据库和内部服务凭据只能通过环境变量配置；示例文件只保留占位值。
- 写操作应有幂等或确认机制，并保留可追踪的业务对象与最终状态。$doc$
    ),
    (
        'official.assistant-faq',
        1,
        '智能助手常见问题与能力边界',
        '常见问题',
        $doc$# 智能助手常见问题与能力边界

> 依据：当前 Agent 工具注册、角色校验与知识引用规则。

## 成员可以做什么

成员可查询本人今日状态、周/月累计、最近打卡记录、当前工时要求和月报周期；写工具启用时可执行本人上班或下班打卡。知识问答只使用已审核资料，并在答案中给出 `[知识#ID]` 引用。

## 管理角色可以做什么

队长和超级管理员还可查询团队确定性统计、按工时和月报状态筛选成员、读取团队月报材料并创建公告草稿。公告草稿不会自动发布或发送邮件。

## 助手不会做什么

- 不会因用户在文本里自称管理员而提升权限。
- 找不到可靠来源时应明确说明未找到，不编造制度。
- 不猜测月报周期 ID，不把不完整打卡记录强行换算为工时。
- 发布公告、发送邮件、删除成员和正式考评等高影响操作仍由人工界面确认。

若问题包含动态状态，应优先调用业务工具；静态文章仅解释规则和流程。$doc$
    )
)
INSERT INTO knowledge_items (
    type, name, description, url, file_size, file_key,
    category_id, category_name, uploader_id, uploader_name,
    status, approved, rejected, reject_reason, reviewer_id, reviewer_name,
    source_origin, source_key, revision, deleted_at, deleted_by, created_at
)
SELECT
    'article', catalog.name, catalog.body, '', '', '',
    category.id, category.name, actor.id, '系统知识库',
    'approved', TRUE, FALSE, '', actor.id, actor.name,
    'official_catalog', catalog.source_key, catalog.revision, NULL, 0, CURRENT_TIMESTAMP
FROM catalog
CROSS JOIN actor
JOIN LATERAL (
    SELECT id, name
    FROM knowledge_categories
    WHERE LOWER(name) = LOWER(catalog.category_name)
    ORDER BY id
    LIMIT 1
) AS category ON TRUE
ON CONFLICT (source_key)
WHERE source_key IS NOT NULL AND BTRIM(source_key) <> ''
DO UPDATE SET
    type = EXCLUDED.type,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    url = EXCLUDED.url,
    file_size = EXCLUDED.file_size,
    file_key = EXCLUDED.file_key,
    category_id = EXCLUDED.category_id,
    category_name = EXCLUDED.category_name,
    status = 'approved',
    approved = TRUE,
    rejected = FALSE,
    reject_reason = '',
    reviewer_id = EXCLUDED.reviewer_id,
    reviewer_name = EXCLUDED.reviewer_name,
    source_origin = EXCLUDED.source_origin,
    revision = EXCLUDED.revision,
    deleted_at = NULL,
    deleted_by = 0;

-- Queue the new descriptions immediately. The periodic reconciler remains the
-- fallback and will replace the empty fingerprint with the current config.
INSERT INTO knowledge_index_jobs (
    knowledge_item_id, status, source_fingerprint, attempts,
    available_at, locked_at, last_error, created_at, updated_at
)
SELECT id, 'queued', '', 0, CURRENT_TIMESTAMP, NULL, '', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM knowledge_items
WHERE source_origin = 'official_catalog'
ON CONFLICT (knowledge_item_id) DO UPDATE SET
    status = 'queued',
    source_fingerprint = '',
    attempts = 0,
    available_at = CURRENT_TIMESTAMP,
    locked_at = NULL,
    last_error = '',
    updated_at = CURRENT_TIMESTAMP;
