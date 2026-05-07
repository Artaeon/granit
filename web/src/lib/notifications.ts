// Web Push subscribe / unsubscribe flow. Wraps PushManager so the
// settings UI doesn't have to deal with conversion of the VAPID
// public key from base64-url to a Uint8Array, or with the multi-
// step "ask permission → register SW → subscribe → POST to server"
// dance.
//
// All functions are no-ops when the browser doesn't support push
// or when the SW isn't registered. Callers can safely call them
// from any device — the iOS PWA / desktop / Android paths all
// converge on the same Subscription endpoint shape.

import { req } from '$lib/api';

export interface PushStatus {
  supported: boolean;
  permission: NotificationPermission;
  subscribed: boolean;
}

const TOKEN_KEY = 'everything.token';

/** Returns whether the browser supports the Web Push pipeline. */
export function isSupported(): boolean {
  if (typeof window === 'undefined') return false;
  return (
    'serviceWorker' in navigator &&
    'PushManager' in window &&
    'Notification' in window
  );
}

/** Convert a VAPID public key from URL-base64 to the Uint8Array
 *  format PushManager.subscribe wants. The applicationServerKey
 *  field accepts either a raw public key or this byte array; we
 *  pre-convert because it's the more reliable path across browsers. */
function urlBase64ToUint8Array(b64: string): Uint8Array {
  const padding = '='.repeat((4 - (b64.length % 4)) % 4);
  const base64 = (b64 + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(base64);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

/** Read the current subscription state, asking the registered SW
 *  for any existing subscription. Doesn't request permission or
 *  subscribe — pure observer. */
export async function getStatus(): Promise<PushStatus> {
  if (!isSupported()) {
    return { supported: false, permission: 'denied', subscribed: false };
  }
  const reg = await navigator.serviceWorker.ready;
  const sub = await reg.pushManager.getSubscription();
  return {
    supported: true,
    permission: Notification.permission,
    subscribed: !!sub
  };
}

/** Full subscribe flow: request permission if needed, subscribe
 *  the SW with the server's VAPID key, POST the result to the
 *  /api/v1/push/subscribe endpoint. Throws on permission denial
 *  or subscribe failure so the caller can surface an error toast. */
export async function subscribe(): Promise<PushStatus> {
  if (!isSupported()) {
    throw new Error('Push notifications not supported in this browser.');
  }
  const perm = Notification.permission === 'default'
    ? await Notification.requestPermission()
    : Notification.permission;
  if (perm !== 'granted') {
    throw new Error('Notification permission denied.');
  }
  const reg = await navigator.serviceWorker.ready;
  // Re-use an existing subscription if the browser still has one
  // (PushManager.subscribe is idempotent but the underlying push
  // service may rotate keys; existing-first avoids surprises).
  let sub = await reg.pushManager.getSubscription();
  if (!sub) {
    const { key } = await req<{ key: string }>('/push/vapid');
    if (!key) throw new Error('Server returned no VAPID key.');
    sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      // Cast to BufferSource — TS lib.dom narrows to ArrayBuffer
      // but PushManager accepts any ArrayBufferView. Newer libs
      // tightened this in a way the runtime doesn't care about.
      applicationServerKey: urlBase64ToUint8Array(key) as unknown as BufferSource
    });
  }
  // Convert the browser Subscription into the JSON shape the
  // server expects. PushManager already exposes toJSON() but the
  // typings don't, so we hand-roll the shape from getKey calls.
  const json = sub.toJSON() as {
    endpoint: string;
    keys?: { p256dh: string; auth: string };
  };
  if (!json.keys?.p256dh || !json.keys?.auth) {
    throw new Error('Subscription is missing keys.');
  }
  await req('/push/subscribe', {
    method: 'POST',
    body: JSON.stringify({
      endpoint: json.endpoint,
      keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
      label: deriveLabel()
    })
  });
  return {
    supported: true,
    permission: 'granted',
    subscribed: true
  };
}

export async function unsubscribe(): Promise<void> {
  if (!isSupported()) return;
  const reg = await navigator.serviceWorker.ready;
  const sub = await reg.pushManager.getSubscription();
  if (!sub) return;
  // Tell the server first so it stops trying to push to the dead
  // endpoint. Then unsubscribe the browser-side. Order matters —
  // if the browser-side unsub races the server-side, we might fire
  // a push that ends up at a 410-Gone, which the server's stale
  // sweep would handle but generates spurious errors.
  try {
    await req('/push/unsubscribe', {
      method: 'POST',
      body: JSON.stringify({ endpoint: sub.endpoint })
    });
  } catch {}
  await sub.unsubscribe();
}

/** Send a test push to every stored subscription. Wired to the
 *  "Test" button in settings. */
export async function sendTest(): Promise<{ sent: number; errors?: string[] }> {
  return req<{ sent: number; errors?: string[] }>('/push/test', {
    method: 'POST'
  });
}

// Friendly device label so the server can show users a list of
// active subscriptions ("iPhone Safari", "Chrome on macOS"). Falls
// back to "Browser" when UA detection misses.
function deriveLabel(): string {
  if (typeof navigator === 'undefined') return 'Browser';
  const ua = navigator.userAgent;
  let device = 'Browser';
  if (/iPhone/.test(ua)) device = 'iPhone';
  else if (/iPad/.test(ua)) device = 'iPad';
  else if (/Android/.test(ua)) device = 'Android';
  else if (/Macintosh/.test(ua)) device = 'Mac';
  else if (/Windows/.test(ua)) device = 'Windows';
  else if (/Linux/.test(ua)) device = 'Linux';
  let app = '';
  if (/Firefox\//.test(ua)) app = 'Firefox';
  else if (/Edg\//.test(ua)) app = 'Edge';
  else if (/Chrome\//.test(ua)) app = 'Chrome';
  else if (/Safari\//.test(ua)) app = 'Safari';
  return app ? `${device} ${app}` : device;
}
