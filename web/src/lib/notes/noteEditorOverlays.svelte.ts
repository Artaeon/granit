// Overlay / drawer toggle state cluster for the notes editor page.
//
// Eight boolean flags that previously sat as independent `$state`
// lets at the top of the route, all with the same one-liner
// signature: open / close some self-contained component mounted at
// the page root. Pulling them together keeps the page's open-state
// declarations to one block and lets future overlays grow the cluster
// without re-spelling the same boilerplate.
//
// No coupling between members beyond convenience: each open / close
// is independent, and the page can open more than one at once
// (e.g. the audio player + the history panel — both legitimate
// concurrent surfaces while reviewing a note).

export interface NoteEditorOverlays {
  // Drawer state for the tree (left) and info rail (right) — only
  // mounted off the lg/xl breakpoints where the desktop aside is
  // hidden. Open via the header hamburger / info button.
  treeDrawerOpen: boolean;
  infoDrawerOpen: boolean;
  // Overflow menu (⋯) anchored at the header — secondary actions
  // (find, history, PDF, slideshow, audio, reading, focus,
  // flashcards, help). Trigger ref lives on the controller too so
  // the menu can compute its viewport-clamped position without the
  // page wiring a separate $state slot for the ref.
  overflowOpen: boolean;
  // Modal-style overlays rooted at the page bottom.
  printOpen: boolean;
  historyOpen: boolean;
  helpOpen: boolean;
  audioOpen: boolean;
  presentationOpen: boolean;
  // Annotation count for the active note — surfaced via the section
  // header badge so the user sees at a glance how many annotations
  // the current note carries without scrolling. AnnotationsPanel
  // owns the load + WS refresh and binds to this slot.
  annotationCount: number;
}

export function createNoteEditorOverlays(): NoteEditorOverlays {
  let treeDrawerOpen = $state(false);
  let infoDrawerOpen = $state(false);
  let overflowOpen = $state(false);
  let printOpen = $state(false);
  let historyOpen = $state(false);
  let helpOpen = $state(false);
  let audioOpen = $state(false);
  let presentationOpen = $state(false);
  let annotationCount = $state(0);

  return {
    get treeDrawerOpen() { return treeDrawerOpen; }, set treeDrawerOpen(v) { treeDrawerOpen = v; },
    get infoDrawerOpen() { return infoDrawerOpen; }, set infoDrawerOpen(v) { infoDrawerOpen = v; },
    get overflowOpen() { return overflowOpen; }, set overflowOpen(v) { overflowOpen = v; },
    get printOpen() { return printOpen; }, set printOpen(v) { printOpen = v; },
    get historyOpen() { return historyOpen; }, set historyOpen(v) { historyOpen = v; },
    get helpOpen() { return helpOpen; }, set helpOpen(v) { helpOpen = v; },
    get audioOpen() { return audioOpen; }, set audioOpen(v) { audioOpen = v; },
    get presentationOpen() { return presentationOpen; }, set presentationOpen(v) { presentationOpen = v; },
    get annotationCount() { return annotationCount; }, set annotationCount(v) { annotationCount = v; }
  };
}
