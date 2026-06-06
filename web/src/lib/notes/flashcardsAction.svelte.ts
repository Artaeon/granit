// Flashcard-scheduling action for the notes editor's overflow menu.
//
// Parses Q:/A: pairs out of the current body (the format the
// InlineAIMenu "flashcards" preset emits) and creates a 5-step
// spaced-rep series per card at 1/3/7/14/30 day offsets — same
// cadence as the scripture memory-verse drill. The busy flag keeps
// the overflow menu from queueing duplicate runs while the API is
// still working through the burst.
//
// Pure orchestration glue — the actual scheduling logic lives in
// $lib/util/scheduleFlashcards. This controller owns the toast
// branches, the busy gate, and the overflow-close handshake.

import { scheduleFlashcards } from '$lib/util/scheduleFlashcards';
import { toast } from '$lib/components/toast';

export interface FlashcardsAction {
  readonly schedulingFlashcards: boolean;
  /** Invoke the scheduler. Closes the overflow menu first via the
   *  provided callback, runs the schedule, surfaces a toast. */
  run: () => Promise<void>;
}

export interface FlashcardsActionOpts {
  /** Source body — pulled at the moment the user fires the action
   *  so any in-flight typing lands in the schedule. */
  getBody: () => string;
  /** Close the overflow menu before the long-running run. */
  closeOverflow: () => void;
}

export function createFlashcardsAction(
  opts: FlashcardsActionOpts
): FlashcardsAction {
  let schedulingFlashcards = $state(false);

  async function run(): Promise<void> {
    if (schedulingFlashcards) return;
    schedulingFlashcards = true;
    opts.closeOverflow();
    try {
      const r = await scheduleFlashcards(opts.getBody());
      if (r.cards === 0) {
        toast.info('No Q:/A: flashcards found in this note.');
        return;
      }
      if (r.failed === 0) {
        toast.success(
          `Scheduled ${r.cards} card${r.cards === 1 ? '' : 's'} × 5 reviews (1/3/7/14/30 days).`
        );
      } else {
        toast.info(
          `Scheduled ${r.scheduled} of ${r.cards * 5} reviews — ${r.failed} failed.`
        );
      }
    } catch (e) {
      toast.error(
        'Schedule failed: ' + (e instanceof Error ? e.message : String(e))
      );
    } finally {
      schedulingFlashcards = false;
    }
  }

  return {
    get schedulingFlashcards() { return schedulingFlashcards; },
    run
  };
}
