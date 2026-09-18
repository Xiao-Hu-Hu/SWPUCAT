# SWPUCAT

CSA（Computer Science Association）实验室管理系统 — 基于 Vue 3 + Go 的高校计算机协会/实验室日常管理平台。

## 功能模块

### 签到打卡
- 签到 / 签退打卡
- 实时工时统计，逐字符滚动动画
- 今日 / 本周 / 本月工时图表（ECharts）
- 成员在线状态
- 每日打卡记录表格（分页）
- 零点自动签退

### 打卡管理（队长 / 管理员）
- **补签**：为指定成员在某天补录打卡时长，仅限本周内日期
- **打卡要求**：按年级设置每周最低打卡时长（小时），年级从学号前4位自动识别
- **发布报告**：一键生成本周打卡统计报告，自动发布为公告（Markdown 表格）

### 知识库
- 链接与文件上传
- 分类管理
- 队长审批流程（成员上传需审批，队长直接通过）
- 文件下载（带进度条）
- Markdown 描述说明（支持预览渲染）

### 成员管理
- 学号注册（12位数字校验）
- 角色权限：超级管理员 / 队长 / 成员
- 邀请码注册机制
- 成员信息展示与在线状态
- 技术方向标签（Java后端、Go后端、Web前端、Python机器学习、Python深度学习、游戏开发）

### 公告系统
- 公告发布与置顶
- Markdown 渲染（标题、表格、代码块、列表等）
- 查看详情弹窗
- 队长 / 管理员可管理
- **邮件通知**：发布公告后可选择成员发送邮件通知，按年级分组一键选择，HTML 格式支持 Markdown 渲染

### 邮箱验证
- 注册时邮箱验证码校验
- SMTP 邮件发送

### 滑块验证码
- 注册时滑块验证，防止机器人注册

## 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | Vue 3、TypeScript、Element Plus、ECharts、Pinia、Vue Router、marked |
| 后端 | Go、Gin、GORM、goldmark、Eino |
| 数据库 | PostgreSQL 16 |
| 部署 | Docker Compose、Nginx（可选） |

## 项目结构

```
SWPUCAT/
├── cmd/server/              # 主站应用入口
├── cmd/agent/               # Eino Agent 入口
├── cmd/agent_eval/          # Agent 角色化评测入口
├── cmd/migrate/             # 数据库迁移入口
├── internal/
│   ├── application/         # 应用层（服务、DTO）
│   │   ├── approval/        # 审批服务
│   │   ├── checkin/         # 签到服务
│   │   ├── knowledge/       # 知识库服务
│   │   └── user/            # 用户服务
│   ├── domain/              # 领域层（实体、值对象、仓储接口）
│   │   ├── checkin/
│   │   ├── knowledge/
│   │   ├── shared/
│   │   └── user/
│   ├── infrastructure/      # 基础设施层
│   │   ├── auth/            # JWT 认证
│   │   ├── config/          # 配置加载
│   │   ├── database/        # GORM 模型
│   │   ├── email/           # SMTP 邮件
│   │   ├── repository/      # 仓储实现
│   │   └── storage/         # 本地文件存储
│   └── interfaces/          # 接口层
│       └── http/            # HTTP Handler、路由、中间件
├── web/                     # 前端 Vue 项目
│   ├── src/
│   │   ├── api/             # API 请求
│   │   ├── components/      # 公共组件
│   │   ├── router/          # 路由配置
│   │   ├── stores/          # Pinia 状态管理
│   │   └── views/           # 页面视图
│   └── package.json
├── migrations/              # SQL 数据库迁移脚本
├── deployments/             # Docker 部署配置
│   ├── docker-compose.yml
│   └── Dockerfile
├── .env.example             # 环境变量示例
└── go.mod
```

## 快速开始

### 环境要求

- Docker 20+
- Docker Compose v2
- Git

### 1. 克隆项目

```bash
git clone https://github.com/Xiao-Hu-Hu/SWPUCAT.git
cd SWPUCAT
```

### 2. 配置环境变量

