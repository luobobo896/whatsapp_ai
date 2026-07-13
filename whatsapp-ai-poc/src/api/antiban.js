import { db } from "../db.js";

export const antibanHandlers = {
  get() {
    const cfg = db.prepare("SELECT * FROM antiban_config WHERE id = 1").get();
    if (!cfg) return { ok: true, config: { replyDelay: { min: 1000, max: 5000 }, rateLimit: { maxPerHour: 30, maxPerDay: 200 }, warmup: { enabled: true, durationHours: 24 }, sessionKeepalive: { enabled: true, intervalMinutes: 5 } } };
    return {
      ok: true,
      config: {
        replyDelay: { min: cfg.reply_delay_min, max: cfg.reply_delay_max },
        rateLimit: { maxPerHour: cfg.rate_limit_hour, maxPerDay: cfg.rate_limit_day },
        warmup: { enabled: !!cfg.warmup_enabled, durationHours: cfg.warmup_hours },
        sessionKeepalive: { enabled: !!cfg.keepalive_enabled, intervalMinutes: cfg.keepalive_interval }
      }
    };
  },

  save(body) {
    const c = body.config || body;
    db.prepare(`UPDATE antiban_config SET
      reply_delay_min=?, reply_delay_max=?, rate_limit_hour=?, rate_limit_day=?,
      warmup_enabled=?, warmup_hours=?, keepalive_enabled=?, keepalive_interval=?,
      updated_at=datetime('now') WHERE id=1`).run(
      c.replyDelay?.min ?? 1000, c.replyDelay?.max ?? 5000,
      c.rateLimit?.maxPerHour ?? 30, c.rateLimit?.maxPerDay ?? 200,
      c.warmup?.enabled ? 1 : 0, c.warmup?.durationHours ?? 24,
      c.sessionKeepalive?.enabled ? 1 : 0, c.sessionKeepalive?.intervalMinutes ?? 5
    );
    return { ok: true };
  }
};
