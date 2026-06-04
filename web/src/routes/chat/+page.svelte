<!--
  Thin route shell. The actual chat surface — chat session manager
  wiring, message list, composer, mode picker — lives in
  $lib/chat/ChatPane.svelte so the workspace shell (Phase 2 of the
  granit vision) can drop the same chat as a pane in any slot. The
  ChatPane is the "AI as a pane type" piece of the vision: a user
  can park it next to TasksPane, NotesListPane, etc. and use the AI
  conversation as a contextual surface alongside their work.
-->
<script lang="ts">
  import ChatPane from '$lib/chat/ChatPane.svelte';
</script>

<ChatPane />
