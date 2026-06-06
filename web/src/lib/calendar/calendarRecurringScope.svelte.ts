// Recurring-scope prompt — replaces the stacked native confirm()
// dialogs the drag-move + resize flows used to fire. The prompt is a
// presentational pill row (RecurringScopePicker) instead of a browser
// modal, so it doesn't block the event loop and obeys our theme.
// Wrapped in a Promise (ask) so the calling flow can `await` a user
// choice — null = cancel; the caller is responsible for reverting
// the visual drag state.
//
// Only move + resize go through this prompt. Delete still uses its
// own confirm path because the destructive tone differs (series red
// vs warning orange) and the caller is the EventDetail dialog which
// has its own UI patterns.

export type RecurringScope = 'this' | 'series';
export type RecurringAction = 'move' | 'resize';

export type RecurringScopePromptState = {
  open: boolean;
  title: string;
  action: RecurringAction;
  seriesTone: 'error' | 'warning' | 'subtext';
  onChoose: (scope: RecurringScope) => void;
  onCancel: () => void;
};

export interface CalendarRecurringScopeController {
  readonly prompt: RecurringScopePromptState | null;
  /** Open the prompt and resolve with the user's choice. Resolves
   *  with null when the user cancels. */
  ask(title: string, action: RecurringAction): Promise<RecurringScope | null>;
}

export function createCalendarRecurringScope(): CalendarRecurringScopeController {
  let prompt = $state<RecurringScopePromptState | null>(null);

  function ask(title: string, action: RecurringAction): Promise<RecurringScope | null> {
    return new Promise((resolve) => {
      prompt = {
        open: true,
        title,
        action,
        // Move/resize are less destructive than delete — the series
        // button stays warning (orange) not error (red). The picker
        // already special-cases this via the seriesTone prop.
        seriesTone: 'warning',
        onChoose: (s) => {
          prompt = null;
          resolve(s);
        },
        onCancel: () => {
          prompt = null;
          resolve(null);
        }
      };
    });
  }

  return {
    get prompt() {
      return prompt;
    },
    ask
  };
}
