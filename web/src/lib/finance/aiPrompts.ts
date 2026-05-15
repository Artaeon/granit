// Finance AI prompt builders. Two surfaces today:
//
//   - buildSnapshotPrompt → a 3-paragraph "where you stand financially"
//     read for the overview tab. Same shape grammar as the morning
//     briefing (Context / Focus / Watch).
//   - buildSubAuditPrompt → an audit of the subscription list against
//     monthly income. Surfaces costliest, duplicates, dormant
//     candidates for cancelling.
//
// Pure functions — testable. The .svelte page imports the system
// prompts + builders, then drives api.chatStream itself.

import type { FinOverview, FinSubscription, FinIncomeStream, FinGoal } from '$lib/api';

// Format cents as a human-readable currency string. Falls back to
// EUR when the overview hasn't loaded yet. Matches the page's
// fmtMoney helper but local so tests don't depend on the .svelte file.
function fmtCents(cents: number, currency: string): string {
  const major = (cents / 100).toLocaleString(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: 2
  });
  return `${major} ${currency || 'EUR'}`;
}

// Normalise cadence strings to a monthly multiplier so the audit
// prompt sees comparable amounts. Anything we don't recognise falls
// back to monthly (so a hand-edited cadence doesn't blow up the
// audit — model gets to decide).
function monthlyAmountCents(s: FinSubscription): number {
  const cadence = (s.cadence ?? '').toLowerCase();
  if (cadence === 'yearly' || cadence === 'annual') return Math.round(s.amount_cents / 12);
  if (cadence === 'quarterly') return Math.round(s.amount_cents / 3);
  if (cadence === 'weekly') return Math.round(s.amount_cents * (52 / 12));
  if (cadence === 'monthly' || cadence === '') return s.amount_cents;
  // Bi-monthly / semi-annually / etc. — keep as-is for the model to
  // judge rather than silently truncating.
  return s.amount_cents;
}

// ─── Snapshot ─────────────────────────────────────────────────────

export const SNAPSHOT_SYSTEM_PROMPT =
  'You are a calm, frank financial assistant writing a one-screen snapshot for the user. ' +
  'No corporate sludge, no false reassurance. Style: a friend with a CFA who looked at your numbers for a minute.\n\n' +
  'Output STRICTLY three short paragraphs. No headers, no bullets, no preamble, no sign-off. Total length 70-130 words.\n\n' +
  "  Paragraph 1 (1-2 sentences): The picture. Net worth + monthly run rate (income − recurring outflow). State it plainly. If the run rate is negative, name that.\n" +
  "  Paragraph 2 (2-3 sentences): The biggest lever. One concrete thing the user could change THIS WEEK to improve the picture — name a specific subscription, income stream, or goal in their data, not a generic suggestion like 'reduce spending'.\n" +
  "  Paragraph 3 (1 sentence): One thing to watch over the next month — e.g. an upcoming renewal cluster, an income stream marked pipeline that needs activation, a goal whose pace is off.\n\n" +
  "Constraints: NEVER invent numbers the user didn't provide. NEVER recommend specific investment products. NEVER name specific funds or stocks. " +
  "When data is sparse, write less — under 70 words is fine. No exclamation marks, no \"!\".";

export interface SnapshotInputs {
  overview: FinOverview;
  subscriptions: FinSubscription[];
  streams: FinIncomeStream[];
  goals: FinGoal[];
}