```bash
cp .env.example .env
```

编辑 `.env` 文件，修改以下关键配置：

```ini
# 数据库密码（必须修改）
CSA_DATABASE_PASSWORD=your_secure_password

# JWT 密钥（必须修改，使用随机字符串）
CSA_JWT_ACCESS_SECRET=your_random_secret_at_least_32_chars
CSA_JWT_REFRESH_SECRET=your_random_secret_at_least_32_chars

# Agent（启用时必须填写）
CSA_AGENT_ENABLED=true
CSA_AGENT_WRITE_ENABLED=false
CSA_AGENT_INTERNAL_SECRET=your_random_internal_secret_at_least_32_chars
CSA_LLM_PROVIDER=openai_compatible
CSA_LLM_MODEL=your_model_id
CSA_LLM_API_KEY=your_model_api_key
CSA_LLM_BASE_URL=https://your-provider.example/v1
```

### 3. 启动服务

```bash
docker compose --env-file .env -f deployments/docker-compose.yml config --quiet
docker compose --env-file .env -f deployments/docker-compose.yml up -d --build
docker compose --env-file .env -f deployments/docker-compose.yml ps -a
```

首次启动会自动：
- 拉取 PostgreSQL、Node、Go 镜像
- 构建后端服务
- 执行数据库迁移脚本
- 使用锁文件安装前端依赖

### 4. 访问系统

| 服务 | 地址 |
|------|------|
| 前端页面 | http://localhost:3000 |
| 后端 API | http://localhost:8080 |

默认超级管理员账号：
- 用户名：`202400000001`
- 密码：`admin123`

首次登录后应立即修改默认密码。已有数据库继续使用其中的现有账号。

### 5. 升级没有迁移历史的旧数据库

如果数据库里已经存在旧版业务表，但没有 `schema_migrations` 表，请先备份数据库，并在第一次升级时设置：

```ini
CSA_MIGRATION_BASELINE=true
```

迁移器会校验旧版 `001` 至 `015` 所需的表和字段，只将这 15 个旧迁移标记为基线，再正常执行 `016` 之后的新迁移。迁移成功后可将该值恢复为 `false`。

如果修改了 `CSA_DATABASE_PASSWORD` 或 `CSA_DATABASE_DBNAME`，已有 PostgreSQL 数据卷不会自动修改数据库角色密码或创建新数据库。必须先在数据库内同步修改，或者在确认测试数据可删除后重新创建数据卷。

## Agent 角色化评测

仓库内置 54 个 `member`、`captain`、`super_admin` 场景，覆盖生产 13 个工具、权限边界、写开关、引用、提示注入与隐私哨兵。默认命令只做严格离线校验，不读取模型凭据或访问网络：

```bash
make agent-eval
```

只有显式选择 `live` 才会调用已配置的模型；业务 API 始终替换为进程内 fixture，不连接真实主站或数据库：

```bash
go run -buildvcs=false ./cmd/agent_eval -mode=live -suite=v1 -role=all -repeat=3 -format=json
```

数据集结构、评分契约和安全约束见 [`internal/agent/eval/testdata/v1/README.md`](internal/agent/eval/testdata/v1/README.md)。

## 部署到生产服务器

### 使用 Nginx 反向代理（推荐）

```bash
# 安装 Nginx
apt install -y nginx

# 创建配置文件
cat > /etc/nginx/sites-available/swpucat << 'EOF'
server {
    listen 80;
    server_name your_domain_or_ip;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
EOF

ln -s /etc/nginx/sites-available/swpucat /etc/nginx/sites-enabled/
nginx -t && systemctl restart nginx
```

### 阿里云安全组

在 ECS 安全组中放行以下端口：
- **80**（HTTP，Nginx）
- **443**（HTTPS，如需 SSL）

### 后续更新

```bash
cd /opt/swpucat
git pull
docker compose --env-file .env -f deployments/docker-compose.yml up -d --build
```

