// dragStore is the discriminator that lets HourGrid distinguish a
// task-from-backlog drop from its existing slot-drag-to-create or
// event-reschedule flows. When non-null, HourGrid renders a ghost on
// the hovered slot using the dragged task's title and — on
// pointerup — fires onTaskDrop(taskId, start, durationMinutes).
//
// We deliberately use a writable Svelte store rather than HTML5
// drag-and-drop: HourGrid already owns pointer-capture for slot drags
// and event reschedules (setPointerCapture on the column /event div),
// and HTML5 drag would fight that capture. A shared store reading
// pointer state from BOTH sides keeps the state machine readable.
//
// Two trigger modes share the same channel:
//   - Desktop drag: TaskBacklog setPointerCapture's the row, then
//     pointermove fires on HourGrid via the captured pointerId. The
//     pointerId field is set so HourGrid can correlate.
//   - Mobile tap: TaskBacklog sets a pending pick (pointerId = -1).
//     The next pointerdown on a HourGrid slot consumes it as a tap.
import { writable } from 'svelte/store';

export interface DraggedTask {
  taskId: string;
  title: string;
  durationMinutes: number;
  /** Pointer ID of the active drag, or -1 for a mobile tap-to-pick. */
  pointerId: number;
}

export const dragStore = writable<DraggedTask | null>(null);
