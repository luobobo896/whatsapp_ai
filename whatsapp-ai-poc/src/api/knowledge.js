import { db } from "../db.js";
import { generateAgentPrompt } from "../build-agent-prompt.js";

function genId(prefix = "e") {
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

export const knowledgeHandlers = {
  // ── 角色 CRUD ──
  listRoles() {
    const roles = db.prepare("SELECT * FROM knowledge_roles ORDER BY created_at DESC").all();
    return roles.map(r => ({
      ...r,
      keywords: JSON.parse(r.keywords || "[]"),
      bases: db.prepare(
        "SELECT kb.* FROM knowledge_bases kb JOIN knowledge_base_roles kbr ON kb.id = kbr.base_id WHERE kbr.role_id = ?"
      ).all(r.id)
    }));
  },

  createRole(body) {
    const id = String(body.id || "").trim() || genId("role");
    const name = String(body.name || "").trim();
    if (!name) throw new Error("name required");
    const tx = db.transaction(() => {
      db.prepare(
        "INSERT INTO knowledge_roles (id, name, description, unknown_reply, out_of_hours_reply, keywords) VALUES (?, ?, ?, ?, ?, ?)"
      ).run(id, name, body.description || "", body.unknownReply || "", body.outOfHoursReply || "", JSON.stringify(body.keywords || []));
      if (Array.isArray(body.baseIds)) {
        for (const bid of body.baseIds.map(String).filter(Boolean)) {
          db.prepare("INSERT OR IGNORE INTO knowledge_base_roles (base_id, role_id) VALUES (?, ?)").run(bid, id);
        }
      }
    });
    tx();
    generateAgentPrompt();
    return { ok: true, id };
  },

  updateRole(id, body) {
    const role = db.prepare("SELECT * FROM knowledge_roles WHERE id = ?").get(id);
    if (!role) throw new Error(`unknown role: ${id}`);
    const tx = db.transaction(() => {
      db.prepare(
        "UPDATE knowledge_roles SET name=?, description=?, unknown_reply=?, out_of_hours_reply=?, keywords=?, updated_at=datetime('now') WHERE id=?"
      ).run(
        body.name || role.name, body.description ?? role.description,
        body.unknownReply ?? role.unknown_reply, body.outOfHoursReply ?? role.out_of_hours_reply,
        JSON.stringify(body.keywords || JSON.parse(role.keywords || "[]")), id
      );
      if (Array.isArray(body.baseIds)) {
        db.prepare("DELETE FROM knowledge_base_roles WHERE role_id = ?").run(id);
        for (const bid of body.baseIds.map(String).filter(Boolean)) {
          db.prepare("INSERT OR IGNORE INTO knowledge_base_roles (base_id, role_id) VALUES (?, ?)").run(bid, id);
        }
      }
    });
    tx();
    generateAgentPrompt();
    return { ok: true };
  },

  deleteRole(id) {
    db.prepare("DELETE FROM knowledge_roles WHERE id = ?").run(id);
    generateAgentPrompt();
    return { ok: true };
  },

  // ── 知识库 CRUD ──
  listBases() {
    const bases = db.prepare("SELECT * FROM knowledge_bases ORDER BY created_at DESC").all();
    return bases.map(b => ({
      ...b,
      roles: db.prepare("SELECT role_id FROM knowledge_base_roles WHERE base_id = ?").all(b.id).map(r => r.role_id),
      entryCount: db.prepare("SELECT COUNT(*) as c FROM knowledge_entries WHERE base_id = ?").get(b.id)?.c || 0
    }));
  },

  createBase(body) {
    const id = body.id || genId("base");
    const name = String(body.name || "").trim();
    const type = String(body.type || "product");
    if (!name) throw new Error("name required");
    if (!["product", "faq", "document", "conversation"].includes(type)) throw new Error("invalid type");

    const tx = db.transaction(() => {
      db.prepare("INSERT INTO knowledge_bases (id, name, type, description) VALUES (?, ?, ?, ?)").run(id, name, type, body.description || "");
      if (Array.isArray(body.roleIds)) {
        for (const rid of body.roleIds.map(String).filter(Boolean)) {
          db.prepare("INSERT OR IGNORE INTO knowledge_base_roles (base_id, role_id) VALUES (?, ?)").run(id, rid);
        }
      }
    });
    tx();
    return { ok: true, id };
  },

  updateBase(id, body) {
    const base = db.prepare("SELECT * FROM knowledge_bases WHERE id = ?").get(id);
    if (!base) throw new Error(`unknown base: ${id}`);
    const tx = db.transaction(() => {
      db.prepare("UPDATE knowledge_bases SET name=?, description=?, updated_at=datetime('now') WHERE id=?")
        .run(body.name || base.name, body.description ?? base.description, id);
      if (Array.isArray(body.roleIds)) {
        db.prepare("DELETE FROM knowledge_base_roles WHERE base_id = ?").run(id);
        for (const rid of body.roleIds.map(String).filter(Boolean)) {
          db.prepare("INSERT OR IGNORE INTO knowledge_base_roles (base_id, role_id) VALUES (?, ?)").run(id, rid);
        }
      }
    });
    tx();
    return { ok: true };
  },

  deleteBase(id) {
    db.prepare("DELETE FROM knowledge_bases WHERE id = ?").run(id);
    return { ok: true };
  },

  // ── 条目 CRUD ──
  listEntries(baseId, { page = 1, pageSize = 20, categoryId = "" } = {}) {
    let sql = "SELECT * FROM knowledge_entries WHERE base_id = ?";
    const params = [baseId];
    if (categoryId) {
      sql += " AND category_id = ?";
      params.push(categoryId);
    }
    const total = db.prepare(`SELECT COUNT(*) as c FROM (${sql})`).get(...params)?.c || 0;
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    const safePage = Math.min(Math.max(1, page), totalPages);
    const rows = db.prepare(`${sql} ORDER BY created_at DESC LIMIT ? OFFSET ?`).all(...params, pageSize, (safePage - 1) * pageSize);
    return {
      page: safePage, pageSize, total, totalPages,
      entries: rows.map(r => ({ ...r, metadata: JSON.parse(r.metadata || "{}") }))
    };
  },

  createEntry(baseId, body) {
    const title = String(body.title || "").trim();
    const content = String(body.content || "").trim();
    if (!title) throw new Error("title required");
    const id = body.id || genId("ent");
    db.prepare(
      "INSERT INTO knowledge_entries (id, base_id, title, content, category_id, metadata) VALUES (?, ?, ?, ?, ?, ?)"
    ).run(id, baseId, title, content, body.categoryId || null, JSON.stringify(body.metadata || {}));
    return { ok: true, id };
  },

  updateEntry(id, body) {
    const entry = db.prepare("SELECT * FROM knowledge_entries WHERE id = ?").get(id);
    if (!entry) throw new Error(`unknown entry: ${id}`);
    db.prepare(
      "UPDATE knowledge_entries SET title=?, content=?, category_id=?, enabled=?, metadata=?, updated_at=datetime('now') WHERE id=?"
    ).run(
      body.title || entry.title, body.content ?? entry.content,
      body.categoryId !== undefined ? body.categoryId : entry.category_id,
      body.enabled !== undefined ? (body.enabled ? 1 : 0) : entry.enabled,
      body.metadata ? JSON.stringify(body.metadata) : entry.metadata,
      id
    );
    return { ok: true };
  },

  deleteEntry(id) {
    db.prepare("DELETE FROM knowledge_entries WHERE id = ?").run(id);
    return { ok: true };
  },

  // ── CSV 导入 ──
  importCsv(baseId, csv) {
    const base = db.prepare("SELECT * FROM knowledge_bases WHERE id = ?").get(baseId);
    if (!base) throw new Error(`unknown base: ${baseId}`);

    const lines = csv.split(/\r?\n/).filter(l => l.trim() && !l.startsWith("名称"));
    const imported = [];
    const errors = [];
    const insert = db.prepare(
      "INSERT INTO knowledge_entries (id, base_id, title, content, metadata) VALUES (?, ?, ?, ?, ?)"
    );

    const tx = db.transaction(() => {
      for (const line of lines) {
        const cols = line.split(",").map(c => c.trim().replace(/^"|"$/g, ""));
        const name = cols[0];
        if (!name) { errors.push(`missing name: ${line.slice(0, 30)}`); continue; }
        const id = name.replace(/[^a-z0-9-]/gi, "-").toLowerCase().replace(/-+/g, "-").replace(/^-|-$/g, "");
        const content = [cols[5] || "", cols[6] || "", cols[7] || ""].join("\n");
        insert.run(id, baseId, name, content, JSON.stringify({
          price: cols[1] || "", sizes: (cols[2] || "").split("/").filter(Boolean),
          colors: (cols[3] || "").split("/").filter(Boolean), delivery: cols[4] || "",
          sellingPoints: (cols[6] || "").split(";").filter(Boolean),
          notes: (cols[7] || "").split(";").filter(Boolean)
        }));
        imported.push(name);
      }
    });
    tx();

    return { ok: true, imported: imported.length, errors };
  },

  // ── 分类 ──
  listCategories(baseId) {
    return db.prepare("SELECT * FROM knowledge_categories WHERE base_id = ? ORDER BY sort_order").all(baseId || undefined);
  },

  createCategory(body) {
    const name = String(body.name || "").trim();
    const baseId = String(body.baseId || "");
    if (!name || !baseId) throw new Error("name and baseId required");
    const id = genId("cat");
    db.prepare("INSERT INTO knowledge_categories (id, name, base_id, sort_order) VALUES (?, ?, ?, ?)").run(id, name, baseId, body.sortOrder || 0);
    return { ok: true, id };
  },

  deleteCategory(id) {
    db.prepare("DELETE FROM knowledge_categories WHERE id = ?").run(id);
    return { ok: true };
  }
};
