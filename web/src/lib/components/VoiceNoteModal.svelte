<!--
  Voice note recorder — capture audio + live transcription (Web Speech
  API) → editable transcript → save as a markdown note. Backend-free
  v1: no Whisper, no audio upload, just the rapid-capture path.

  Flow:
   1. User taps record. We start MediaRecorder (for the audio blob,
      held only in memory) AND SpeechRecognition (for the live
      transcript).
   2. While recording: timer ticks, transcript streams live, audio
      level dot pulses.
   3. Stop. Transcript becomes editable.
   4. User edits, hits "Create note". A new note is written under
      voice-notes/YYYY-MM-DD-HHmm.md with the transcript as the body
      and frontmatter { type: voice, captured_at }. Optional: tap
      "Ask AI" on the new note to extract tasks / summarise — that
      surface already exists in the note editor.

  Why Web Speech API and not server-side Whisper for v1: zero backend
  work, zero token cost, decent accuracy on a quiet mic. Works in
  Chrome / Edge; Safari has webkitSpeechRecognition which the
  detection below picks up. Firefox lacks it — we surface a friendly
  message and offer to record audio anyway (user can transcribe
  later, e.g. via a future Whisper backend).

  AI enrichment hook is intentionally NOT inline here — once the note
  is created the user lands in the editor where they have all the
  existing AI surfaces (Mod-Shift-A, Mod-Shift-/, link suggester). One
  surface, one mental model.
