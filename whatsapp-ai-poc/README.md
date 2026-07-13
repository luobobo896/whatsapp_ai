# WhatsApp AI POC

两个 WhatsApp 号自动问答的最小验证项目。

## 快速开始

```bash
cd /path/to/whatsapp-ai-poc
cp .env.example .env
npm install
npm start
```

## API 路由

所有业务 API 使用 `/api/*` 路由（SQLite 后端）。部分遗留端点仍保留 `/admin-api/*` 前缀用于兼容。

### Accounts `/api/accounts`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/accounts` | 列出所有账号 |
| POST | `/api/accounts` | 创建新账号 |
| PUT | `/api/accounts/:key` | 更新账号 |
| DELETE | `/api/accounts/:key` | 删除账号 |
| POST | `/api/accounts/:key/qr` | 触发 QR 登录 |
| GET | `/api/accounts/:key/qr-status` | 查询 QR 状态 |
| PUT | `/api/accounts/:key/toggle` | 启用/禁用账号 |

### Knowledge `/api/knowledge/*`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/knowledge/roles` | 角色列表 |
| POST | `/api/knowledge/roles` | 创建角色 |
| PUT | `/api/knowledge/roles/:id` | 更新角色 |
| DELETE | `/api/knowledge/roles/:id` | 删除角色 |
| GET | `/api/knowledge/bases` | 知识库列表 |
| POST | `/api/knowledge/bases` | 创建知识库 |
| PUT | `/api/knowledge/bases/:id` | 更新知识库 |
| DELETE | `/api/knowledge/bases/:id` | 删除知识库 |
| GET | `/api/knowledge/bases/:id/entries` | 知识条目（分页） |
| POST | `/api/knowledge/bases/:id/entries` | 创建条目 |
| POST | `/api/knowledge/bases/:id/import-csv` | CSV 导入 |
| PUT | `/api/knowledge/entries/:id` | 更新条目 |
| DELETE | `/api/knowledge/entries/:id` | 删除条目 |
| GET | `/api/knowledge/categories` | 分类列表 |
| POST | `/api/knowledge/categories` | 创建分类 |
| DELETE | `/api/knowledge/categories/:id` | 删除分类 |

### Models `/api/models`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/models` | 读取模型配置 |
| POST | `/api/models` | 保存模型配置 |

### Messages `/api/messages`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/messages` | 会话列表（分页） |
| GET | `/api/messages/export` | 导出 CSV |

### Customers `/api/customers`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/customers` | 客户列表（分页，可按账号筛选） |

### Anti-Ban `/api/antiban`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/antiban` | 读取防封配置 |
| POST | `/api/antiban` | 保存防封配置 |

### Settings `/api/settings`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/settings` | 读取系统设置 |
| POST | `/api/settings` | 保存系统设置 |

## 验证

```bash
# 启动服务器
npm start

# 测试两个号
npm run test:route-a
npm run test:route-b

# 启动 HTTPS 隧道
npm run tunnel
```

详细步骤见 `docs/poc-guide.md`。