## 环境变量说明

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `CSA_SERVER_PORT` | 服务端口 | `8080` |
| `CSA_SERVER_MODE` | 运行模式 (debug/release) | `debug` |
| `CSA_DATABASE_HOST` | 数据库主机 | `localhost` |
| `CSA_DATABASE_PORT` | 数据库端口 | `5432` |
| `CSA_DATABASE_USER` | 数据库用户 | `csa` |
| `CSA_DATABASE_PASSWORD` | 数据库密码，必须通过 `.env` 提供 | - |
| `CSA_DATABASE_DBNAME` | 数据库名称 | `csa_db` |
| `CSA_MIGRATION_BASELINE` | 无迁移历史的旧版 `001-015` 数据库首次升级开关 | `false` |
| `CSA_JWT_ACCESS_SECRET` | JWT 访问密钥 | - |
| `CSA_JWT_REFRESH_SECRET` | JWT 刷新密钥 | - |
| `CSA_STORAGE_UPLOAD_DIR` | 文件上传目录 | `./uploads` |
| `CSA_STORAGE_MAX_FILE_SIZE` | 最大文件大小 (字节) | `104857600` |
| `CSA_EMAIL_SMTP_HOST` | SMTP 服务器地址 | - |
| `CSA_EMAIL_SMTP_PORT` | SMTP 端口 | `587` |
| `CSA_EMAIL_USERNAME` | SMTP 用户名 | - |
| `CSA_EMAIL_PASSWORD` | SMTP 密码 | - |
| `CSA_EMAIL_FROM` | 发件人地址 | - |
| `CSA_LOG_LEVEL` | 日志级别 | `debug` |
| `CSA_LOG_FORMAT` | 日志格式 | `console` |
| `CSA_AGENT_ENABLED` | 是否启用 Eino Agent | `false` |
| `CSA_AGENT_WRITE_ENABLED` | 是否注册打卡和公告草稿等写工具 | `false` |
| `CSA_AGENT_INTERNAL_SECRET` | 主站和 Agent 的内部共享密钥，启用时至少 32 位 | - |
| `CSA_AGENT_WORKER_CONCURRENCY` | Agent 同时执行的请求数 | `4` |
| `CSA_AGENT_QUEUE_WAIT_TIMEOUT` | 达到并发上限后的最长排队时间 | `5s` |
| `CSA_AGENT_EXECUTION_TIMEOUT` | 单次 Agent 请求的总执行预算 | `180s` |
| `CSA_AGENT_CONVERSATION_RETENTION_DAYS` | 会话保留天数；`0` 禁用清理 | `0` |
| `CSA_LLM_PROVIDER` | 模型适配器，目前支持 OpenAI-compatible | `openai_compatible` |
| `CSA_LLM_MODEL` | 服务商提供的模型 ID | - |
| `CSA_LLM_API_KEY` | 模型 API Key，仅注入 Agent 容器 | - |
| `CSA_LLM_BASE_URL` | OpenAI-compatible API 根地址 | - |
| `CSA_AGENT_COST_ESTIMATION_ENABLED` | 是否按配置价格估算 Agent 调用费用 | `false` |
| `CSA_AGENT_INPUT_USD_PER_MILLION` | 每百万非缓存输入 Token 的美元价格 | `0` |
| `CSA_AGENT_CACHED_INPUT_USD_PER_MILLION` | 每百万缓存输入 Token 的美元价格 | `0` |
| `CSA_AGENT_OUTPUT_USD_PER_MILLION` | 每百万输出 Token 的美元价格 | `0` |

## API 接口

### Agent 审计（仅超级管理员）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/agent/audits` | 分页查询隐私安全的运行明细 |
| GET | `/api/agent/audits/summary` | 查询调用量、成功率、延迟、Token、费用和工具聚合 |
| GET | `/api/agent/audits/:id` | 查询单次运行及工具调用投影 |

费用为基于当前配置价格的估算值；价格未启用或 Token 上报不完整时返回“未知”，不会按零费用处理。审计记录不包含提示词、回答正文、授权头或原始敏感工具参数。

