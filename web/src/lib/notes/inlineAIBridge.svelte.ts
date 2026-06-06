// Bridge between the CodeMirror inline-AI extension family and the
// notes editor's Svelte surface.
//
// CodeMirror state can't be subscribed to directly from Svelte; we
// inject two CM extensions that forward events into reactive Svelte
// $state slots:
//
//   • inlineAITriggerExtension — fires when the user opens the
//     inline AI menu (Cmd-K / typing "/ai"). The page renders
//     <InlineAIMenu> off `triggerEvent`; the menu calls back through
//     `clearTrigger` (Stop / Close distinction: the menu owns the
//     stream cancel — this just dismisses the menu).
//
//   • inlineAIObserver — fires whenever the inline-AI state field
//     changes (start / stream / done / clear). The page renders
//     <AIActionBar> off `ghostState` so the floating Keep / Try
//     again / Discard / Stop chip follows the ghost.
//
// Why an array constant: Editor.svelte reads extraExtensions ONCE at
// setupView time, so the array must not be re-created on every
// render. The controller exposes it via a stable readonly getter.

import {
  inlineAITriggerExtension,
  type InlineAITriggerEvent
} from '$lib/editor/inline-ai-trigger';
import { inlineAIObserver, type InlineAIState } from '$lib/editor/inline-ai';
import type { Extension } from '@codemirror/state';

export interface InlineAIBridge {
  /** Set by inlineAITriggerExtension, cleared by the menu's onClose. */
  readonly triggerEvent: InlineAITriggerEvent | null;
  /** Cleared from the page's InlineAIMenu onClose. */
  clearTrigger: () => void;
  /** Set by inlineAIObserver, drives the floating AIActionBar. */
  readonly ghostState: InlineAIState | null;
  /** Stable array — Editor.svelte reads extraExtensions ONCE. */
  readonly extensions: Extension[];
}

export function createInlineAIBridge(): InlineAIBridge {
  let triggerEvent = $state<InlineAITriggerEvent | null>(null);
  let ghostState = $state<InlineAIState | null>(null);

  const extensions: Extension[] = [
    inlineAITriggerExtension((e) => {
      triggerEvent = e;
    }),
    inlineAIObserver((s) => {
      ghostState = s;
    })
  ];

  return {
    get triggerEvent() { return triggerEvent; },
    clearTrigger: () => { triggerEvent = null; },
    get ghostState() { return ghostState; },
    get extensions() { return extensions; }
  };
}
