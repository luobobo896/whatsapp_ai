---
name: WhatsApp AI Ops
scope: authenticated operations workspace
colors:
  canvas: "#f6f8f7"
  surface: "#ffffff"
  surface-soft: "#eef5f1"
  ink: "#18211d"
  text: "#425048"
  muted: "#6b786f"
  border: "#dce5df"
  primary: "#087f5b"
  primary-strong: "#056947"
  primary-soft: "#ddf5e9"
  primary-border: "#ccead9"
  sidebar: "#10251b"
  sidebar-text: "#c4d1c8"
  danger: "#c53c32"
  warning: "#ad6b12"
typography:
  family: "Inter, system-ui, -apple-system, BlinkMacSystemFont, Segoe UI, Microsoft YaHei, sans-serif"
  page-title: { size: "20px", weight: 700, lineHeight: 1.3 }
  section-title: { size: "16px", weight: 700, lineHeight: 1.4 }
  body: { size: "14px", weight: 400, lineHeight: 1.55 }
  meta: { size: "12px", weight: 500, lineHeight: 1.45 }
spacing:
  xxs: "4px"
  xs: "8px"
  sm: "12px"
  md: "16px"
  lg: "24px"
  xl: "32px"
rounded:
  control: "6px"
  card: "8px"
  dialog: "10px"
components:
  chat-workspace:
    columns: "280px / minmax(420px, 1fr) / 280px"
    regions: "conversation rail / message canvas / context inspector"
    alignment: "all three regions share one height and independent scrolling"
  knowledge-binding:
    input: "virtualized, searchable multiselect with collapsed tags"
    selected-summary: "count is always visible; never render every selected item inline"
  account-table:
    knowledge-cell: "show count and at most two names; preview is capped"
---

# Operations Workspace Design

This interface is an operational tool used repeatedly by customer-service managers. It should feel calm, compact, and dependable rather than promotional. WhatsApp green is an identity anchor and an action color, not a background effect. The dark sidebar establishes product identity while work surfaces stay light, neutral, and easy to scan.

## Layout And Density

Keep the main content on a consistent grid with a 24px desktop gutter and 14px mobile gutter. Section headers carry the title, a one-line operational description, and one clear primary action. Tables are for comparison; long relationship data must be represented as a summary rather than raw repeated chips. Account rows should remain close to a single line even when an account has hundreds or thousands of knowledge bases.

The chat workspace is a fixed three-region working surface on desktop: a 280px conversation rail, a flexible message canvas, and a 280px context inspector. All three regions share the same vertical boundary and own their scrolling. The center canvas gets the most width and visual weight; the inspector never competes with the transcript, and the conversation rail never expands because of a long customer name or preview.

## Knowledge Binding

Knowledge-base selection is a high-cardinality relationship. Use a virtualized control with local filtering so a large option list stays responsive. The field always communicates selection count. In collapsed state, retain no more than one visible label and a numeric overflow indicator. Support one-click clear and select-all actions, but keep both beside the summary rather than inside the list of selected values. A table cell previews no more than two names and reports the remainder as a number. Do not make users scan a wall of tags.

The inspector repeats only high-value context: account connection and reply capacity, knowledge-base scope, the selected customer's identity and message count, and the delivery policy. It is deliberately read-only during chat so the operator's attention stays on the message composer.

## Components

Controls have a 6px radius and strong focus outlines. Cards and dialogs use an 8-10px radius with a restrained border and shallow shadow. Use the existing Lucide icon set for functional icons. Motion is limited to 180-220ms color and opacity transitions, and must respect reduced-motion preferences. Normal text maintains at least 4.5:1 contrast; muted metadata remains legible on white surfaces.

## Responsive Behavior

At narrow widths, preserve the account name, status, knowledge summary, and actions as the priority columns. Less essential metadata can be hidden by the table's horizontal scroll container, never squeezed into unreadable fragments. The binding field remains full width; its virtualized popper is capped in height so it does not become an unbounded page within a dialog.

## Do And Don't

Do use the semantic variables in `main.css` for new UI. Do present counts before long lists. Do provide clear selected and empty states. Do not introduce gradients, decorative icon clusters, or nested cards. Do not render every selected knowledge base as a tag in an account row or multiselect input.
