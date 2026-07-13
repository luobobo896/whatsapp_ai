/**
 * Generate the main agent's AGENTS.md from SQLite database.
 * Run after any config change. No execFileSync — just write a file.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { rootDir } from "./config.js";
import { db } from "./db.js";

const outputPath = path.join(rootDir, "AGENTS.md");

export function generateAgentPrompt() {
  const accounts = db.prepare("SELECT * FROM accounts").all();
  const roles = db.prepare("SELECT * FROM knowledge_roles").all();

  const roleMap = Object.fromEntries(roles.map(r => [r.id, r]));

  // 账号→角色映射
  const accountEntries = accounts.map(acct => {
    const roleIds = db.prepare("SELECT role_id FROM account_roles WHERE account_id = ?").all(acct.id).map(r => r.role_id);
    const roleNames = roleIds.map(id => roleMap[id]?.name || id).join(", ");
    return `| \`${acct.id}\` | ${acct.label} | ${roleNames || "未分配"} | \`${roleIds.join(", ") || "无"}\` |`;
  });

  // 角色知识 → 从 knowledge_entries 渲染
  const roleSections = roles.map(role => {
    const bases = db.prepare(
      "SELECT kb.* FROM knowledge_bases kb JOIN knowledge_base_roles kbr ON kb.id = kbr.base_id WHERE kbr.role_id = ?"
    ).all(role.id);

    const productLines = [];
    for (const base of bases) {
      const entries = db.prepare("SELECT * FROM knowledge_entries WHERE base_id = ? AND enabled = 1").all(base.id);
      for (const e of entries) {
        let meta = {};
        try { meta = JSON.parse(e.metadata || "{}"); } catch (_) { /* ignore malformed metadata */ }
        const parts = [`- **${e.title}** (${e.id})`];
        if (meta.price) parts.push(`  价格: ${meta.price}`);
        if (meta.sizes?.length) parts.push(`  尺码: ${meta.sizes.join(", ")}`);
        if (meta.colors?.length) parts.push(`  颜色: ${meta.colors.join(", ")}`);
        if (meta.delivery) parts.push(`  发货: ${meta.delivery}`);
        if (e.content) parts.push(`  描述: ${e.content.split("\n")[0]}`);
        if (meta.sellingPoints?.length) parts.push(`  卖点: ${meta.sellingPoints.join("; ")}`);
        if (meta.notes?.length) parts.push(`  备注: ${meta.notes.join("; ")}`);
        if (meta.aliases?.length) parts.push(`  别名: ${meta.aliases.join(", ")}`);
        productLines.push(parts.join("\n"));
      }
    }

    let keywords = [];
    try { keywords = JSON.parse(role.keywords || "[]"); } catch (_) { /* ignore malformed keywords */ }
    return [
      `### ${role.name} (\`${role.id}\`)`,
      `范围: ${role.description}`,
      `关键词: ${keywords.join(", ")}`,
      "",
      "**产品:**",
      ...productLines,
      role.unknown_reply ? `\n**兜底回复:** ${role.unknown_reply}` : ""
    ].join("\n");
  });

  const prompt = `# WhatsApp 智能客服 — 主 Agent

你是 WhatsApp 多账号智能客服。你同时服务多个 WhatsApp 账号，每个账号有独立的产品权限。

## 账号→角色映射

| 账号 Key | 名称 | 允许角色 | 角色 ID |
|----------|------|----------|---------|
${accountEntries.join("\n")}

## 核心规则（必须严格遵守）

### 1. 识别来源
收到消息时，根据消息来源的 WhatsApp 账号（accountId），查上表确定该账号允许哪些角色。

### 2. 权限隔离
- **只能**使用该账号允许的角色知识回答。
- **禁止**使用其他角色的知识，即使你知道。
- **禁止**提及未分配给该账号的角色或产品。
- **禁止**透露其他账号的存在或配置。

### 3. 知识边界
- 知识库里写明的信息 → 可以回答。
- 知识库**没有写明的**（颜色、材质、优惠、退换货、保修等）→ 回复:"这部分需要人工客服确认，我先帮您转人工。"
- **禁止编造、猜测、推理**任何知识库以外的信息。

### 4. 越权处理
客户询问该账号不允许的品类 → 回复:"这部分需要人工客服确认，我先帮您转人工。"

### 5. 回复风格
- 简洁、温暖、直接，一两句话即可。
- 用客户的语言回复（中文问→中文答，英文问→英文答）。
- 不要用 emoji 做结构性图标。

### 6. 安全限制
- **禁止**执行任何代码、Shell 命令、文件操作。
- **禁止**访问外部 URL 或搜索网络。
- 只从下面的知识库中回答。

### 7. 身份保密（极其重要）
- **禁止**透露你是什么 AI 模型、大语言模型、或任何技术架构信息。
- **禁止**提及 DeepSeek、OpenAI、OpenClaw、GPT、Claude 等任何模型或平台名称。
- **禁止**说你"基于知识库"、"由 AI 驱动"、或解释你的工作原理。
- 如果有人问你是谁、是什么模型、怎么工作的 → 只回答："我是客服小助手，有什么可以帮您的？"
- 你的身份只有一个：**客服**。不是 AI，不是机器人，不是助手。"

---

${roleSections.join("\n---\n\n")}
`;

  fs.writeFileSync(outputPath, prompt, "utf8");
  return { roles: roles.length, accounts: accountEntries.length };
}

// CLI entry
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const result = generateAgentPrompt();
  console.log(`Generated AGENTS.md: ${result.roles} roles, ${result.accounts} accounts`);
}
