import { db } from "../db.js";

export const messageHandlers = {
  list({ accountKey = "", query = "", page = 1, pageSize = 20 } = {}) {
    const conditions = [];
    const params = [];
    if (accountKey) { conditions.push("account_key = ?"); params.push(accountKey); }
    if (query) { conditions.push("text LIKE ?"); params.push(`%${query}%`); }
    const where = conditions.length ? `WHERE ${conditions.join(" AND ")}` : "";

    const total = db.prepare(`SELECT COUNT(*) as c FROM messages ${where}`).get(...params)?.c || 0;
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    const safePage = Math.min(Math.max(1, page), totalPages);
    const rows = db.prepare(
      `SELECT * FROM messages ${where} ORDER BY at DESC LIMIT ? OFFSET ?`
    ).all(...params, pageSize, (safePage - 1) * pageSize);

    // Group by customer (same customer -> one conversation)
    const conversations = [];
    let current = null;
    for (const msg of rows.reverse()) {
      if (!current || current.customer !== msg.customer) {
        if (current) conversations.push(current);
        current = { customer: msg.customer, accountKey: msg.account_key, messages: [] };
      }
      current.messages.push(msg);
    }
    if (current) conversations.push(current);

    return { ok: true, page: safePage, pageSize, total, totalPages, conversations };
  },

  exportCsv({ accountKey = "", query = "" } = {}) {
    const conditions = [];
    const params = [];
    if (accountKey) { conditions.push("account_key = ?"); params.push(accountKey); }
    if (query) { conditions.push("text LIKE ?"); params.push(`%${query}%`); }
    const where = conditions.length ? `WHERE ${conditions.join(" AND ")}` : "";
    const rows = db.prepare(`SELECT * FROM messages ${where} ORDER BY at DESC LIMIT 10000`).all(...params);
    const header = "客户,账号,方向,内容,时间";
    const csvRows = [header];
    for (const r of rows) {
      csvRows.push(`"${r.customer}","${r.account_key}","${r.direction}","${(r.text||"").replace(/"/g,'""')}","${r.at}"`);
    }
    return "﻿" + csvRows.join("\n");
  }
};
