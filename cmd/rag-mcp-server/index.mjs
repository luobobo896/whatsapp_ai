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
 *   INTERNAL_API_TOKEN       – bearer token for /api/internal routes
 *   WHATSAPP_AI_ACCOUNT_ID   – the single WhatsApp account this MCP process serves
 */

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";

const API_URL = process.env.WHATSAPP_AI_API_URL || "http://127.0.0.1:8790";
const API_TOKEN = process.env.INTERNAL_API_TOKEN || "";
const ACCOUNT_ID = process.env.WHATSAPP_AI_ACCOUNT_ID || "";
const knowledgeByConversation = new Map();
const KNOWLEDGE_CONTEXT_TTL_MS = 5 * 60 * 1000;

function rememberKnowledge(conversationId, ids, retrievalToken) {
	const entry = {
		ids: JSON.stringify(ids),
		retrievalToken,
		expiresAt: Date.now() + KNOWLEDGE_CONTEXT_TTL_MS,
	};
  knowledgeByConversation.set(conversationId, entry);
  const timer = setTimeout(() => {
    if (knowledgeByConversation.get(conversationId) === entry) {
      knowledgeByConversation.delete(conversationId);
    }
  }, KNOWLEDGE_CONTEXT_TTL_MS);
  timer.unref?.();
}

function takeKnowledge(conversationId) {
	const entry = knowledgeByConversation.get(conversationId);
	knowledgeByConversation.delete(conversationId);
	return entry && entry.expiresAt > Date.now() ? entry : null;
}

if (!API_TOKEN) {
  console.error("[rag-mcp] INTERNAL_API_TOKEN is not set – exiting.");
  process.exit(1);
}
if (!ACCOUNT_ID) {
  console.error("[rag-mcp] WHATSAPP_AI_ACCOUNT_ID is not set – exiting.");
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
        "查询当前 WhatsApp 账号绑定的业务资料，获取与客户问题相关的事实、对话历史和客服回答规则。" +
        "在回复任何 WhatsApp 消息之前，必须先调用此工具；首次查询应包含客户原始关键词和相关同义词，" +
        "若没有匹配结果，应扩大同义词后再查询一次。",
      inputSchema: {
        type: "object",
        properties: {
          query: {
            type: "string",
            description:
              "检索词：保留客户原始关键词，并补充2到4个相关同义词或业务术语，例如“退货规则 退货政策 退换货”",
          },
          customerMessage: {
            type: "string",
            description: "客户本轮发送的原始 WhatsApp 消息，必须逐字保留，不能改写或扩展",
          },
          conversationId: {
            type: "string",
            description: "WhatsApp 会话 ID（通常是用户手机号，如 +8613800000000）",
          },
          customerName: {
            type: "string",
            description: "客户名称（可选，默认为 conversationId 的值）",
          },
        },
        required: ["query", "customerMessage", "conversationId"],
      },
    },
    {
      name: "save_reply",
      description:
        "保存即将发送给客户的客服回复。形成最终答案后、向 WhatsApp 输出前调用；" +
        "工具成功后，最终输出必须与 content 完全相同，不能补充保存、检索或内部流程说明。",
      inputSchema: {
        type: "object",
        properties: {
          conversationId: {
            type: "string",
            description: "WhatsApp 会话 ID（与 search_knowledge 使用的相同）",
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
        required: ["conversationId", "content"],
      },
    },
  ],
}));

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const { name, arguments: args } = request.params;

	if (name === "save_reply") {
		const { conversationId, customerName, content } = args;
		const retrieval = takeKnowledge(conversationId);
		if (!retrieval) {
			return {
				content: [{ type: "text", text: "请先查询当前客户问题的业务资料，再形成并保存回复。" }],
				isError: true,
			};
		}
		try {
      await apiPost("/api/internal/conversations/reply", {
        conversationId,
        accountId: ACCOUNT_ID,
        customerName: customerName || conversationId,
        content,
				knowledgeIds: retrieval.ids,
				retrievalToken: retrieval.retrievalToken,
      });
      return {
        content: [{
          type: "text",
          text: "回复内容已记录。现在仅输出与 content 完全相同的客服回复，不要提及记录、检索、资料来源或任何内部流程。",
        }],
      };
    } catch {
      return {
        content: [{ type: "text", text: "客服回复记录暂未保存；不要向客户透露内部错误或技术细节。" }],
        isError: true,
      };
    }
  }

  if (name !== "search_knowledge") {
    throw new Error(`Unknown tool: ${name}`);
  }

  const { query, customerMessage, conversationId, customerName } = args;

  try {
    const data = await apiPost("/api/internal/conversations/query", {
      message: customerMessage,
      searchQuery: query,
      conversationId,
      accountId: ACCOUNT_ID,
      customerName: customerName || conversationId,
      maxHistory: 10,
      maxKnowledge: 5,
    });

		if (!data.retrievalToken) {
		}
		rememberKnowledge(
			conversationId,
			(data.knowledge || []).map((item) => item.id).filter(Boolean),
			data.retrievalToken,
		);

    // The backend already generates a persona-aware system prompt.
    // Pass it through as the primary instruction for the agent.
    const personaPrompt = data.systemPrompt || "";

    // Format knowledge results
    let knowledgeText = "";
    if (data.knowledge && data.knowledge.length > 0) {
      knowledgeText = "## 可用业务资料\n\n";
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
        const role = msg.role === "user" || msg.role === "customer" ? "用户" : "客服";
        historyText += `**${role}**: ${msg.content}\n\n`;
      }
    }

    const latestHistory = data.history?.at(-1);
    const historyContainsLatest =
      (latestHistory?.role === "user" || latestHistory?.role === "customer") &&
      latestHistory?.content === customerMessage;

    const result = [
      personaPrompt,
      knowledgeText,
      historyText,
      historyContainsLatest ? "" : `\n用户最新消息: ${customerMessage}`,
      `\n[重要] 回复后必须调用 save_reply 保存。`,
    ].join("\n");

    return { content: [{ type: "text", text: result }] };
  } catch (err) {
    knowledgeByConversation.delete(conversationId);
    return {
      content: [
        {
          type: "text",
          text: "当前无法取得足够事实依据。最终回复只能自然说明这个问题需要进一步核实，不得编造事实，也不得提及资料、检索、错误、系统、平台、工具或其他内部细节。",
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
