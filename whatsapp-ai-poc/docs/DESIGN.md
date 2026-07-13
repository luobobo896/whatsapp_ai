---
version: alpha
name: whatsapp-ai-ops-admin
description: A WhatsApp-green-inspired admin dashboard for the WhatsApp AI customer service platform. Dark green-black sidebar anchors a light gray-blue main content area. The system uses WhatsApp's signature teal-green (#128C7E / #075E54 / #25D366) as brand voltage — applied sparingly to CTAs, status indicators, and active states — against a neutral professional admin palette. Chinese-first type stack with "Inter" / "Microsoft YaHei" sans-serif throughout. Designed for dense data display (tables, cards, chat bubbles, forms) with clear information hierarchy.

colors:
  primary: "#128C7E"
  primary-active: "#075E54"
  primary-light: "#25D366"
  primary-soft: "#e1f5f0"
  primary-ghost: "#f0faf7"
  ink: "#1a1f1c"
  body: "#3d403d"
  muted: "#6b736d"
  muted-soft: "#949e96"
  hairline: "#e2e6e3"
  hairline-soft: "#eef0ee"
  canvas: "#f5f7fa"
  surface: "#ffffff"
  surface-soft: "#f9fafb"
  surface-raised: "#ffffff"
  sidebar: "#0d1f17"
  sidebar-elevated: "#152a20"
  sidebar-hairline: "rgba(255,255,255,0.08)"
  sidebar-text: "#b8c9bf"
  sidebar-text-muted: "#7a9685"
  sidebar-text-active: "#ffffff"
  sidebar-accent: "#25D366"
  on-primary: "#ffffff"
  on-sidebar: "#e8f0ea"
  success: "#1fa855"
  success-soft: "#e6f7ed"
  warning: "#e8a317"
  warning-soft: "#fef7e6"
  error: "#d94535"
  error-soft: "#fdecea"
  info: "#2d6a8e"
  info-soft: "#e4eff6"
  chat-inbound: "#f0f2f5"
  chat-outbound: "#d9fdd3"
  chat-outbound-border: "#b5e8c3"

typography:
  display-lg:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 28px
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: -0.3px
  display-md:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 22px
    fontWeight: 700
    lineHeight: 1.25
    letterSpacing: -0.2px
  title-lg:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 18px
    fontWeight: 600
    lineHeight: 1.35
    letterSpacing: 0
  title-md:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.4
    letterSpacing: 0
  title-sm:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 14px
    fontWeight: 600
    lineHeight: 1.4
    letterSpacing: 0
  body-md:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: 0
  body-sm:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: 0
  caption:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 12px
    fontWeight: 500
    lineHeight: 1.4
    letterSpacing: 0
  caption-uppercase:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 11px
    fontWeight: 700
    lineHeight: 1.3
    letterSpacing: 0.06em
  code:
    fontFamily: '"JetBrains Mono", "Consolas", "Microsoft YaHei", monospace'
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.6
    letterSpacing: 0
  button:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 14px
    fontWeight: 600
    lineHeight: 1
    letterSpacing: 0
  nav-link:
    fontFamily: '"Inter", system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Microsoft YaHei", sans-serif'
    fontSize: 14px
    fontWeight: 500
    lineHeight: 1.3
    letterSpacing: 0

rounded:
  xs: 4px
  sm: 6px
  md: 8px
  lg: 12px
  xl: 16px
  pill: 9999px

spacing:
  xxs: 4px
  xs: 8px
  sm: 12px
  md: 16px
  lg: 24px
  xl: 32px
  xxl: 48px

