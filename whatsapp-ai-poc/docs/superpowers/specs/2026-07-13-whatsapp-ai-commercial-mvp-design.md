# WhatsApp 多租户智能客服商业 MVP 设计

日期：2026-07-13  
状态：已完成方案确认，待书面复核  
替代范围：现有 demo 中的 SQLite 单租户数据层、JSON 双写、仅靠 prompt 的知识权限，以及 `/admin-api/*` 遗留业务接口

## 1. 目标与范围

将当前管理后台 demo 改造成可运营、可审计、可恢复的多租户 WhatsApp 智能客服 MVP。

第一阶段必须形成以下闭环：

1. 平台管理员创建租户和首个租户 owner。
2. 租户 owner 管理成员、WhatsApp 账号、知识角色、知识库和运行策略。
3. OpenClaw 运行时事件进入统一消息管线。
4. 服务端强制执行租户隔离、账号知识权限、工作时间和发送限额。
5. 可回答问题使用授权知识回复；越权、未知知识或运行失败进入人工接管。
6. 回复、失败、重试、人工处理、状态变更和配置操作均可追踪、可审计。
7. 管理后台能够查看真实账号状态、会话状态、告警和运营指标。

### 1.1 明确不做

- 在线支付、订单、退款、发票和套餐计费。
- 公开注册和自助创建租户。
- 微服务、Redis 和外部消息队列。
- 同时维护 SQLite 和 PostgreSQL 两种生产数据源。
- 依赖模型自行遵守的软权限隔离。
- 与本目标无关的营销自动化、群发和 CRM 扩展。

## 2. 已确认的产品决策

| 决策 | 结论 |
|---|---|
| 交付范围 | 商业闭环 MVP |
| 消息入口 | OpenClaw 运行时事件 |
| 转人工 | 自动暂停会话，人工显式恢复 AI |
| 管理员模型 | 多管理员、四租户角色 |
| 租户模型 | 多组织 SaaS |
| 计费 | 第一阶段不做 |
| 租户创建 | 平台管理员创建租户和首个 owner |
| 数据库 | PostgreSQL，数据库名 `whatsapp_ai` |
| 前端 | React + Vite |
| 后端 | Fastify 模块化单体 |
| 实时更新 | SSE，断开后退化为定时刷新 |

数据库凭据、会话密钥、OpenClaw 配置和模型密钥只从运行环境或密钥管理系统读取，不进入仓库、日志、API 响应或前端状态。

## 3. 总体架构

```text
OpenClaw WhatsApp Runtime
          |
          | runtime event / send result
          v
+-----------------------------+
| Fastify modular monolith    |
|                             |
| auth       tenants          |
| members    accounts         |
| knowledge  conversations    |
| runtime    audit / metrics  |
|                             |
| API process + DB job worker |
+---------------+-------------+
                |
                v
         PostgreSQL
                ^
                |
       REST API + SSE
                |
         React / Vite Admin
```

### 3.1 模块边界

- `auth`：登录、登出、会话、密码哈希、身份解析。
- `tenants`：平台级租户创建、停用和生命周期。
- `members`：租户成员、邀请和 RBAC。
- `accounts`：WhatsApp 账号配置、扫码、启停和健康状态。
- `knowledge`：知识角色、知识库、条目、导入、检索和授权范围。
- `conversations`：客户、会话、消息、人工回复和状态机。
- `runtime`：OpenClaw 事件适配、回复作业、重试和发送结果。
- `audit`：写操作审计、告警和安全事件。
- `metrics`：基于业务事实表计算聚合指标。

模块可以共享同一数据库连接池，但不能在路由中跨模块拼接业务 SQL。跨模块行为通过服务接口完成，并由服务层持有事务边界。

### 3.2 OpenClaw 边界

所有 OpenClaw 交互由 `OpenClawAdapter` 封装：

- `onInboundMessage(event)`
- `sendMessage(command)`
- `startAccountLink(accountId)`
- `getAccountLinkStatus(accountId)`
- `setAccountEnabled(accountId, enabled)`
- `getAccountHealth(accountId)`

业务模块不直接读取或改写 OpenClaw 配置文件，不直接调用其内部目录，也不散落执行 CLI 命令。适配器错误必须转换成稳定的领域错误。

入站事件至少包含：

