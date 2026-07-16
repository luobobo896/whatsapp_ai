import assert from "node:assert/strict";
import http from "node:http";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";

test("MCP returns one current customer message and saves knowledge references", async (t) => {
  const requests = [];
  const api = http.createServer((req, res) => {
    let body = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => { body += chunk; });
    req.on("end", () => {
      requests.push({ path: req.url, body: JSON.parse(body) });
      res.setHeader("Content-Type", "application/json");
      if (req.url.endsWith("/query")) {
        res.end(JSON.stringify({
          systemPrompt: "只使用知识库",
          knowledge: [{ id: "article-1", title: "退货", knowledgeBaseName: "售后", content: "七天内可退货" }],
          history: [
            { role: "customer", content: "之前的问题" },
            { role: "assistant", content: "之前的答复" },
            { role: "customer", content: "怎么退货" },
          ],
        }));
        return;
      }
      res.end(JSON.stringify({ id: "reply-1" }));
    });
  });
  await new Promise((resolve) => api.listen(0, "127.0.0.1", resolve));
  t.after(() => api.close());

  const transport = new StdioClientTransport({
    command: process.execPath,
    args: [fileURLToPath(new URL("./index.mjs", import.meta.url))],
    env: {
      WHATSAPP_AI_API_URL: `http://127.0.0.1:${api.address().port}`,
      INTERNAL_API_TOKEN: "test-token",
      WHATSAPP_AI_ACCOUNT_ID: "account-1",
    },
  });
  const client = new Client({ name: "rag-test", version: "1.0.0" });
  await client.connect(transport);
  t.after(() => client.close());

  const search = await client.callTool({
    name: "search_knowledge",
    arguments: { query: "怎么退货", conversationId: "+8613800000000" },
  });
  const prompt = search.content[0].text;
  assert.match(prompt, /\*\*用户\*\*: 之前的问题/);
  assert.equal(prompt.match(/怎么退货/g)?.length, 1);

  await client.callTool({
    name: "save_reply",
    arguments: { conversationId: "+8613800000000", content: "七天内可以退货" },
  });
  assert.equal(requests[1].path, "/api/internal/conversations/reply");
  assert.equal(requests[1].body.knowledgeIds, '["article-1"]');
});
