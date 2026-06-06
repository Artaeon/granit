// Viewport-breakpoint controller for the notes route page.
//
// Tracks the Tailwind lg (1024px) and xl (1280px) breakpoints via
// matchMedia. The page reads `isLg` / `isXl` to decide whether to
// mount the left tree and right info-rail to the desktop aside or to
// a Drawer wrapper. Previously each rail rendered twice — once in a
// `<aside class="hidden md:flex">` and once in a `<Drawer>` wrapped
// by `md:hidden contents`. Both DOM trees were always mounted; CSS
// just hid one. That meant every panel's $derived/$effect ran twice,
// doubling the per-keystroke cost of body-derived recomputation in
// the rail panels — a meaningful chunk of the save-time freeze on
// long notes. The page now mounts each rail to ONLY one location at
// a time based on these reactive flags.
//
// Initial values come from synchronous matchMedia. SvelteKit hydrates
// the page on the client only after the bundle loads, so `window` is
// always defined here — but the typeof guard keeps SSR (if it ever
// happens) from throwing. The install function wires up live updates;
// the initializer just avoids a one-frame flash where the wrong
// layout renders before the listener fires.

export interface ViewportBreakpoints {
  readonly isLg: boolean;
  readonly isXl: boolean;
}

function matches(query: string): boolean {
  return typeof window !== 'undefined' && window.matchMedia(query).matches;
}

export function createViewportBreakpoints(): ViewportBreakpoints & {
  install: () => () => void;
} {
  let isLg = $state(matches('(min-width: 1024px)'));
  let isXl = $state(matches('(min-width: 1280px)'));

  function install(): () => void {
    if (typeof window === 'undefined') return () => {};
    const lgMql = window.matchMedia('(min-width: 1024px)');
    const xlMql = window.matchMedia('(min-width: 1280px)');
    isLg = lgMql.matches;
    isXl = xlMql.matches;
    const onLg = (e: MediaQueryListEvent) => { isLg = e.matches; };
    const onXl = (e: MediaQueryListEvent) => { isXl = e.matches; };
    lgMql.addEventListener('change', onLg);
    xlMql.addEventListener('change', onXl);
    return () => {
      lgMql.removeEventListener('change', onLg);
      xlMql.removeEventListener('change', onXl);
    };
  }

  return {
    get isLg() { return isLg; },
    get isXl() { return isXl; },
    install
  };
}