- 稳定的 `eventId`
- OpenClaw 侧 `accountId`
- 稳定的 `customerId`
- `sessionId`
- 消息 ID、时间、方向、文本和原始事件摘要

## 4. 身份、租户与权限

### 4.1 身份模型

- `users`：全局用户身份、登录标识、密码哈希和状态。
- `tenants`：租户名称、状态、创建时间和停用原因。
- `tenant_memberships`：用户与租户关系、角色和成员状态。
- `member_invitations`：租户内成员邀请、目标角色、过期时间和使用状态。
- `platform_roles`：平台级管理权限，与租户成员角色分离。

一个用户可以属于多个租户。登录后必须选择或恢复一个已授权租户上下文。平台管理员默认不能查看租户消息、客户和知识正文。

### 4.2 租户角色

| 能力 | owner | admin | agent | viewer |
|---|---|---|---|---|
| 管理成员与角色 | 全部；不能删除最后一个 owner | 只读 | 无 | 无 |
| 管理 WhatsApp 账号 | 全部 | 全部 | 只读状态 | 只读状态 |
| 管理知识与模型 | 全部 | 全部 | 只读知识 | 无模型密钥 |
| 会话与客户 | 全部 | 全部 | 查看、人工回复、转接、恢复、关闭 | 不查看消息正文和客户 PII |
| 设置与安全策略 | 全部 | 全部 | 无 | 无 |
| 指标、告警和审计 | 全部 | 全部 | 与会话相关 | 聚合只读 |

RBAC 必须在服务端执行。前端隐藏按钮只是体验优化，不是权限控制。

### 4.3 会话认证

- 密码使用成熟的密码哈希算法和唯一随机盐。
- 登录成功创建服务端会话，浏览器只保存 `HttpOnly`、`Secure`、`SameSite=Lax` Cookie。
- 会话支持到期、登出、管理员撤销和密码变更后失效。
- 所有写接口执行 CSRF 防护和请求来源校验。
- 不允许默认密码或代码内回退 token。

## 5. 数据模型

除平台级表外，所有业务表必须包含 `tenant_id`。所有业务唯一约束、关联查询和外键关系必须包含租户边界。

### 5.1 核心表组

身份与租户：

- `users`
- `platform_roles`
- `tenants`
- `tenant_memberships`
- `member_invitations`
- `auth_sessions`

账号与知识：

- `whatsapp_accounts`
- `knowledge_roles`
- `account_knowledge_roles`
- `knowledge_bases`
- `knowledge_base_roles`
- `knowledge_entries`
- `knowledge_imports`

客户与会话：

- `customers`
- `conversations`
- `messages`
- `handoffs`

运行与运营：

- `runtime_events`
- `reply_jobs`
- `account_usage_daily`
- `tenant_settings`
- `audit_logs`
- `alerts`

### 5.2 租户隔离

隔离采用两层防护：

1. 应用服务的所有读写显式携带 `tenant_id`。
2. PostgreSQL 为租户业务表启用 Row Level Security。

每个 HTTP 请求和 worker 作业都必须在数据库事务内设置当前租户上下文。平台操作使用独立数据库角色或受控策略，不能复用普通租户旁路。

### 5.3 关键约束

- `runtime_events(provider, event_id)` 唯一，确保入站事件幂等。
- `whatsapp_accounts(tenant_id, external_account_id)` 唯一。
- `customers(tenant_id, account_id, external_customer_id)` 唯一。
- `conversations(tenant_id, account_id, external_session_id)` 唯一。
- `tenant_memberships(tenant_id, user_id)` 唯一。
- 所有跨租户复合外键包含 `tenant_id`。
- 删除最后一个有效 owner 必须由业务约束拒绝。

## 6. 会话状态机

会话状态仅允许：

```text
ai_active -> handoff -> ai_active
ai_active -> closed
handoff   -> closed
```

规则：

- 新会话默认 `ai_active`。
- 越权、未知知识、客户要求人工或连续发送失败时进入 `handoff`。
- 进入 `handoff` 前，系统尝试发送一次客户语言对应的固定转人工文案。
- `handoff` 状态下的新消息只落库并通知人工，不创建 AI 回复作业。
- 人工回复后会话保持 `handoff`，必须显式执行“恢复 AI”。
- `agent`、`admin`、`owner` 可以人工回复、恢复或关闭会话。
- 状态变化必须记录操作者、原因和时间。
- 已关闭会话收到新的 OpenClaw `sessionId` 时创建新会话；相同 `sessionId` 只追加消息。

