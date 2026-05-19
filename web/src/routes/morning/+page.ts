import { redirect } from '@sveltejs/kit';

// /morning used to host the 8-section morning ritual (scripture +
// win sentence + goal + tasks + habits + prayer + thoughts + AI
// briefing). The 2026-05-19 Rhythmus-OS pivot replaced the morning
// surface with the Heute-Karte on `/`: a quieter 3-question
// check-in plus the five-pillar card. The underlying features that
// /morning aggregated — scripture, prayer, habits, jots, the AI
// briefing — still live on their own routes (and the Heute-Karte
// header offers the briefing as a one-click).
//
// The route stays bookmarkable: any link / muscle-memory hit to
// /morning lands on the new home instead of 404-ing.
//
// Bookmarks from before the pivot land on the new home. Removing
// the route entirely would 404 those URLs and the home is more
// useful than a "not found".
export const load = () => {
  throw redirect(302, '/');
};
