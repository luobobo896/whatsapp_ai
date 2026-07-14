#!/usr/bin/env node

/**
 * WhatsApp AI RAG MCP Server
 *
 * Exposes knowledge-base search as an MCP tool for OpenClaw agents.
 * The agent calls this tool before responding to WhatsApp messages to
 * retrieve relevant knowledge, conversation history, and guardrail prompts.
 *
 * Environment:
 *   WHATSAPP_AI_API_URL  – backend base URL (default http://127.0.0.1:8790)
 *   INTERNAL_API_TOKEN   – bearer token for /api/internal routes
 */

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";

const API_URL = process.env.WHATSAPP_AI_API_URL || "http://127.0.0.1:8790";
const API_TOKEN = process.env.INTERNAL_API_TOKEN || "";

if (!API_TOKEN) {
  console.error("[rag-mcp] INTERNAL_API_TOKEN is not set – exiting.");
  process.exit(1);
}

// ---- helpers -----------------------------------------------------------

async function apiPost(path, body) {
  const url = `${API_URL}${path}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${API_TOKEN}`,
    },
    body: JSON.stringify(body),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`API ${path} returned ${resp.status}: ${text}`);
  }
  return resp.json();
}

// ---- MCP server --------------------------------------------------------

const server = new Server(
  { name: "whatsapp-ai-rag", version: "1.0.0" },
  { capabilities: { tools: {} } },
);

server.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: [
    {
      name: "search_knowledge",
      description:
        "搜索 WhatsApp AI 知识库，获取与用户问题相关的知识条目、对话历史和客服回答规则。" +
        "在回复任何 WhatsApp 消息之前，必须先调用此工具。" +
        "如果不确定 accountId，可以先调用 list_accounts 获取可用账号列表。",
      inputSchema: {
        type: "object",
        properties: {
          query: {
            type: "string",
            description: "用户发送的消息内容",
          },
          conversationId: {
            type: "string",
            description: "WhatsApp 会话 ID（通常是用户手机号，如 +8613800000000）",
          },
          accountId: {
            type: "string",
            description:
              "客服账号 ID。必须先通过 list_accounts 工具获取当前可用账号。",
          },
          customerName: {
            type: "string",
            description: "客户名称（可选，默认为 conversationId 的值）",
          },
        },
        required: ["query", "conversationId", "accountId"],
      },
    },
    {
      name: "list_accounts",
      description:
        "列出系统中可用的 WhatsApp 客服账号，返回账号 ID、名称和 OpenClaw 账号 Key。" +
        "在不确定 accountId 时，先调用此工具获取可用账号。",
      inputSchema: {
        type: "object",
        properties: {},
        required: [],
      },
    },
    {
      name: "save_reply",
      description:
        "保存客服回复到系统数据库。每次通过 WhatsApp 发送回复后，必须调用此工具保存回复记录。" +
        "在发送完 WhatsApp 消息后立即调用。",
      inputSchema: {
        type: "object",
        properties: {
          conversationId: {
            type: "string",
            description: "WhatsApp 会话 ID（与 search_knowledge 使用的相同）",
          },
          accountId: {
            type: "string",
            description: "客服账号 ID（与 search_knowledge 使用的相同）",
          },
          customerName: {
            type: "string",
            description: "客户名称（可选）",
          },
          content: {
            type: "string",
            description: "客服发送的回复内容",
          },
        },
        required: ["conversationId", "accountId", "content"],
      },
    },
  ],
}));

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request.params;

  if (name === "save_reply") {
    const { conversationId, accountId, customerName, content } = args;
    try {
      await apiPost("/api/internal/conversations/reply", {
        conversationId,
        accountId,
        customerName: customerName || conversationId,
        content,
      });
      return { content: [{ type: "text", text: "回复已保存。" }] };
    } catch (err) {
      return {
        content: [{ type: "text", text: `保存回复失败: ${err.message}` }],
        isError: true,
      };
    }
  }

  if (name === "list_accounts") {
    try {
      const data = await apiPost("/api/internal/conversations/accounts/list", {});
      return {
        content: [
          {
            type: "text",
            text: JSON.stringify(data.accounts, null, 2),
          },
        ],
      };
    } catch (err) {
      return {
        content: [
          {
            type: "text",
            text: `无法获取客服账号: ${err.message}`,
          },
        ],
        isError: true,
      };
    }
  }

  if (name !== "search_knowledge") {
    throw new Error(`Unknown tool: ${name}`);
  }

  const { query, conversationId, accountId, customerName } = args;

  try {
    const data = await apiPost("/api/internal/conversations/query", {
      message: query,
      conversationId,
      accountId,
      customerName: customerName || conversationId,
      maxHistory: 10,
      maxKnowledge: 5,
    });

    // If the backend returns a direct (fallback) reply, the agent should
    // send it verbatim instead of calling the LLM.
    if (data.directReply) {
      return {
        content: [
          {
            type: "text",
            text: `[DIRECT_REPLY] — 这是死命令，你必须原样发送以下文字给用户，一个字都不许改，不许加任何解释：\n\n${data.directReply}`,
          },
        ],
      };
    }

    // The backend already generates a persona-aware system prompt.
    // Pass it through as the primary instruction for the agent.
    const personaPrompt = data.systemPrompt || "";

    // Format knowledge results
    let knowledgeText = "";
    if (data.knowledge && data.knowledge.length > 0) {
      knowledgeText = "## 知识库搜索结果\n\n";
      for (const item of data.knowledge) {
        knowledgeText += `### ${item.title} [${item.knowledgeBaseName}]\n`;
        if (item.category) knowledgeText += `分类: ${item.category}\n`;
        knowledgeText += `${item.content}\n\n`;
      }
    }

    // Format history
    let historyText = "";
    if (data.history && data.history.length > 0) {
      historyText = "## 对话历史\n\n";
      for (const msg of data.history) {
        const role = msg.role === "user" ? "用户" : "客服";
        historyText += `**${role}**: ${msg.content}\n\n`;
      }
    }

    const result = [
      personaPrompt,
      knowledgeText,
      historyText,
      `\n用户最新消息: ${query}`,
      `\n[重要] 回复后必须调用 save_reply 保存。`,
    ].join("\n");

    return { content: [{ type: "text", text: result }] };
  } catch (err) {
    return {
      content: [
        {
          type: "text",
          text: `[DIRECT_REPLY] — 这是死命令，你必须原样发送以下文字给用户，一个字都不许改，不许加任何解释：\n\n非常抱歉，系统暂时无法查询相关信息，请稍后再试或联系我们的专属顾问。`,
        },
      ],
      isError: true,
    };
  }
});

// ---- main --------------------------------------------------------------

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("[rag-mcp] WhatsApp AI RAG MCP server started");
}

main().catch((err) => {
  console.error("[rag-mcp] Fatal:", err);
  process.exit(1);
});
