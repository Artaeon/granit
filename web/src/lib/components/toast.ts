import { writable } from 'svelte/store';

export type ToastKind = 'info' | 'success' | 'warning' | 'error';

export interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
  /** Auto-dismiss after this many ms. 0 = sticky. */
  ttl: number;
}

let nextId = 1;

export const toasts = writable<Toast[]>([]);

function push(kind: ToastKind, message: string, ttl = 4000): number {
  const id = nextId++;
  toasts.update((list) => [...list, { id, kind, message, ttl }]);
  if (ttl > 0) {
    setTimeout(() => dismiss(id), ttl);
  }
  return id;
}

export function dismiss(id: number) {
  toasts.update((list) => list.filter((t) => t.id !== id));
}

export const toast = {
  info: (m: string, ttl = 4000) => push('info', m, ttl),
  success: (m: string, ttl = 3000) => push('success', m, ttl),
  warning: (m: string, ttl = 5000) => push('warning', m, ttl),
  error: (m: string, ttl = 6000) => push('error', m, ttl),
  dismiss
};
