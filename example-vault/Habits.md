---
type: reference
tags: [meta, habits]
created: 2026-04-01
---

# Habits

Track daily habits inside each daily note under `## Habits`. The web dashboard derives streaks and completion rates by scanning recent dailies.

## Current habits

- **gym / fighting sports daily** — at least 30 min of movement
- **daily blog writing / journaling** — 200+ words to clear my head
- **daily praying** — morning + evening
- **read 20 pages** — book or substack longform
- **no doomscrolling after 21:00**

## How it works

In your daily note (e.g. `Jots/2026-05-01.md`), include this section:

```markdown
## Habits

- [ ] gym / fighting sports daily
- [ ] daily blog writing / journaling
- [ ] daily praying
- [ ] read 20 pages
- [ ] no doomscrolling after 21:00
```

Toggle the checkboxes as you complete each habit. The web app:

- Aggregates the last 60 days of dailies
- Computes current streak per habit
- Shows last-7-days completion % per habit
- Renders a dot grid like GitHub contributions

You can also use granit's TUI habit tracker (`Alt+H` → habit panel) — both views read the same daily-note checkboxes, so they stay in sync.

## Adding a habit

Just add a new `- [ ] habit name` line to your daily note's `## Habits` section. The next refresh picks it up.

## Removing a habit

Stop adding it to new dailies — it'll fade out of the web dashboard automatically once it hasn't appeared in recent days.