工作时间外不会改变会话状态。每个会话每天最多发送一次离线回复，工作时间恢复后继续正常处理。

## 7. 消息处理管线

### 7.1 入站阶段

1. 适配器校验 OpenClaw 事件结构。
2. 以 `eventId` 幂等保存 `runtime_events`。
3. 用 `accountId` 定位唯一租户和账号。
4. 在一个事务内写入客户、会话、入站消息和回复作业。
5. 账号禁用、租户停用或会话处于 `handoff` 时，不创建 AI 作业。

### 7.2 策略阶段

worker 使用 `FOR UPDATE SKIP LOCKED` 领取作业，并依次检查：

1. 租户和账号是否可用。
2. 会话是否仍为 `ai_active`。
3. 是否在工作时间。
4. 账号小时、每日发送限额和热身策略。
5. 当前账号允许的知识角色。

达到安全限额后停止自动发送、创建告警，并把受影响会话转人工。计数只能在确认发送成功后增加；重试同一消息不得重复计数。

### 7.3 知识边界

1. 意图判定只读取当前租户的知识角色名称和关键词。
2. 匹配到当前账号未授权角色时直接转人工。
3. 无法确定角色时直接转人工。
4. 检索只能访问当前账号已授权角色关联的启用知识条目。
5. 模型返回结构化结果：`answer | handoff`、回复文本、知识条目 ID 列表和原因。
6. 服务端校验所有引用条目属于当前租户、当前账号授权范围。
7. 没有有效引用、引用越权或结构异常时，使用固定转人工文案。

服务端授权、检索隔离和引用校验缺一不可。模型不能用常识补全知识库没有写明的颜色、材质、优惠、退换货、保修或其他事实。

### 7.4 出站与重试

- 每条出站消息先生成稳定 `clientMessageId` 并写入数据库。
- OpenClaw 发送命令携带该 ID；重复执行同一作业不得产生新的业务消息。
- 消息状态包括 `queued`、`sending`、`sent`、`retrying`、`failed`、`cancelled`。
- 失败按受控退避策略重试；超过次数后创建告警并进入人工接管。
- OpenClaw 不可用时后台和数据维护仍然可用。

## 8. 管理后台

### 8.1 平台工作区

- 租户列表、创建、启用和停用。
- 创建租户首个 owner。
- 平台审计和系统健康。
- 不提供租户消息、客户和知识正文的默认浏览入口。

### 8.2 租户工作区

- 总览。
- WhatsApp 账号。
- 知识角色、知识库和条目。
- 会话收件箱和人工接管。
- 客户。
- 团队成员和邀请。
- 模型配置。
- 工作时间、安全策略和固定回复。
- 告警和审计。

### 8.3 关键交互状态

- 账号状态：`draft`、`linking`、`online`、`reconnecting`、`disabled`、`error`。
- 会话分组：`AI 处理中`、`待人工`、`已关闭`。
- 时间线显示入站、AI 回复、人工回复、发送失败和状态变化。
- CSV 导入先解析和预览错误，确认后才写入。
- 所有写操作显示加载、成功、失败和权限不足状态。
- 敏感字段只允许替换，不回显明文。
- SSE 断开时显示实时连接中断，并退化为定时刷新。

## 9. API 与错误契约

所有业务 API 使用 `/api/*`。现有 `/admin-api/*` 不再承载新业务，迁移完成后删除兼容层。

错误响应使用稳定结构：

```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "You do not have permission to perform this action.",
    "requestId": "req_..."
  }
}
```

第一阶段至少定义：

- `AUTH_REQUIRED`
- `SESSION_EXPIRED`
- `FORBIDDEN`
- `TENANT_SUSPENDED`
- `ACCOUNT_NOT_READY`
- `CONVERSATION_IN_HANDOFF`
- `KNOWLEDGE_NOT_FOUND`
- `RATE_LIMITED`
- `OPENCLAW_UNAVAILABLE`
- `CONFLICT`
- `VALIDATION_FAILED`