components:
  sidebar:
    backgroundColor: "{colors.sidebar}"
    textColor: "{colors.sidebar-text}"
    width: 260px
    position: sticky
    height: 100vh
  nav-button:
    backgroundColor: transparent
    textColor: "{colors.sidebar-text}"
    typography: "{typography.nav-link}"
    rounded: "{rounded.md}"
    padding: 10px 12px
    height: 44px
  nav-button-active:
    backgroundColor: "{colors.sidebar-elevated}"
    textColor: "{colors.sidebar-text-active}"
    borderColor: "rgba(37,211,102,0.20)"
  nav-section-label:
    textColor: "{colors.sidebar-text-muted}"
    typography: "{typography.caption-uppercase}"
    padding: 16px 8px 4px
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: 8px 16px
    height: 36px
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: 8px 16px
    height: 36px
    borderColor: "{colors.hairline}"
  button-danger:
    backgroundColor: "{colors.error}"
    textColor: "{colors.on-primary}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: 8px 16px
    height: 36px
  button-ghost:
    backgroundColor: transparent
    textColor: "{colors.muted}"
    typography: "{typography.button}"
    rounded: "{rounded.md}"
    padding: 8px 12px
    height: 36px
  card:
    backgroundColor: "{colors.surface}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.lg}"
    padding: "{spacing.md}"
  panel:
    backgroundColor: "{colors.surface}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.lg}"
    shadow: "0 1px 3px rgba(0,0,0,0.04)"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    typography: "{typography.body-md}"
    rounded: "{rounded.md}"
    padding: 8px 12px
    height: 36px
    borderColor: "{colors.hairline}"
  textarea:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    typography: "{typography.code}"
    rounded: "{rounded.md}"
    padding: 12px
    borderColor: "{colors.hairline}"
  select:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    typography: "{typography.body-md}"
    rounded: "{rounded.md}"
    padding: 8px 12px
    height: 36px
    borderColor: "{colors.hairline}"
  badge-status:
    typography: "{typography.caption}"
    rounded: "{rounded.pill}"
    padding: 2px 10px
    height: 24px
  badge-status-online:
    backgroundColor: "{colors.success-soft}"
    textColor: "{colors.success}"
  badge-status-offline:
    backgroundColor: "{colors.surface-soft}"
    textColor: "{colors.muted}"
  badge-status-error:
    backgroundColor: "{colors.error-soft}"
    textColor: "{colors.error}"
  badge-status-disabled:
    backgroundColor: "{colors.warning-soft}"
    textColor: "{colors.warning}"
  tab:
    backgroundColor: transparent
    textColor: "{colors.muted}"
    typography: "{typography.title-sm}"
    padding: 8px 16px
    rounded: "{rounded.md}"
    borderBottom: "2px solid transparent"
  tab-active:
    backgroundColor: transparent
    textColor: "{colors.primary}"
    borderBottom: "2px solid {colors.primary}"
  modal-overlay:
    backgroundColor: "rgba(0,0,0,0.45)"
  modal-box:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.xl}"
    padding: "{spacing.lg}"
  toast:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.on-primary}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: 12px 20px
  chat-bubble-inbound:
    backgroundColor: "{colors.chat-inbound}"
    textColor: "{colors.ink}"
    rounded: "12px 12px 12px 4px"
    padding: 10px 14px
    maxWidth: 70%
  chat-bubble-outbound:
    backgroundColor: "{colors.chat-outbound}"
    textColor: "{colors.ink}"
    rounded: "12px 12px 4px 12px"
    padding: 10px 14px
    maxWidth: 70%
  progress-bar:
    backgroundColor: "{colors.hairline-soft}"
    rounded: "{rounded.pill}"
    height: 8px
  progress-bar-fill:
    backgroundColor: "{colors.primary}"
    rounded: "{rounded.pill}"
  metric-card:
    backgroundColor: "{colors.surface}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.lg}"
    padding: "{spacing.md}"
---

## Overview

