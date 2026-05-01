// Parse a single-line task input that may contain inline metadata.
// Mirrors the markers granit's TaskStore parses, so what the user types here
// is byte-for-byte the line that ends up in the daily note.
//
//   buy milk !2 due:2026-05-15 #errand
//   →  text="buy milk", priority=2, dueDate="2026-05-15", tags=["errand"]
//
// Returns the cleaned text and the extracted markers — the caller serializes
// them back to canonical form via buildTaskTextLine on the server.

export interface ParsedTask {
  text: string;
  priority: number; // 0 if absent
  dueDate: string; // '' if absent
  tags: string[];
}

const reTag = /(^|\s)#([\p{L}\p{N}_/-]+)/gu;
const rePriority = /(^|\s)!([1-3])(\s|$)/;
const reDue = /(^|\s)due:(\d{4}-\d{2}-\d{2})(\s|$)/;

export function parseTaskInput(raw: string): ParsedTask {
  let text = raw;
  const tags: string[] = [];
  let priority = 0;
  let dueDate = '';

  for (const m of text.matchAll(reTag)) {
    const t = m[2];
    if (!tags.includes(t)) tags.push(t);
  }
  text = text.replace(reTag, '$1');

  const pm = text.match(rePriority);
  if (pm) {
    priority = Number(pm[2]);
    text = text.replace(rePriority, '$1$3');
  }

  const dm = text.match(reDue);
  if (dm) {
    dueDate = dm[2];
    text = text.replace(reDue, '$1$3');
  }

  text = text.trim().replace(/\s+/g, ' ');
  return { text, priority, dueDate, tags };
}

// "today" / "tomorrow" / "fri" / "next mon" → YYYY-MM-DD
export function smartDate(token: string, ref = new Date()): string | null {
  const t = token.toLowerCase().trim();
  const day = (n: number) => {
    const d = new Date(ref);
    d.setDate(d.getDate() + n);
    return iso(d);
  };
  if (t === 'today') return iso(ref);
  if (t === 'tomorrow' || t === 'tmrw') return day(1);
  if (t === 'yesterday') return day(-1);
  // weekday tokens
  const wd = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];
  const next = t.startsWith('next ');
  const word = next ? t.slice(5) : t;
  const idx = wd.findIndex((w) => word.startsWith(w));
  if (idx >= 0) {
    const cur = ref.getDay();
    let delta = (idx - cur + 7) % 7;
    if (delta === 0 || next) delta += 7;
    return day(delta);
  }
  return null;
}

function iso(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
