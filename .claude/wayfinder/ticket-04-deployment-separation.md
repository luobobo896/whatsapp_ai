---
parent: map.md
type: task
status: closed
assignee: agent
resolution: split-scripts-remove-watchdog
---

## Question

How should the deployment be structured so that OpenClaw is an external dependency (component), not bundled into the admin backend deployment?

## Resolution

**拆分为两个独立服务：管理后台不管理 OpenClaw 的生命周期。**

### 当前问题

| 文件 | 问题行 | 问题描述 |
|------|--------|---------|
| `scripts/start-all.sh:18-23` | `kill -9` 两个端口 | 强制杀死 OpenClaw 进程 |
| `scripts/start-all.sh:29` | 启动 `openclaw gateway` | 管理后台脚本启动了 OpenClaw |
| `src/server.js:1053-1062` | watchdog interval | 每 30s 检查 OpenClaw 状态，超时自动重启 |
| `src/openclaw-bridge.js:109-121` | `scheduleGatewayRestart()` | 管理后台代码中硬编码了 gateway 重启逻辑 |

### 决定：三项修改

#### 1. 拆分启动脚本

**`scripts/start-all.sh` → 保留但改为调用两个独立脚本**：
```bash
bash scripts/start-admin.sh   # 只启动管理后台
bash scripts/start-gateway.sh  # 只启动 OpenClaw
```

**`scripts/start-admin.sh`** (新建):
- 检查 Node.js 环境
- `npm install`（如需）
- 生成 AGENTS.md
- 启动 `node src/server.js`（端口 8790）
- **不触碰** OpenClaw 进程或端口 18789

**`scripts/start-gateway.sh`** (新建):
- 检查 OpenClaw CLI 是否已安装
- 验证 `~/.openclaw-whatsapp-poc/openclaw.json` 存在
- 启动 `openclaw --profile whatsapp-poc gateway run`

#### 2. 移除管理后台的 Gateway 看门狗

删除 `src/server.js` 中的这段代码：
```javascript
// Watchdog: restart gateway if it's down (LINE 1052-1062)
setInterval(() => {
  const cs = openclaw.getChannelStatus();
  if (cs.refreshedAt) {
    const age = Date.now() - new Date(cs.refreshedAt).getTime();
    if (age > 120_000) {
      console.error("Gateway appears down...");
      openclaw.scheduleGatewayRestart();
    }
  }
}, 30_000);
```

替代方案：管理后台检测到 OpenClaw 不可用时，在 UI 中显示警告（已在 `/admin-api/health/gateway` 中实现），但不自动重启。

#### 3. scheduleGatewayRestart → 降级为可选提示

`openclaw-bridge.js` 中的 `scheduleGatewayRestart()` 改为：
- 不再自动执行 `kill` + `nohup openclaw gateway run`
- 仅在配置变更后**提示**用户需要手动重启 gateway
- 或者提供可选的重启按钮（通过管理后台 API 手动触发）

```javascript
// 旧代码：自动重启 gateway
export function scheduleGatewayRestart() { ... kill + nohup ... }

// 新代码：标记需要重启，由人工触发
let restartNeeded = false;
export function markRestartNeeded() { restartNeeded = true; }
export function isRestartNeeded() { return restartNeeded; }
export function performGatewayRestart() { ... } // 仅当用户明确触发时调用
```

### 部署模型

```
┌─────────────────────────────────────┐
│              宿主机                  │
│                                     │
│  ┌──────────────┐  ┌─────────────┐  │
│  │ 管理后台 :8790 │  │ OpenClaw    │  │
│  │              │  │ Gateway     │  │
│  │ node server  │  │ :18789      │  │
│  │              │  │             │  │
│  │ 配置管理      │  │ WhatsApp    │  │
│  │ 知识库编辑    │  │ Web 连接    │  │
│  │ 问答历史      │  │ Agent 路由   │  │
│  │ 数据存储      │  │ LLM 调用    │  │
│  └──────────────┘  └─────────────┘  │
│         │                 │         │
│         └───────┬─────────┘         │
│                 │                   │
│         只读 + OpenClaw CLI          │
│         (不管理生命周期)              │
└─────────────────────────────────────┘
```

### 验证标准

1. `start-admin.sh` 单独启动，OpenClaw 未运行时管理后台仍可访问（显示 "gateway down" 状态）
2. `start-gateway.sh` 单独启动 OpenClaw
3. 管理后台 UI 显示 OpenClaw 连接状态而不尝试自动重启
4. `start-all.sh` 作为便捷脚本仍然可用，但内部调用两个独立脚本