前端只根据 `code` 选择交互，不解析后端任意错误字符串。

## 10. 数据迁移

### 10.1 数据库初始化

- PostgreSQL 数据库名固定为 `whatsapp_ai`。
- 连接字符串只通过 `DATABASE_URL` 提供。
- 使用版本化 SQL migrations 创建 schema、约束、索引和 RLS 策略。
- 创建数据库与执行 schema migrations 是两个独立部署步骤。

### 10.2 旧数据导入

当前 SQLite 和 JSON 数据只作为一次性导入源：

1. 创建一个初始租户和首个 owner。
2. dry-run 解析账号、角色、知识库、知识条目、消息和设置。
3. 输出数量、无法映射的数据和冲突，不写数据库。
4. 正式导入在一个事务内执行。
5. 任意错误导致完整回滚。
6. 验证数量和关联关系后，停止旧数据源双写。

导入逻辑使用结构化 CSV 解析器，禁止以 `split(',')` 解析 CSV。

## 11. 审计、告警与指标

### 11.1 审计

审计至少记录：

- `tenant_id`
- 操作者用户和角色
- 动作和目标类型/ID
- 请求 ID
- 结果
- 变更前后摘要，不包含密钥和密码
- IP、User-Agent 和时间

### 11.2 告警

第一阶段告警包括：

- OpenClaw 连接异常。
- WhatsApp 账号离线或重连失败。
- 回复作业最终失败。
- 账号达到发送限额。
- 异常登录或连续认证失败。
- 邀请或管理员权限变更。

### 11.3 指标

指标从 PostgreSQL 业务事实计算，不混用 JSON 日志和 SQLite：

- 入站/出站消息量。
- AI 回复成功率和失败率。
- 转人工数量和比例。
- 待人工会话数量和等待时长。
- 账号健康状态。
- 角色和知识命中分布。

## 12. 错误与恢复

- OpenClaw 不可用：保留入站事件和待处理作业，后台继续可用。
- PostgreSQL 暂时不可用：不确认事件处理成功，由 OpenClaw 重试；不得以内存数据替代落库。
- 重复运行时事件：返回已处理结果，不重复创建消息或回复。
- worker 崩溃：作业租约超时后可由其他 worker 重新领取。
- 知识或模型输出异常：不发送未验证答案，改为转人工。
- 账号配置操作失败：保留期望状态和失败原因，允许重试，不伪装成已完成。

## 13. 测试与验收

### 13.1 自动化测试

- 领域单元测试：权限、状态机、限流和知识边界。
- PostgreSQL 集成测试：约束、事务、RLS 和 migrations。
- RBAC 矩阵测试：每个角色对每个资源的允许和拒绝。
- 租户隔离测试：列表、详情、更新、删除、导出和 SSE。
- OpenClaw 适配器契约测试。
- 消息幂等、作业重试、发送计数和转人工测试。
- Fastify API 测试。
- React 关键组件和权限状态测试。
- 浏览器端到端测试：登录、租户、账号、知识导入、会话接管和恢复。

### 13.2 上线验收

以下条件必须全部满足：

1. 跨租户访问全部拒绝，包含猜测 ID、导出和实时订阅。
2. 重复 OpenClaw 事件不会重复回复。
3. 未授权角色和未知知识必定转人工。
4. `handoff` 后 AI 停止，人工恢复后才继续。
5. OpenClaw 中断期间消息不丢失，恢复后可重试。
6. 账号、知识、会话、审计和指标的数据来源一致。
7. 平台管理员默认无法读取租户消息和知识正文。
8. 所有真实密钥均未进入 Git、日志或浏览器响应。

## 14. 实施分解

本设计按以下顺序交付，每阶段都必须可独立验证：

1. 基础层：Fastify 项目、PostgreSQL migrations、租户、认证、RBAC、审计和旧数据 dry-run。
2. 运行时闭环：OpenClawAdapter、事件幂等、会话状态机、回复作业、限流、知识授权和转人工。
3. 管理后台：React/Vite、平台工作区、租户工作区、会话收件箱和 SSE。
4. 迁移与验收：正式数据导入、浏览器 E2E、故障恢复测试、部署文档和上线检查。

每一阶段通过测试和验收后再进入下一阶段。第一阶段不以“页面看起来完成”作为业务完成标准。
