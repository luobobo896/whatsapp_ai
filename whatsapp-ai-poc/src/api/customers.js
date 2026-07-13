import { db } from "../db.js";

export const customerHandlers = {
  list({ accountKey = "", page = 1, pageSize = 20 } = {}) {
    let where = "";
    const params = [];
    if (accountKey) { where = "WHERE account_key = ?"; params.push(accountKey); }

    const customers = db.prepare(`
      SELECT customer, account_key,
        COUNT(*) as conversation_count,
        MIN(at) as first_contact,
        MAX(at) as last_contact
      FROM messages
      ${where}
      GROUP BY customer, account_key
      ORDER BY last_contact DESC
    `).all(...params);

    // Enrich with last message preview
    const enriched = customers.map(c => {
      const lastMsg = db.prepare("SELECT text FROM messages WHERE customer = ? AND account_key = ? ORDER BY at DESC LIMIT 1").get(c.customer, c.account_key);
      return { ...c, lastMessagePreview: (lastMsg?.text || "").slice(0, 50) };
    });

    const total = enriched.length;
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    const safePage = Math.min(Math.max(1, page), totalPages);
    const start = (safePage - 1) * pageSize;

    return { ok: true, customers: enriched.slice(start, start + pageSize), page: safePage, total, totalPages };
  }
};