The WhatsApp AI Ops admin dashboard is a **professional, Chinese-first admin panel** for managing WhatsApp customer service AI agents. The design anchors on a **dark green-black sidebar** (`{colors.sidebar}` — #0d1f17) paired with a **light gray-blue main canvas** (`{colors.canvas}` — #f5f7fa). WhatsApp's signature teal-green (`{colors.primary}` — #128C7E) provides brand voltage — applied sparingly to primary CTAs, active navigation states, status indicators, and progress bars.

The system serves dense operational data: account status cards, knowledge base editors, chat conversation histories, real-time message feeds, audit logs, and configuration forms. Information hierarchy is paramount — the sidebar provides persistent navigation while the main area uses panel-based layouts with clear visual separation.

**Key Characteristics:**
- Dark green-black sidebar (`{colors.sidebar}`) with WhatsApp-green accent highlights for active nav items
- Light canvas (`{colors.canvas}`) main area with white surface cards (`{colors.surface}`)
- Chinese-first typography stack: Inter > system-ui > Microsoft YaHei
- Status badges in four semantic colors: online (green), pending (gray), error (red), disabled (amber)
- Chat bubbles use WhatsApp-style asymmetric rounding: inbound left-aligned gray, outbound right-aligned green-tint
- All interactive elements use 8px border radius; cards use 12px
- Responsive: sidebar collapses to top bar below 1080px, single-column below 720px

## Colors

### Brand & Accent
- **Primary / Teal Green** (`{colors.primary}` — #128C7E): WhatsApp signature teal. Used on primary buttons, active nav, progress bars, and selected tabs.
- **Primary Active** (`{colors.primary-active}` — #075E54): Darker teal for button press states.
- **Primary Light** (`{colors.primary-light}` — #25D366): WhatsApp bright green accent. Used for online status dots and sidebar accent glows.
- **Primary Soft** (`{colors.primary-soft}` — #e1f5f0): Light teal tint for hover backgrounds on white surfaces.
- **Primary Ghost** (`{colors.primary-ghost}` — #f0faf7): Extremely subtle green tint.

### Surface
- **Sidebar** (`{colors.sidebar}` — #0d1f17): Deep green-black sidebar background.
- **Sidebar Elevated** (`{colors.sidebar-elevated}` — #152a20): Active/hover nav item background in sidebar.
- **Canvas** (`{colors.canvas}` — #f5f7fa): Main content area background — light gray-blue.
- **Surface** (`{colors.surface}` — #ffffff): Card and panel backgrounds.
- **Surface Soft** (`{colors.surface-soft}` — #f9fafb): Subtle alternating row/band backgrounds.

### Text
- **Ink** (`{colors.ink}` — #1a1f1c): Primary text on light surfaces.
- **Body** (`{colors.body}` — #3d403d): Body/paragraph text.
- **Muted** (`{colors.muted}` — #6b736d): Secondary labels, descriptions.
- **Muted Soft** (`{colors.muted-soft}` — #949e96): Placeholder text, fine print.
- **Sidebar Text** (`{colors.sidebar-text}` — #b8c9bf): Default nav text in sidebar.
- **Sidebar Text Muted** (`{colors.sidebar-text-muted}` — #7a9685): Section labels in sidebar.
- **Sidebar Text Active** (`{colors.sidebar-text-active}` — #ffffff): Active nav item text.

### Semantic
- **Success** (`{colors.success}` — #1fa855): Online/healthy indicators.
- **Warning** (`{colors.warning}` — #e8a317): Disabled/degraded states.
- **Error** (`{colors.error}` — #d94535): Error/failure states.
- **Info** (`{colors.info}` — #2d6a8e): Informational highlights.

### Chat
- **Chat Inbound** (`{colors.chat-inbound}` — #f0f2f5): Customer message bubble background (WhatsApp-style gray).
- **Chat Outbound** (`{colors.chat-outbound}` — #d9fdd3): Agent reply bubble background (WhatsApp-style green tint).

## Typography

The system uses a single sans-serif family stack throughout: **Inter** as primary, falling back through system-ui to **Microsoft YaHei** for Chinese character rendering. This ensures consistent rendering across Mac, Windows, and Linux. Code and JSON editors use JetBrains Mono with Microsoft YaHei fallback for mixed CN/EN content.

### Hierarchy

| Token | Size | Weight | Line | Use |
|---|---|---|---|---|
| `display-lg` | 28px | 700 | 1.2 | Page titles |
| `display-md` | 22px | 700 | 1.25 | Panel titles |
| `title-lg` | 18px | 600 | 1.35 | Section headers |
| `title-md` | 16px | 600 | 1.4 | Card titles |
| `title-sm` | 14px | 600 | 1.4 | Field labels |
| `body-md` | 14px | 400 | 1.55 | Default body |
| `body-sm` | 13px | 400 | 1.5 | Secondary text |
| `caption` | 12px | 500 | 1.4 | Badges, small labels |
| `caption-uppercase` | 11px | 700 | 1.3 | Sidebar section labels |
| `code` | 13px | 400 | 1.6 | Code/JSON editor |
| `button` | 14px | 600 | 1.0 | Button labels |
| `nav-link` | 14px | 500 | 1.3 | Sidebar navigation |

### Principles
- All text uses the same font stack; hierarchy comes from size, weight, and color.
- Chinese text renders well at all sizes with Microsoft YaHei in the stack.
- Display sizes use weight 700 for clear page-level hierarchy.
- Body text stays at weight 400 for readability in dense data views.

## Layout

### Grid System
- **Sidebar + Main**: CSS Grid `grid-template-columns: 260px minmax(0, 1fr)` at desktop.
- **Content grids**: `grid-template-columns: repeat(auto-fill, minmax(...))` for cards.
- **Two-column panels**: `grid-template-columns: minmax(0, 1fr) minmax(0, 1fr)`.
- **Metric cards**: `grid-template-columns: repeat(4, minmax(0, 1fr))`.

### Sidebar
- `position: sticky; top: 0; height: 100vh; overflow-y: auto;`
- Flex column layout with brand header, nav items (grouped with dividers), and footer note.
- Nav items: 44px height, full-width, icon + label + optional badge.

### Main Area
- `padding: 24px` at desktop, `14px` at mobile.
- Top bar: page title + description + action buttons + sync status.

## Do's and Don'ts

### Do
- Use the dark sidebar for navigation; never move nav to top bar on desktop.
- Use semantic status colors consistently: green=online/ok, gray=pending, red=error, amber=disabled/warning.
- Keep chat bubbles asymmetric: inbound left-rounded, outbound right-rounded — matching WhatsApp convention.
- Use the panel component for all content sections — consistent border + radius + shadow.
- Show loading states ("加载中...") before data arrives.
- Use the toast component for transient success/error messages.

### Don't
- Don't use pure black for text — always {colors.ink} (#1a1f1c).
- Don't use WhatsApp green excessively — it's an accent, not a fill color.
- Don't place action buttons in the sidebar — keep it navigation-only.
- Don't remove the sidebar footer note — it provides important operational context.
- Don't use the old /admin-api/* routes — all API calls use /api/*.

## Responsive Behavior

| Name | Width | Key Changes |
|---|---|---|
| Mobile | < 720px | Single column; sidebar becomes horizontal top bar; metric cards 1-up; form grids single column |
| Tablet | 720-1080px | Sidebar horizontal; two-column grids; form grids 2-col |
| Desktop | > 1080px | Full sticky sidebar; 4-up metrics; multi-column panels |

## Known Gaps
- Animations and transitions are minimal (fadeIn only).
- Dark mode is not supported — the sidebar is always dark, main area always light.
- The QR modal polling could be replaced with WebSocket/SSE for real-time updates.
- No drag-and-drop or advanced sorting in knowledge base entry management.
