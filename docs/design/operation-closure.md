# 运营闭环

## 人工客服回复

人工客服在会话工作台发送文本时，服务端只接受当前租户中已连接的客服账号，并将会话标识规范化为 E.164 WhatsApp 电话号码。服务端调用：

```sh
openclaw message send --channel whatsapp --account <accountKey> --target <phone> --message <content> --json
```

OpenClaw 成功确认后，回复才写入 `conversation_messages`。非电话号码的历史会话会保留可读，但不能误发给任意目标。

每日回复限额会在调用 OpenClaw 前预检，达到限额时不会向客户发送新消息。

## 账号绑定

二维码登录前先检查目标 OpenClaw 账号是否已是 `running` 且 `connected`。已连接的账号直接复用，不重新生成二维码、不强制登录。首次绑定时可创建缺失的 `~/.openclaw/openclaw.json` 配置目录和账号条目。

二维码桥接不再使用强制登录，避免同一 WhatsApp 凭据被重复接入不同账号时触发 OpenClaw 的冲突断开。

OpenClaw 只有一个 gateway 进程，因此扫码完成后的“注册账号、重启、等待连接”会串行执行，避免多个账号同时激活时互相中断。

断开连接会先清除 WhatsApp 凭据，再移除该账号的知识库路由并将数据库状态改为待连接。OpenClaw 会热重载配置，HTTP 请求不再同步等待一次完整 gateway 重启。

删除客服账号是不可逆操作：服务先将账号标记为停用，再注销 WhatsApp、删除对应 OpenClaw Agent 和账号路由，最后以单一数据库事务删除客服账号及其专属会话消息；账号绑定过的知识库保留。删除通常在请求内完成，慢速 OpenClaw 清理会返回“正在删除”并在后台安全续跑。

## 知识库导入

知识库详情页支持一次选择多个文件。单次最多 20 个文件、500 条知识，每个文件不超过 5MB。支持：

- `.txt` / `.md`：每个文件创建一条知识，文件名为标题。
- `.csv`：必须有 `title`、`content` 列；可选 `category`、`attributes` 列，其中 `attributes` 是 JSON。
- `.json`：知识数组，每项包含 `title`、`content`，可选 `category`、`attributes`；`attributes` 可为 JSON 对象或 JSON 字符串。

所有文件会先完整校验，再使用单一数据库事务创建文章和检索分片。任一文件或条目无效时，不会写入部分导入结果。