### Agent 会话

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/agent/conversations` | 查询当前用户的会话列表 |
| POST | `/api/agent/conversations` | 创建当前用户的新会话 |
| GET | `/api/agent/conversations/:session_id` | 读取当前用户的会话和消息 |
| PATCH | `/api/agent/conversations/:session_id` | 修改当前用户的会话标题 |
| DELETE | `/api/agent/conversations/:session_id` | 永久删除当前用户的会话；重复调用幂等 |

会话自动清理默认关闭，清理命令默认只执行 dry-run；数据范围、删除后的审计处理和显式执行方式见 [`docs/agent-conversation-retention.md`](docs/agent-conversation-retention.md)。

### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/auth/register` | 用户注册 |
| POST | `/api/auth/login` | 用户登录 |
| POST | `/api/auth/send-code` | 发送邮箱验证码 |

### 签到

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/checkin/clock-in` | 签到 |
| POST | `/api/checkin/clock-out` | 签退 |
| GET | `/api/checkin/status` | 获取签到状态 |
| GET | `/api/checkin/records` | 获取签到记录 |
| GET | `/api/checkin/stats` | 获取工时统计 |
| GET | `/api/checkin/online` | 获取在线成员 |
| GET | `/api/checkin/today-records` | 获取今日打卡记录 |

### 打卡管理（队长 / 管理员）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/checkin/makeup` | 补签 |
| GET | `/api/checkin/requirements` | 获取打卡要求 |
| POST | `/api/checkin/requirements` | 设置打卡要求 |
| POST | `/api/checkin/report` | 发布打卡报告 |

### 知识库

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/knowledge/items` | 获取资源列表 |
| GET | `/api/knowledge/items/pending` | 获取待审批资源 |
| GET | `/api/knowledge/items/quarantined` | 获取已隔离资源（队长/管理员） |
| GET | `/api/knowledge/items/my` | 获取我的资源 |
| GET | `/api/knowledge/items/:id` | 获取资源详情 |
| POST | `/api/knowledge/links` | 添加链接 |
| POST | `/api/knowledge/files` | 上传文件 |
| DELETE | `/api/knowledge/items/:id` | 删除资源 |
| POST | `/api/knowledge/items/:id/quarantine` | 隔离已审核资源（队长/管理员） |
| POST | `/api/knowledge/items/:id/restore` | 恢复已隔离资源（队长/管理员） |
| PUT | `/api/knowledge/items/:id/approve` | 审批通过 |
| PUT | `/api/knowledge/items/:id/reject` | 审批拒绝 |
| GET | `/api/knowledge/download/:id` | 下载文件 |
| GET | `/api/knowledge/categories` | 获取分类列表 |
| POST | `/api/knowledge/categories` | 创建分类 |
| DELETE | `/api/knowledge/categories/:id` | 删除分类 |

### 成员

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/members` | 获取成员列表 |
| DELETE | `/api/members/:id` | 移除成员 |
| POST | `/api/members/:id/transfer-captain` | 转让队长 |

### 个人设置

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/profile` | 获取个人信息 |
| PUT | `/api/profile` | 更新个人信息 |
| PUT | `/api/profile/password` | 修改密码 |
| PUT | `/api/profile/tech-direction` | 更新技术方向 |
| POST | `/api/profile/avatar` | 上传头像 |

### 公告

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/announcements` | 获取公告列表 |
| POST | `/api/announcements` | 发布公告 |
| PUT | `/api/announcements/:id` | 更新公告 |
| DELETE | `/api/announcements/:id` | 删除公告 |
| POST | `/api/announcements/:id/notify` | 通知成员 |

### 审批

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/approvals` | 获取待审批列表 |
| POST | `/api/approvals` | 提交审批 |
| POST | `/api/approvals/:id/approve` | 审批通过 |
| POST | `/api/approvals/:id/reject` | 审批拒绝 |

### 邀请码

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/invitations/generate` | 生成邀请码 |
| GET | `/api/invitations/my` | 获取我的邀请码 |

## 许可证

MIT License
