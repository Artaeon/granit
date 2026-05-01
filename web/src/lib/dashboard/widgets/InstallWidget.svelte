<script lang="ts">
  // Bootstrap a fresh laptop / device. Static — granit's actual install commands.

  interface Cmd {
    label: string;
    desc: string;
    cmd: string;
  }

  const cmds: Cmd[] = [
    {
      label: 'Arch (AUR)',
      desc: 'fastest on Arch / Manjaro / EndeavourOS',
      cmd: 'yay -S granit-bin'
    },
    {
      label: 'From source',
      desc: 'requires Go 1.24+ — works on any Linux/macOS',
      cmd: 'git clone https://github.com/artaeon/granit.git\ncd granit\ngo install ./cmd/granit/'
    },
    {
      label: 'Open your vault',
      desc: 'after install — point granit at your synced vault',
      cmd: 'granit open ~/Documents/Main'
    },
    {
      label: 'Run granit web (this app)',
      desc: 'serve the JSON API + this UI on :8787',
      cmd: 'granit web ~/Documents/Main'
    }
  ];

  import { toast } from '$lib/components/toast';

  let copied = $state<string | null>(null);

  async function copy(cmd: string, label: string) {
    try {
      await navigator.clipboard.writeText(cmd);
      copied = label;
      toast.success(`copied: ${label}`);
      setTimeout(() => {
        if (copied === label) copied = null;
      }, 1500);
    } catch {
      toast.warning('clipboard unavailable — long-press to select manually');
    }
  }
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4">
  <div class="flex items-baseline justify-between mb-3">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Install Granit</h2>
    <a href="https://github.com/artaeon/granit" rel="noreferrer" target="_blank" class="text-xs text-secondary hover:underline">github →</a>
  </div>
  <p class="text-xs text-dim mb-3 leading-relaxed">
    Set up Granit on a new laptop. Tap any block to copy.
  </p>
  <ul class="space-y-2">
    {#each cmds as c (c.label)}
      <li>
        <button
          onclick={() => copy(c.cmd, c.label)}
          class="w-full text-left bg-mantle border border-surface1 rounded p-2 hover:border-primary/40 transition-colors group"
        >
          <div class="flex items-baseline justify-between mb-1">
            <span class="text-xs text-text font-medium">{c.label}</span>
            <span class="text-[10px] text-dim group-hover:text-primary">
              {copied === c.label ? '✓ copied' : 'copy'}
            </span>
          </div>
          <div class="text-[11px] text-dim mb-1">{c.desc}</div>
          <pre class="text-xs text-secondary font-mono whitespace-pre-wrap break-all">{c.cmd}</pre>
        </button>
      </li>
    {/each}
  </ul>
  <p class="text-[11px] text-dim mt-3 leading-relaxed">
    Vault sync uses git — see <a href="https://github.com/artaeon/granit#sync" rel="noreferrer" target="_blank" class="text-secondary hover:underline">granit sync</a>.
  </p>
</section>
