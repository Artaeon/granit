// Curated habit templates. Picking "Faith Stack" should plant
// prayer + scripture + examen with one click instead of typing
// each name in the add-habit form. Pure data — no IO, no state,
// no reactive bindings. The dialog controller reads this list,
// the apply flow turns each entry into an api.createHabit call
// (or, if categories/tags aren't accepted at create time, a
// create + patch chain).
//
// Categories / tags / frequencies are advisory — the create call
// silently drops any field the backend doesn't honour yet, so
// adding new template fields here doesn't break older servers.
// Keep names short (2-5 words) and concrete so a fresh checkbox
// line in today's ## Habits reads cleanly.

export interface HabitTemplateItem {
  name: string;
  category?: string;
  tags?: string[];
  /** "daily" | "weekly" | "3x-week" — same tag-style values the
   *  task recurrence picker uses. Templates pre-fill the most
   *  obvious cadence; the user can change it after adoption. */
  frequency?: string;
  /** HH:MM 24h — when set, the morning-rhythm view can ping the
   *  user at this time. Most templates leave it unset and let the
   *  user pick. */
  reminderTime?: string;
}

export interface HabitTemplate {
  id: string;
  name: string;
  description: string;
  habits: HabitTemplateItem[];
}

export const HABIT_TEMPLATES: HabitTemplate[] = [
  {
    id: 'health-basics',
    name: 'Health Basics',
    description: 'The three rocks: walk, water, sleep. Hard to outrun a bad day on these.',
    habits: [
      { name: 'walk 20 min', category: 'Health', tags: ['movement'], frequency: 'daily' },
      { name: 'drink 2L water', category: 'Health', tags: ['hydration'], frequency: 'daily' },
      { name: 'sleep by 23:00', category: 'Health', tags: ['sleep'], frequency: 'daily', reminderTime: '22:30' }
    ]
  },
  {
    id: 'faith-stack',
    name: 'Faith Stack',
    description: 'Morning prayer, scripture reading, evening examen. The classical rhythm.',
    habits: [
      { name: 'morning prayer', category: 'Faith', tags: ['prayer'], frequency: 'daily', reminderTime: '07:00' },
      { name: 'scripture reading', category: 'Faith', tags: ['scripture'], frequency: 'daily' },
      { name: 'examen', category: 'Faith', tags: ['reflection'], frequency: 'daily', reminderTime: '21:30' }
    ]
  },
  {
    id: 'deep-work',
    name: 'Deep Work',
    description: 'Phone away in the morning, one protected block, end-of-day review.',
    habits: [
      { name: 'no-phone morning', category: 'Focus', tags: ['attention'], frequency: 'daily' },
      { name: 'deep-work block', category: 'Focus', tags: ['focus'], frequency: 'daily' },
      { name: 'daily review', category: 'Focus', tags: ['review'], frequency: 'daily', reminderTime: '17:30' }
    ]
  },
  {
    id: 'strength',
    name: 'Strength',
    description: 'Mobility, strength training, protein floor. Aimed at building, not just maintaining.',
    habits: [
      { name: 'mobility 10 min', category: 'Health', tags: ['movement', 'mobility'], frequency: 'daily' },
      { name: 'strength training', category: 'Health', tags: ['training'], frequency: '3x-week' },
      { name: 'protein target', category: 'Health', tags: ['nutrition'], frequency: 'daily' }
    ]
  },
  {
    id: 'learning',
    name: 'Learning',
    description: 'Read every day, weekly review, one protected study block.',
    habits: [
      { name: 'read 20 min', category: 'Learning', tags: ['reading'], frequency: 'daily' },
      { name: 'weekly review', category: 'Learning', tags: ['review'], frequency: 'weekly' },
      { name: 'study block', category: 'Learning', tags: ['study'], frequency: 'daily' }
    ]
  }
];
