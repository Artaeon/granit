import { describe, expect, it } from 'vitest';
import {
  SNAPSHOT_SYSTEM_PROMPT,
  SUB_AUDIT_SYSTEM_PROMPT,
  buildSnapshotPrompt,
  buildSubAuditPrompt
} from './aiPrompts';
import type { FinOverview, FinSubscription, FinIncomeStream, FinGoal } from '$lib/api';

function emptyOverview(): FinOverview {
  return {
    currency: 'EUR',
    net_worth_cents: 0,
    assets_cents: 0,
    liabilities_cents: 0,
    income_monthly_actual_cents: 0,
    income_monthly_projected_cents: 0,
    subscription_monthly_cents: 0,
    upcoming_subs_count: 0,
    accounts_count: 0,
    income_active_count: 0,
    income_pipeline_count: 0,
    goals_active_count: 0
  };
}

describe('SNAPSHOT_SYSTEM_PROMPT', () => {
  it('mandates 3 short paragraphs', () => {
    expect(SNAPSHOT_SYSTEM_PROMPT.toLowerCase()).toContain('three short paragraphs');
  });

  it('caps at 70-130 words', () => {
    expect(SNAPSHOT_SYSTEM_PROMPT).toContain('70-130 words');
  });

  it('forbids inventing numbers + investment products', () => {
    const lc = SNAPSHOT_SYSTEM_PROMPT.toLowerCase();
    expect(lc).toContain('never invent numbers');
    expect(lc).toContain('never recommend specific investment products');
    expect(lc).toContain('never name specific funds');
  });

  it('describes the picture / lever / watch contract', () => {
    const lc = SNAPSHOT_SYSTEM_PROMPT.toLowerCase();
    expect(lc).toContain('the picture');
    expect(lc).toContain('the biggest lever');
    expect(lc).toContain('one thing to watch');
  });
});

describe('buildSnapshotPrompt', () => {
  it('reports net worth, run rate, and currency', () => {
    const ov = emptyOverview();
    ov.net_worth_cents = 1_500_000; // €15,000
    ov.assets_cents = 1_800_000;
    ov.liabilities_cents = 300_000;
    ov.income_monthly_actual_cents = 500_000; // €5,000
    ov.subscription_monthly_cents = 75_000; // €750
    const got = buildSnapshotPrompt({ overview: ov, subscriptions: [], streams: [], goals: [] });
    expect(got).toContain('Currency: EUR');
    expect(got).toContain('Net worth: 15,000 EUR');
    expect(got).toContain('Monthly income (actual): 5,000 EUR');
    expect(got).toContain('Monthly subscriptions: 750 EUR');
    expect(got).toContain('Run rate (income − subs): 4,250 EUR');
  });

  it('includes top 5 subs sorted by monthly cost across cadences', () => {
    const ov = emptyOverview();
    const subs: FinSubscription[] = [
      { id: 's1', name: 'GitHub Enterprise', amount_cents: 100_000, currency: 'EUR', cadence: 'yearly', next_renewal: '', active: true, created_at: '', updated_at: '' },
      { id: 's2', name: 'Spotify', amount_cents: 1_200, currency: 'EUR', cadence: 'monthly', next_renewal: '', active: true, created_at: '', updated_at: '' },
      { id: 's3', name: 'Notion Plus', amount_cents: 1_500, currency: 'EUR', cadence: 'monthly', next_renewal: '', active: true, created_at: '', updated_at: '' },
      { id: 's4', name: 'Inactive', amount_cents: 100_000, currency: 'EUR', cadence: 'monthly', next_renewal: '', active: false, created_at: '', updated_at: '' }
    ];
    const got = buildSnapshotPrompt({ overview: ov, subscriptions: subs, streams: [], goals: [] });
    expect(got).toContain('Top active subscriptions by monthly cost');
    // €100,000 yearly = ~€8,333/mo — biggest
    expect(got).toMatch(/- GitHub Enterprise:/);
    // Spotify (12) and Notion (15) follow.
    expect(got).toContain('Spotify');
    expect(got).toContain('Notion Plus');
    // Inactive subscriptions are filtered.
    expect(got).not.toContain('Inactive');
  });

  it('flags pipeline income streams separately from active', () => {
    const ov = emptyOverview();
    const streams: FinIncomeStream[] = [
      { id: 'i1', name: 'Day job', status: 'active', kind: 'salary', projected_monthly_cents: 500_000, actual_monthly_cents: 500_000, currency: 'EUR' } as FinIncomeStream,
      { id: 'i2', name: 'Side venture', status: 'pipeline', kind: 'venture', projected_monthly_cents: 200_000, actual_monthly_cents: 0, currency: 'EUR' } as FinIncomeStream
    ];
    const got = buildSnapshotPrompt({ overview: ov, subscriptions: [], streams, goals: [] });
    expect(got).toContain('Side venture');
    expect(got).toContain('projected 2,000 EUR/mo');
    // Active stream isn't in the pipeline section.
    expect(got).not.toMatch(/pipeline.*Day job/s);
  });

  it('includes active money goals with target + current', () => {
    const ov = emptyOverview();
    const goals: FinGoal[] = [
      { id: 'g1', name: 'Emergency fund', target_cents: 1_000_000, current_cents: 250_000, currency: 'EUR', status: 'active' } as FinGoal,
      { id: 'g2', name: 'Done already', target_cents: 100_000, current_cents: 100_000, currency: 'EUR', status: 'done' } as FinGoal
    ];
    const got = buildSnapshotPrompt({ overview: ov, subscriptions: [], streams: [], goals });
    expect(got).toContain('Emergency fund');
    expect(got).toContain('target 10,000 EUR');
    expect(got).toContain('at 2,500 EUR');
    expect(got).not.toContain('Done already');
  });
});

