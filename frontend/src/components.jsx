import { MessageCircleMore, X } from "lucide-react";

export function Button({ children, icon: Icon, variant = "primary", className = "", ...props }) {
  return (
    <button className={`button button-${variant} ${className}`} {...props}>
      {Icon ? <Icon aria-hidden="true" size={16} strokeWidth={2} /> : null}
      {children}
    </button>
  );
}

export function IconButton({ label, icon: Icon, ...props }) {
  return (
    <button className="icon-button" aria-label={label} title={label} {...props}>
      <Icon aria-hidden="true" size={18} strokeWidth={2} />
    </button>
  );
}

export function Brand({ compact = false }) {
  return (
    <div className={`brand ${compact ? "brand-compact" : ""}`}>
      <span className="brand-mark"><MessageCircleMore aria-hidden="true" size={22} strokeWidth={2.2} /></span>
      <span className="brand-copy">
        <strong>WhatsApp AI Ops</strong>
        {!compact ? <small>智能客服运营台</small> : null}
      </span>
    </div>
  );
}

export function Badge({ children, tone = "muted" }) {
  return <span className={`badge badge-${tone}`}>{children}</span>;
}

export function Panel({ title, description, action, children, className = "" }) {
  return (
    <section className={`panel ${className}`}>
      {title ? (
        <header className="panel-header">
          <div>
            <h2>{title}</h2>
            {description ? <p>{description}</p> : null}
          </div>
          {action}
        </header>
      ) : null}
      <div className="panel-body">{children}</div>
    </section>
  );
}

export function Field({ label, hint, children }) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint ? <small>{hint}</small> : null}
    </label>
  );
}

export function Dialog({ title, description, onClose, children, footer }) {
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onClose}>
      <section className="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title" onMouseDown={(event) => event.stopPropagation()}>
        <header className="dialog-header">
          <div>
            <h2 id="dialog-title">{title}</h2>
            {description ? <p>{description}</p> : null}
          </div>
          <IconButton label="关闭" icon={X} onClick={onClose} />
        </header>
        <div className="dialog-body">{children}</div>
        {footer ? <footer className="dialog-footer">{footer}</footer> : null}
      </section>
    </div>
  );
}

export function EmptyState({ icon: Icon, title, description, action }) {
  return (
    <div className="empty-state">
      <span className="empty-icon"><Icon aria-hidden="true" size={22} /></span>
      <h3>{title}</h3>
      <p>{description}</p>
      {action}
    </div>
  );
}

export function Toast({ toast }) {
  if (!toast) return null;
  return <div className={`toast toast-${toast.tone || "info"}`} role="status">{toast.message}</div>;
}
