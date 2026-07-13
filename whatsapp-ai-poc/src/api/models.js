import * as openclaw from "../openclaw-bridge.js";

export const modelHandlers = {
  get() {
    const config = openclaw.readOpenClawConfig();
    return {
      ok: true,
      providers: config.models?.providers || {},
      primary: config.agents?.defaults?.model?.primary || "",
      fallback: config.agents?.defaults?.model?.fallback || ""
    };
  },

  save(body) {
    const config = openclaw.readOpenClawConfig();
    config.models ||= { mode: "merge", providers: {} };
    if (body.providers) config.models.providers = body.providers;
    if (body.fallback !== undefined) {
      config.agents ||= { defaults: {} };
      config.agents.defaults.model ||= {};
      config.agents.defaults.model.fallback = body.fallback;
    }
    if (body.primary !== undefined) {
      config.agents ||= { defaults: {} };
      config.agents.defaults.model ||= {};
      config.agents.defaults.model.primary = body.primary;
    }
    openclaw.writeOpenClawConfig(config);
    openclaw.scheduleGatewayRestart();
    return { ok: true };
  }
};
