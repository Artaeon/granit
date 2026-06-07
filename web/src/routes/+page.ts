import { redirect } from '@sveltejs/kit';

// The workspace is Granit's home surface — a canvas/desktop that shows
// your widgets (the dashboard pane) and where everything opens. Opening
// the app lands you there. The editable widget dashboard now lives at
// /dashboard (reached via the "Today" nav + the canvas "Customize"
// link). SPA (ssr=false) so this redirect runs client-side on load.
export const load = () => {
  redirect(307, '/workspace');
};
