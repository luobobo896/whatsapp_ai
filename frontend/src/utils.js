export function formatDate(value) {
  if (!value) return "-";
  // API returns YYYY-MM-DD HH:MI:SS format — use directly if valid
  if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(value)) return value;
  // Fallback: parse and format to YYYY-MM-DD HH:mm:ss
  const d = new Date(value);
  if (isNaN(d.getTime())) return value;
  const pad = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}
