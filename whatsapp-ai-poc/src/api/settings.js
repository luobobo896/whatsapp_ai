import { db } from "../db.js";

export const settingsHandlers = {
  get() {
    const s = db.prepare("SELECT * FROM settings WHERE id = 1").get();
    if (!s) return { ok: true, settings: { workingHours: { start: "09:00", end: "18:00", timezone: "Asia/Shanghai" }, autoReply: { outOfHours: "", greeting: "" }, globalDefaults: { dailyLimit: 30 } } };
    return {
      ok: true,
      settings: {
        workingHours: { start: s.work_start, end: s.work_end, timezone: s.timezone },
        autoReply: { outOfHours: s.out_of_hours_reply, greeting: s.greeting },
        globalDefaults: { dailyLimit: s.default_daily_limit }
      }
    };
  },

  save(body) {
    const s = body.settings || body;
    db.prepare(`UPDATE settings SET
      work_start=?, work_end=?, timezone=?,
      out_of_hours_reply=?, greeting=?, default_daily_limit=?,
      updated_at=datetime('now') WHERE id=1`).run(
      s.workingHours?.start ?? "09:00", s.workingHours?.end ?? "18:00", s.timezone ?? "Asia/Shanghai",
      s.autoReply?.outOfHours ?? "", s.autoReply?.greeting ?? "",
      s.globalDefaults?.dailyLimit ?? 30
    );
    return { ok: true };
  }
};
