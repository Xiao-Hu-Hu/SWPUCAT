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
| 后端 | Go、Gin、GORM、goldmark |
| 数据库 | PostgreSQL 16 |
| 部署 | Docker Compose、Nginx（可选） |

## 项目结构

```
SWPUCAT/
├── cmd/server/              # 应用入口
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

# 超级管理员密码（建议修改）
CSA_SUPER_ADMIN_PASSWORD=your_admin_password
```

### 3. 启动服务

```bash
cd deployments
docker compose up -d --build
```

首次启动会自动：
- 拉取 PostgreSQL、Node、Go 镜像
- 构建后端服务
- 执行数据库迁移脚本
- 安装前端依赖

### 4. 访问系统

| 服务 | 地址 |
|------|------|
| 前端页面 | http://localhost:3000 |
| 后端 API | http://localhost:8080 |

默认超级管理员账号：
- 用户名：`admin`（或 `.env` 中配置的 `CSA_SUPER_ADMIN_USERNAME`）
- 密码：`admin123`（或 `.env` 中配置的 `CSA_SUPER_ADMIN_PASSWORD`）

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
cd deployments
docker compose up -d --build
```

## 环境变量说明

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `CSA_SERVER_PORT` | 服务端口 | `8080` |
| `CSA_SERVER_MODE` | 运行模式 (debug/release) | `debug` |
| `CSA_DATABASE_HOST` | 数据库主机 | `localhost` |
| `CSA_DATABASE_PORT` | 数据库端口 | `5432` |
| `CSA_DATABASE_USER` | 数据库用户 | `csa` |
| `CSA_DATABASE_PASSWORD` | 数据库密码 | `csa_password` |
| `CSA_DATABASE_DBNAME` | 数据库名称 | `csa_db` |
| `CSA_JWT_ACCESS_SECRET` | JWT 访问密钥 | - |
| `CSA_JWT_REFRESH_SECRET` | JWT 刷新密钥 | - |
| `CSA_JWT_ACCESS_EXPIRY` | 访问令牌有效期 | `24h` |
| `CSA_SUPER_ADMIN_USERNAME` | 超级管理员用户名 | `admin` |
| `CSA_SUPER_ADMIN_PASSWORD` | 超级管理员密码 | `admin123` |
| `CSA_STORAGE_UPLOAD_DIR` | 文件上传目录 | `./uploads` |
| `CSA_STORAGE_MAX_FILE_SIZE` | 最大文件大小 (字节) | `104857600` |
| `CSA_EMAIL_SMTP_HOST` | SMTP 服务器地址 | - |
| `CSA_EMAIL_SMTP_PORT` | SMTP 端口 | `587` |
| `CSA_EMAIL_USERNAME` | SMTP 用户名 | - |
| `CSA_EMAIL_PASSWORD` | SMTP 密码 | - |
| `CSA_EMAIL_FROM` | 发件人地址 | - |
| `CSA_LOG_LEVEL` | 日志级别 | `debug` |
| `CSA_LOG_FORMAT` | 日志格式 | `console` |

## API 接口

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
| GET | `/api/knowledge/items/my` | 获取我的资源 |
| GET | `/api/knowledge/items/:id` | 获取资源详情 |
| POST | `/api/knowledge/links` | 添加链接 |
| POST | `/api/knowledge/files` | 上传文件 |
| DELETE | `/api/knowledge/items/:id` | 删除资源 |
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