describe('SUB_AUDIT_SYSTEM_PROMPT', () => {
  it('asks for 3-6 cancellation candidates', () => {
    expect(SUB_AUDIT_SYSTEM_PROMPT).toContain('3-6 candidates');
  });

  it('mandates markdown bullets + name first', () => {
    const lc = SUB_AUDIT_SYSTEM_PROMPT.toLowerCase();
    expect(lc).toContain('markdown bullet list');
    expect(lc).toContain('subscription name');
  });

  it('asks the model to order by impact (largest saving first)', () => {
    expect(SUB_AUDIT_SYSTEM_PROMPT.toLowerCase()).toContain('largest annual saving first');
  });
});

describe('buildSubAuditPrompt', () => {
  it('lists every active subscription with monthly + annual amounts', () => {
    const subs: FinSubscription[] = [
      { id: 's1', name: 'Netflix', amount_cents: 1_799, currency: 'EUR', cadence: 'monthly', next_renewal: '', active: true, created_at: '', updated_at: '' },
      { id: 's2', name: 'iCloud 2TB', amount_cents: 99_99, currency: 'EUR', cadence: 'yearly', next_renewal: '', active: true, created_at: '', updated_at: '' }
    ];
    const got = buildSubAuditPrompt({ subscriptions: subs, monthlyIncomeCents: 500_000, currency: 'EUR' });
    expect(got).toContain('Monthly income: 5,000 EUR');
    expect(got).toContain('Active subscriptions (2):');
    expect(got).toContain('Netflix');
    // Netflix monthly = 17.99 EUR; annual = 215.88 EUR
    expect(got).toMatch(/Netflix.*17\.99/);
    expect(got).toMatch(/Netflix.*215\.88/);
    // iCloud yearly = 99.99 EUR; monthly = 8.33
    expect(got).toMatch(/iCloud 2TB.*8\.33/);
  });

  it('drops inactive subscriptions before listing', () => {
    const subs: FinSubscription[] = [
      { id: 's1', name: 'Active', amount_cents: 100, currency: 'EUR', cadence: 'monthly', next_renewal: '', active: true, created_at: '', updated_at: '' },
      { id: 's2', name: 'Inactive', amount_cents: 100, currency: 'EUR', cadence: 'monthly', next_renewal: '', active: false, created_at: '', updated_at: '' }
    ];
    const got = buildSubAuditPrompt({ subscriptions: subs, monthlyIncomeCents: 0, currency: 'EUR' });
    expect(got).toContain('Active subscriptions (1):');
    expect(got).toContain('Active');
    expect(got).not.toContain('Inactive');
  });

  it('omits the income line when monthlyIncomeCents is zero', () => {
    const got = buildSubAuditPrompt({ subscriptions: [], monthlyIncomeCents: 0, currency: 'EUR' });
    expect(got).not.toContain('Monthly income');
    expect(got).toContain('Active subscriptions (0):');
  });
});