export function buildSnapshotPrompt(input: SnapshotInputs): string {
  const ov = input.overview;
  const lines: string[] = [];
  lines.push(`Currency: ${ov.currency || 'EUR'}`);
  lines.push(`Net worth: ${fmtCents(ov.net_worth_cents, ov.currency)}`);
  lines.push(`Assets: ${fmtCents(ov.assets_cents, ov.currency)}`);
  lines.push(`Liabilities: ${fmtCents(ov.liabilities_cents, ov.currency)}`);
  lines.push(`Monthly income (actual): ${fmtCents(ov.income_monthly_actual_cents, ov.currency)}`);
  if (ov.income_monthly_projected_cents > ov.income_monthly_actual_cents) {
    lines.push(`Monthly income (projected with pipeline): ${fmtCents(ov.income_monthly_projected_cents, ov.currency)}`);
  }
  lines.push(`Monthly subscriptions: ${fmtCents(ov.subscription_monthly_cents, ov.currency)}`);
  lines.push(`Run rate (income − subs): ${fmtCents(ov.income_monthly_actual_cents - ov.subscription_monthly_cents, ov.currency)}`);
  if (ov.upcoming_subs_count > 0) {
    lines.push(`Subscriptions renewing in next 7 days: ${ov.upcoming_subs_count}`);
  }

  // Costliest subs — top 5 by monthly-normalised amount so the model
  // can pick one to name in its "biggest lever" paragraph.
  const topSubs = [...input.subscriptions]
    .filter((s) => s.active !== false)
    .sort((a, b) => monthlyAmountCents(b) - monthlyAmountCents(a))
    .slice(0, 5);
  if (topSubs.length > 0) {
    lines.push('');
    lines.push('Top active subscriptions by monthly cost:');
    for (const s of topSubs) {
      lines.push(
        `- ${s.name}: ${fmtCents(monthlyAmountCents(s), s.currency)}/mo (${s.cadence || 'monthly'})`
      );
    }
  }

  // Pipeline income — flagged so the model can suggest activation.
  const pipeline = input.streams.filter((s) => s.status === 'pipeline').slice(0, 3);
  if (pipeline.length > 0) {
    lines.push('');
    lines.push('Income streams in pipeline (not yet earning):');
    for (const s of pipeline) {
      lines.push(`- ${s.name}: projected ${fmtCents(s.projected_monthly_cents, s.currency)}/mo`);
    }
  }

  // Active money goals — capped at 3 for prompt size.
  const activeGoals = input.goals.filter((g) => (g.status ?? 'active') === 'active').slice(0, 3);
  if (activeGoals.length > 0) {
    lines.push('');
    lines.push('Active money goals:');
    for (const g of activeGoals) {
      const tgt = g.target_cents
        ? ` (target ${fmtCents(g.target_cents, g.currency || ov.currency)}`
        : '';
      const cur = g.current_cents !== undefined ? `; at ${fmtCents(g.current_cents, g.currency || ov.currency)})` : ')';
      lines.push(`- ${g.name}${tgt}${tgt ? cur : ''}`);
    }
  }

  return lines.join('\n');
}

// ─── Subscription audit ───────────────────────────────────────────

export const SUB_AUDIT_SYSTEM_PROMPT =
  'You are an audit assistant looking at a list of recurring subscriptions. Your job: surface 3-6 candidates for cancellation, downgrade, or consolidation. Be specific and direct.\n\n' +
  'Output a markdown bullet list. Each bullet:\n' +
  '  - Lead with the subscription NAME exactly as the user wrote it.\n' +
  "  - Then a colon and a short reason: \"costliest\", \"likely overlaps with X\", \"high relative to income\", \"could be downgraded\", etc.\n" +
  '  - One sentence max per bullet.\n\n' +
  "Order by impact (largest annual saving first). Don't include subscriptions you can't justify flagging. Don't recommend ALL of them — pick a useful 3-6.\n\n" +
  "When the input has fewer than 3 subscriptions, write a one-line note saying there's not enough to audit yet. No exclamation marks.";

export interface SubAuditInputs {
  subscriptions: FinSubscription[];
  monthlyIncomeCents: number;
  currency: string;
}

export function buildSubAuditPrompt(input: SubAuditInputs): string {
  const active = input.subscriptions.filter((s) => s.active !== false);
  const lines: string[] = [];
  lines.push(`Currency: ${input.currency || 'EUR'}`);
  if (input.monthlyIncomeCents > 0) {
    lines.push(`Monthly income: ${fmtCents(input.monthlyIncomeCents, input.currency)}`);
  }
  lines.push('');
  lines.push(`Active subscriptions (${active.length}):`);
  for (const s of active) {
    const monthly = monthlyAmountCents(s);
    const annual = monthly * 12;
    const cat = s.category ? ` [${s.category}]` : '';
    lines.push(
      `- ${s.name}: ${fmtCents(monthly, s.currency)}/mo · ${fmtCents(annual, s.currency)}/yr (${s.cadence || 'monthly'})${cat}`
    );
  }
  return lines.join('\n');
}
