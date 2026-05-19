<script lang="ts">
  // The "what now?" card. Pure presentation of the nextAction()
  // result — the rule engine decides the verb, this component
  // makes it visible.
  //
  // No buttons inside, on purpose: the user marks the pillar done
  // on its own row when it's done. Coupling "do the thing" + "mark
  // it done" onto the same button rewards lying to the rhythm,
  // which defeats the whole point.
  //
  // Visual weight matches the rule's tone: rest gets a quiet green,
  // evening gets a deep mauve, food gets a warning amber, work and
  // body stay neutral. The user should be able to glance at the
  // colour and feel "the app agrees with where I am".

  import type { NextAction } from './nextAction';

  type Props = { action: NextAction };
  let { action }: Props = $props();

  const TONE_BY_PILLAR: Record<NextAction['pillar'], string> = {
    food:    'border-warning bg-warning/5',
    evening: 'border-mauve bg-mauve/5',
    work:    'border-secondary bg-secondary/5',
    body:    'border-secondary bg-secondary/5',
    spirit:  'border-secondary bg-secondary/5',
    rest:    'border-success bg-success/5'
  };
</script>

<section class="rounded-lg border p-5 {TONE_BY_PILLAR[action.pillar]}">
  <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Was jetzt?</div>
  <p class="text-xl text-text font-medium leading-snug">{action.label}</p>
  <p class="text-xs text-dim mt-2">{action.reason}</p>
</section>
