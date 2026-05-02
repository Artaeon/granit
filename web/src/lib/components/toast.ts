import { writable } from 'svelte/store';

export type ToastKind = 'info' | 'success' | 'warning' | 'error';

/** Optional click-through rendered as a small inline link inside the
 *  toast body. Used by the AI-error path so the user can jump straight
 *  to /settings to fix a missing key / unreachable provider. */
export interface ToastAction {
  label: string;
  href: string;
}

export interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
  /** Auto-dismiss after this many ms. 0 = sticky. */
  ttl: number;
  /** Optional CTA link rendered inside the toast. */
  action?: ToastAction;
  /** Optional raw / detail string the user can expand. We keep this off
   *  the default surface so a "ollama: 404 {…}" noise dump doesn't
   *  drown the headline. */
  details?: string;
}

export interface ToastOptions {
  ttl?: number;
  action?: ToastAction;
  details?: string;
}

let nextId = 1;

export const toasts = writable<Toast[]>([]);

function push(kind: ToastKind, message: string, opts: ToastOptions = {}): number {
  const id = nextId++;
  const ttl = opts.ttl ?? defaultTtl(kind);
  toasts.update((list) => [
    ...list,
    { id, kind, message, ttl, action: opts.action, details: opts.details }
  ]);
  if (ttl > 0) {
    setTimeout(() => dismiss(id), ttl);
  }
  return id;
}

function defaultTtl(kind: ToastKind): number {
  switch (kind) {
    case 'success':
      return 3000;
    case 'warning':
      return 5000;
    case 'error':
      return 6000;
    default:
      return 4000;
  }
}

export function dismiss(id: number) {
  toasts.update((list) => list.filter((t) => t.id !== id));
}

// Overload-friendly API. The historical signature `toast.error(msg, ttl)`
// keeps working: a number is treated as ttl. New call sites pass an
// options object to attach a CTA action and/or expandable details.
function call(kind: ToastKind, message: string, opts?: number | ToastOptions): number {
  if (typeof opts === 'number') return push(kind, message, { ttl: opts });
  return push(kind, message, opts ?? {});
}

export const toast = {
  info: (m: string, opts?: number | ToastOptions) => call('info', m, opts),
  success: (m: string, opts?: number | ToastOptions) => call('success', m, opts),
  warning: (m: string, opts?: number | ToastOptions) => call('warning', m, opts),
  error: (m: string, opts?: number | ToastOptions) => call('error', m, opts),
  dismiss
};