-->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';
  import {
    createSpeechRecognition,
    isSpeechRecognitionSupported,
    type SpeechRecognitionLike
  } from '$lib/util/speechRecognition';
  import { toast } from '$lib/components/toast';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  // Persist the in-flight transcript so a refresh / accidental close
  // doesn't lose what the user just spoke. Cleared on successful
  // save or explicit Discard. Stored as a single object rather than
  // separate keys so cleanup is one localStorage.removeItem.
  const TRANSCRIPT_KEY = 'granit.voiceNote.draft';
  interface VoiceDraft { transcript: string; capturedAt: string; }

  // ─── Web Speech detection ─────────────────────────────────────────
  // Shared wrapper from $lib/util/speechRecognition handles the
  // vendor-prefix dance + SSR-safe detection — same module the
  // dashboard QuickCapture and AIOverlay use.
  let recognitionSupported = $derived(isSpeechRecognitionSupported());

  // ─── Recording state ──────────────────────────────────────────────
  type Phase = 'idle' | 'requesting' | 'recording' | 'stopped' | 'error';
  let phase = $state<Phase>('idle');
  let elapsedMs = $state(0);
  let errorMsg = $state('');

  // Live transcript: finals are committed; interim is the in-progress
  // chunk shown faded so the user sees it appear as they speak.
  let finalText = $state('');
  let interimText = $state('');
  let editedTranscript = $state('');
  let saving = $state(false);

  let mediaRecorder: MediaRecorder | null = null;
  let recognition: SpeechRecognitionLike | null = null;
  let stream: MediaStream | null = null;
  let timerHandle: ReturnType<typeof setInterval> | null = null;
  let startedAt = 0;
  let audioBlob: Blob | null = null;
  let audioUrl = $state<string | null>(null);

  function fmtTime(ms: number) {
    const total = Math.floor(ms / 1000);
    const mm = Math.floor(total / 60).toString().padStart(2, '0');
    const ss = (total % 60).toString().padStart(2, '0');
    return `${mm}:${ss}`;
  }

  async function start() {
    if (phase === 'recording') return;
    errorMsg = '';
    finalText = '';
    interimText = '';
    editedTranscript = '';
    audioBlob = null;
    if (audioUrl) {
      URL.revokeObjectURL(audioUrl);
      audioUrl = null;
    }
    phase = 'requesting';
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    } catch (e) {
      phase = 'error';
      errorMsg = e instanceof Error ? e.message : 'Microphone access denied.';
      return;
    }

    // Pick a mime the browser actually supports. webm/opus is widely
    // supported on Chrome/Firefox; Safari needs mp4. Fallback to '' so
    // the browser picks its default if neither matches.
    const candidates = ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4', ''];
    const mime = candidates.find((c) => c === '' || (typeof MediaRecorder !== 'undefined' && MediaRecorder.isTypeSupported(c))) ?? '';
    const chunks: Blob[] = [];
    mediaRecorder = new MediaRecorder(stream, mime ? { mimeType: mime } : undefined);
    mediaRecorder.ondataavailable = (e) => {
      if (e.data.size > 0) chunks.push(e.data);
    };
    mediaRecorder.onstop = () => {
      audioBlob = new Blob(chunks, { type: mime || 'audio/webm' });
      audioUrl = URL.createObjectURL(audioBlob);
    };
    mediaRecorder.start(1000);

    const r = createSpeechRecognition();
    if (r) {
      recognition = r;
      r.continuous = true;
      r.interimResults = true;
      r.lang = navigator.language || 'en-US';
      r.onresult = (ev) => {
        let interim = '';
        let final = finalText;
        for (let i = ev.resultIndex; i < ev.results.length; i++) {
          const res = ev.results[i];
          if (!res || !res[0]) continue;
          const text = res[0].transcript;
          if (res.isFinal) final += (final && !final.endsWith(' ') ? ' ' : '') + text.trim();
          else interim += text;
        }
        finalText = final;
        interimText = interim;
      };
      r.onerror = (ev) => {
        // 'no-speech' / 'aborted' fire commonly during normal use; we
        // ignore them. Only surface unexpected ones.
        const err = ev.error ?? '';
        if (err && err !== 'no-speech' && err !== 'aborted') {
          errorMsg = `transcription: ${err}`;
        }
      };
      r.onend = () => {
        // If we're still recording when recognition ends (Chrome
        // sometimes auto-cycles after long silences), restart it.
        if (phase === 'recording' && recognition) {
          try { recognition.start(); } catch {}
        }
      };
      try {
        r.start();
      } catch {
        // Already-started errors are harmless.
      }
    }

    startedAt = Date.now();
    elapsedMs = 0;
    timerHandle = setInterval(() => {
      elapsedMs = Date.now() - startedAt;
    }, 250);
    phase = 'recording';
  }

  function stop() {
    if (phase !== 'recording') return;
    if (timerHandle) {
      clearInterval(timerHandle);
      timerHandle = null;
    }
    try { mediaRecorder?.stop(); } catch {}
    try { recognition?.stop(); } catch {}
    stream?.getTracks().forEach((t) => t.stop());
    stream = null;
    phase = 'stopped';
    // Seed the editable transcript with whatever we captured.
    editedTranscript = (finalText + (interimText ? ' ' + interimText : '')).trim();
  }

  function reset() {
    if (phase === 'recording') stop();
    finalText = '';
    interimText = '';
    editedTranscript = '';
    audioBlob = null;
    if (audioUrl) {
      URL.revokeObjectURL(audioUrl);
      audioUrl = null;
    }
    phase = 'idle';
    errorMsg = '';
    elapsedMs = 0;
  }

  function close() {
    if (phase === 'recording') stop();
    if (audioUrl) {
      URL.revokeObjectURL(audioUrl);
      audioUrl = null;
    }
    open = false;
  }

  function notePathForNow(): string {
    const d = new Date();
    const yy = d.getFullYear();
    const mm = String(d.getMonth() + 1).padStart(2, '0');
    const dd = String(d.getDate()).padStart(2, '0');
    const hh = String(d.getHours()).padStart(2, '0');
    const mi = String(d.getMinutes()).padStart(2, '0');
    return `voice-notes/${yy}-${mm}-${dd}-${hh}${mi}.md`;
  }

  async function saveAsNote() {
    if (saving) return;
    const text = editedTranscript.trim();
    if (!text) {
      toast.error('Transcript is empty.');
      return;
    }
    saving = true;
    const path = notePathForNow();
    const now = new Date().toISOString();
    const body = `# Voice note · ${new Date().toLocaleString()}\n\n${text}\n`;
    try {
      await api.createNote({
        path,
        frontmatter: {
          type: 'voice',
          captured_at: now,
          duration_ms: elapsedMs,
          tags: ['voice']
        },
        body
      });
      try { localStorage.removeItem(TRANSCRIPT_KEY); } catch {}
      toast.success('Voice note saved');
      open = false;
      void goto(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      toast.error(`save failed: ${msg}`);
    } finally {
      saving = false;
    }
  }

  // Cleanup on close — guarantee no zombie recorder if the user
  // closes the modal mid-record.
  $effect(() => {
    if (!open) {
      if (phase === 'recording') stop();
    }
  });

  // Auto-persist the editable transcript so a refresh or
  // accidentally-clicked outside doesn't kill the user's words. We
  // only persist after stop — during recording the live transcript
  // is volatile and we'd be writing every chunk.
  $effect(() => {
    if (phase !== 'stopped') return;
    const t = editedTranscript.trim();
    if (!t) {
      try { localStorage.removeItem(TRANSCRIPT_KEY); } catch {}
      return;
    }
    const draft: VoiceDraft = { transcript: editedTranscript, capturedAt: new Date().toISOString() };
    try { localStorage.setItem(TRANSCRIPT_KEY, JSON.stringify(draft)); } catch {}
  });

  // On open, restore an unsaved transcript if there is one. The user
  // gets dropped straight into the stopped/edit state — no audio,
  // since we don't persist the blob — and can save or discard.
  $effect(() => {
    if (!open) return;
    if (phase !== 'idle') return;
    try {
      const raw = localStorage.getItem(TRANSCRIPT_KEY);
      if (!raw) return;
      const d = JSON.parse(raw) as VoiceDraft;
      if (!d.transcript || !d.transcript.trim()) return;
      editedTranscript = d.transcript;
      phase = 'stopped';
      // Estimate a duration field if the persisted draft doesn't carry
      // one — purely cosmetic so the save metadata stays sane.
      elapsedMs = 0;
    } catch {}
  });
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 bg-base flex items-end sm:items-center justify-center p-0 sm:p-6"
    role="dialog"
    aria-modal="true"
    aria-label="Voice note recorder"
  >
    <div class="w-full sm:max-w-xl bg-mantle border border-surface1 rounded-t-xl sm:rounded-xl shadow-2xl flex flex-col max-h-[92dvh]">
      <header class="flex items-center gap-3 px-3 py-2 border-b border-surface1">
        <div class="w-8 h-8 rounded-full bg-surface1 text-primary flex items-center justify-center">
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="9" y="3" width="6" height="12" rx="3"/>
            <path d="M5 11a7 7 0 0014 0M12 18v3" stroke-linecap="round"/>
          </svg>
        </div>
        <div class="flex-1">
          <div class="text-sm font-semibold text-text">Voice note</div>
          <div class="text-[11px] text-dim">
            {#if phase === 'recording'}
              recording · {fmtTime(elapsedMs)}
            {:else if phase === 'stopped'}
              captured · {fmtTime(elapsedMs)}
            {:else}
              tap record to start
            {/if}
          </div>
        </div>
        <button
          onclick={close}
          aria-label="Close"
          class="w-8 h-8 flex items-center justify-center text-subtext hover:text-text hover:bg-surface0 rounded"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M6 6l12 12M6 18L18 6" stroke-linecap="round"/>
          </svg>
        </button>
      </header>

      <div class="flex-1 min-h-0 overflow-y-auto p-4 space-y-3">
        {#if errorMsg}
          <div class="text-xs text-error bg-surface0 border border-error rounded px-2 py-1.5">{errorMsg}</div>
        {/if}
        {#if !recognitionSupported && phase === 'idle'}
          <div class="text-xs text-warning bg-surface0 border border-warning rounded px-2 py-1.5">
            Live transcription not supported in this browser. You can still record audio and transcribe later.
          </div>
        {/if}

        {#if phase === 'idle' || phase === 'error'}
          <div class="flex items-center justify-center py-6">
            <button
              onclick={start}
              class="w-20 h-20 rounded-full bg-error text-white shadow-lg hover:scale-105 transition-transform flex items-center justify-center"
              aria-label="Start recording"
              title="Start recording"
            >
              <svg viewBox="0 0 24 24" class="w-8 h-8" fill="currentColor">
                <circle cx="12" cy="12" r="6"/>
              </svg>
            </button>
          </div>
        {:else if phase === 'requesting'}
          <div class="text-sm text-dim text-center py-6">Requesting microphone…</div>
        {:else if phase === 'recording'}
          <div class="flex items-center justify-center py-6 gap-4">
            <button
              onclick={stop}
              class="w-20 h-20 rounded-full bg-error text-white shadow-lg flex items-center justify-center recording-pulse"
              aria-label="Stop recording"
              title="Stop recording"
            >
              <svg viewBox="0 0 24 24" class="w-8 h-8" fill="currentColor">
                <rect x="7" y="7" width="10" height="10" rx="1"/>
              </svg>
            </button>
          </div>
          <div class="rounded border border-surface1 bg-surface0 p-3 min-h-[6rem] text-sm leading-relaxed">
            {#if finalText || interimText}
              <span class="text-text">{finalText}</span>
              {#if interimText}
                <span class="text-dim italic"> {interimText}</span>
              {/if}
            {:else}
              <span class="text-dim italic">Speak — your words will appear here as you talk.</span>
            {/if}
          </div>
        {:else if phase === 'stopped'}
          {#if audioUrl}
            <div>
              <div class="text-[10px] uppercase tracking-wider text-dim mb-1">Audio</div>
              <audio src={audioUrl} controls class="w-full"></audio>
            </div>
          {/if}
          <div>
            <div class="text-[10px] uppercase tracking-wider text-dim mb-1">Transcript</div>
            <textarea
              bind:value={editedTranscript}
              rows="6"
              class="w-full px-3 py-2 rounded border border-surface1 bg-surface0 text-text text-sm resize-y"
              placeholder="Transcript will appear here. Edit before saving if needed."
            ></textarea>
            <div class="text-[10px] text-dim mt-1">
              {editedTranscript.trim().split(/\s+/).filter(Boolean).length} words
            </div>
          </div>
        {/if}
      </div>

      {#if phase === 'stopped'}
        <footer class="flex items-center gap-2 px-4 py-3 border-t border-surface1">
          <button
            onclick={reset}
            class="px-3 py-1.5 rounded text-sm text-subtext hover:text-text hover:bg-surface0"
          >
            Record again
          </button>
          <span class="flex-1"></span>
          <button
            onclick={() => { try { localStorage.removeItem(TRANSCRIPT_KEY); } catch {} close(); }}
            class="px-3 py-1.5 rounded text-sm text-subtext hover:text-text hover:bg-surface0"
            title="Discard the transcript and close"
          >
            Discard
          </button>
          <button
            onclick={saveAsNote}
            disabled={saving || editedTranscript.trim().length === 0}
            class="px-4 py-1.5 rounded text-sm font-medium bg-primary text-on-primary disabled:opacity-50"
          >
            {saving ? 'Saving…' : 'Save as note'}
          </button>
        </footer>
      {/if}
    </div>
  </div>
{/if}

<style>
  .recording-pulse {
    animation: rec-pulse 1.4s ease-in-out infinite;
  }
  @keyframes rec-pulse {
    0%, 100% {
      box-shadow: 0 0 0 0 rgba(220, 38, 38, 0.55);
    }
    70% {
      box-shadow: 0 0 0 18px rgba(220, 38, 38, 0);
    }
  }
</style>
