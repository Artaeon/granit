// Lock-in (save) flow for the morning ritual.
//
// Seventh extraction step out of routes/morning/+page.svelte. Wraps
// the api.saveMorning round-trip plus the post-save
// clearPersisted+toast+goto orchestration. Saves a daily note shaped
// like:
//
//   scripture: { text, source }
//   goal:      "<focus sentence> — contributes to: <goal title>"
//   tasks:     picked task texts
//   habits:    picked habit names
//   thoughts:  Today's win: ...
//              Praying for: ...
//              <free-text thoughts>
//
// Prayer intentions ride along in the thoughts block under a
// `Praying for:` heading — server has no dedicated prayer field, and
// keeping them in thoughts means the daily note stays self-contained
// without a schema change.
//
// Pure pieces (buildPrayerBlock, buildThoughtsBody) are extracted as
// helpers so a future test can assert the markdown shape without
// instantiating the controller.

import type { PrayerIntention, Goal } from '$lib/api';
import { goto } from '$app/navigation';
import { toast } from '$lib/components/toast';
import { errorMessage } from '$lib/util/errorMessage';

export interface SaveMorningBody {
  scripture?: { text: string; source: string };
  goal?: string;
  tasks?: string[];
  habits?: string[];
  thoughts?: string;
}

/** Build the "Praying for:" list for the thoughts block. Returns an
 *  empty string when no intentions are picked, so the joining caller
 *  can drop the section cleanly. */
export function buildPrayerBlock(args: {
  pickedIntentions: ReadonlySet<string>;
  activeIntentions: PrayerIntention[];
}): string {
  const lines: string[] = [];
  for (const id of args.pickedIntentions) {
    const intent = args.activeIntentions.find((x) => x.id === id);
    if (!intent) continue;
    let line = `- ${intent.text}`;
    if (intent.venture) line += ` (🏢 ${intent.venture})`;
    else if (intent.project) line += ` (📁 ${intent.project})`;
    else if (intent.person) line += ` (👤 ${intent.person})`;
    if (intent.passage_ref) line += ` — ${intent.passage_ref}`;
    lines.push(line);
  }
  return lines.length > 0 ? `Praying for:\n${lines.join('\n')}` : '';
}

/** Compose the thoughts body from the win sentence, the prayer
 *  block, and the user's free-text thoughts. Returns undefined when
 *  every part is empty so api.saveMorning can drop the field. */
export function buildThoughtsBody(args: {
  winSentence: string;
  prayerBlock: string;
  thoughts: string;
}): string | undefined {
  const winLine = args.winSentence.trim();
  const winPart = winLine ? `Today's win: ${winLine}` : '';
  const thoughtsRaw = args.thoughts.trim();
  return (
    [winPart, args.prayerBlock, thoughtsRaw]
      .filter((s) => s.length > 0)
      .join('\n\n') || undefined
  );
}

/** Compose the goal field with the optional "contributes to" tail. */
export function buildGoalForSave(args: {
  goal: string;
  linked: Goal | undefined;
}): string | undefined {
  const goalText = args.goal.trim();
  if (!goalText) return undefined;
  return args.linked ? `${goalText} — contributes to: ${args.linked.title}` : goalText;
}

export interface MorningLockInDeps {
  /** Reactive accessors — read at save time so the API sees the
   *  freshest field values. */
  getActiveScripture: () => { text: string; source: string };
  getActiveGoals: () => Goal[];
  getActiveIntentions: () => PrayerIntention[];
  getFocus: () => {
    winSentence: string;
    goal: string;
    linkedGoalId: string;
  };
  getPicks: () => {
    pickedTaskTexts: string[];
    pickedHabits: ReadonlySet<string>;
    pickedIntentions: ReadonlySet<string>;
  };
  getThoughts: () => string;

  /** Side-effects called after a successful save. Injected so tests
   *  don't have to stub localStorage / goto / toast. */
  clearPersisted: () => void;

  /** api.saveMorning binding. */
  saveMorning: (body: SaveMorningBody) => Promise<unknown>;
}

export interface MorningLockInController {
  readonly saving: boolean;
  /** Last save error message. Empty after a successful save. */
  readonly error: string;
  lockIn(): Promise<void>;
}

export function createMorningLockIn(
  deps: MorningLockInDeps
): MorningLockInController {
  let saving = $state(false);
  let error = $state('');

  async function lockIn() {
    saving = true;
    error = '';
    try {
      const focus = deps.getFocus();
      const picks = deps.getPicks();
      const linked = deps
        .getActiveGoals()
        .find((g) => g.id === focus.linkedGoalId);
      const goalForSave = buildGoalForSave({
        goal: focus.goal,
        linked
      });
      const prayerBlock = buildPrayerBlock({
        pickedIntentions: picks.pickedIntentions,
        activeIntentions: deps.getActiveIntentions()
      });
      const thoughtsBody = buildThoughtsBody({
        winSentence: focus.winSentence,
        prayerBlock,
        thoughts: deps.getThoughts()
      });
      const activeScripture = deps.getActiveScripture();
      await deps.saveMorning({
        scripture: activeScripture.text ? activeScripture : undefined,
        goal: goalForSave,
        tasks: picks.pickedTaskTexts,
        habits: Array.from(picks.pickedHabits),
        thoughts: thoughtsBody
      });
      deps.clearPersisted();
      toast.success('today is locked in');
      goto('/');
    } catch (e) {
      const msg = errorMessage(e);
      error = msg;
      toast.error(`save failed: ${msg}`);
    } finally {
      saving = false;
    }
  }

  return {
    get saving() {
      return saving;
    },
    get error() {
      return error;
    },
    lockIn
  };
}
