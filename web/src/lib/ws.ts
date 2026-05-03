// Lightweight WebSocket client with auto-reconnect.
// Pages subscribe via onWsEvent and decide what to refetch.

import { writable, type Writable } from 'svelte/store';
import { getToken } from './api';

export type WsEvent =
  | { type: 'hello' }
  | { type: 'note.changed'; path: string }
  | { type: 'note.removed'; path: string }
  | { type: 'task.changed'; id: string }
  | { type: 'vault.rescanned' }
  | { type: 'event.changed'; id: string }
  | { type: 'event.removed'; id: string }
  | { type: 'project.changed'; id: string }
  | { type: 'project.removed'; id: string }
  | { type: 'agent.event'; id: string; data: { step: number; kind: string; text: string } }
  | {
      type: 'agent.complete';
      id: string;
      path?: string;
      data: {
        status: string;
        finalAnswer?: string;
        steps?: number;
        // Cost telemetry only present when the LLM is priced
        // (OpenAI w/ a known model). Ollama runs omit these fields.
        microCents?: number;
        promptTokens?: number;
        completionTokens?: number;
      };
    }
  | { type: 'timer.started'; id: string; data: { taskText: string } }
  | { type: 'timer.stopped'; id: string; data: { minutes: number } }
  // State files (.granit/goals.json, habits sidecars, etc.) — broadcast
  // when granitmeta or habits packages write to disk so the web doesn't
  // go stale after a TUI edit. Path is the vault-relative file path.
  | { type: 'state.changed'; path: string };

export const wsConnected: Writable<boolean> = writable(false);

const listeners = new Set<(ev: WsEvent) => void>();

let ws: WebSocket | null = null;
let backoff = 500;
let manualClose = false;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

export function onWsEvent(fn: (ev: WsEvent) => void): () => void {
  listeners.add(fn);
  // ensure we're connected when somebody listens
  if (typeof window !== 'undefined') connect();
  return () => listeners.delete(fn);
}

export function connect() {
  if (typeof window === 'undefined') return;
  const tok = getToken();
  if (!tok) return;
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;

  manualClose = false;
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }

  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const url = `${proto}://${window.location.host}/api/v1/ws`;
  // Pass the bearer through Sec-WebSocket-Protocol (the only auth
  // header browsers let us set on `new WebSocket()`). The server
  // recognizes the `granit.<tok>` form and echoes it back so the
  // handshake completes. This keeps the token out of the URL
  // querystring → out of access logs, browser history, and Referer.
  ws = new WebSocket(url, [`granit.${tok}`]);

  ws.onopen = () => {
    wsConnected.set(true);
    backoff = 500;
  };
  ws.onmessage = (msg) => {
    try {
      const ev = JSON.parse(msg.data) as WsEvent;
      for (const l of listeners) {
        try {
          l(ev);
        } catch (err) {
          console.error('ws listener error', err);
        }
      }
    } catch (err) {
      console.warn('ws bad msg', err);
    }
  };
  ws.onclose = () => {
    wsConnected.set(false);
    ws = null;
    if (manualClose) return;
    reconnectTimer = setTimeout(connect, backoff);
    backoff = Math.min(backoff * 2, 30000);
  };
  ws.onerror = () => {
    ws?.close();
  };
}

export function disconnect() {
  manualClose = true;
  if (reconnectTimer) clearTimeout(reconnectTimer);
  ws?.close();
  ws = null;
  wsConnected.set(false);
}
