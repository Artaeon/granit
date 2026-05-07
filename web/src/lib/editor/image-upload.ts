// Paste-and-drop image upload for the editor. Handles two flows:
//
//   • Paste — user takes a screenshot or copies an image and pastes
//     into the editor. The clipboard event carries a File entry; we
//     POST it to /api/v1/upload, get back a vault-relative path, and
//     splice `![[<path>]]` (or markdown image syntax) at the cursor.
//
//   • Drop — user drags one or more files from the OS file manager
//     onto the editor. Same upload path; multiple files insert as
//     consecutive image lines.
//
// Behaviour:
//   • Only image/* + audio/* + video/* + application/pdf are
//     uploaded. Everything else falls through to the default text
//     paste so the user can still paste plain text reliably.
//   • A placeholder `![uploading…]` is inserted optimistically and
//     replaced once the server returns the real path. If the upload
//     fails, the placeholder is replaced with `[upload failed]` so
//     the user notices.
//   • The upload uses the same auth token as other API calls (Bearer
//     in localStorage).

import { EditorView } from '@codemirror/view';
import { EditorSelection, type ChangeSpec } from '@codemirror/state';

const TOKEN_KEY = 'everything.token';

interface UploadResult {
  path: string;
  contentType: string;
}

async function uploadFile(file: File): Promise<UploadResult> {
  const fd = new FormData();
  fd.append('file', file, file.name || 'upload');
  const headers: Record<string, string> = {};
  try {
    const tok = localStorage.getItem(TOKEN_KEY);
    if (tok) headers['Authorization'] = `Bearer ${tok}`;
  } catch {}
  const res = await fetch('/api/v1/upload', {
    method: 'POST',
    headers,
    body: fd
  });
  if (!res.ok) {
    let msg = res.statusText;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {}
    throw new Error(`upload failed: ${msg}`);
  }
  return (await res.json()) as UploadResult;
}

// Render markdown for a successful upload. Images use the wikilink
// embed syntax (`![[…]]`) so granit's MarkdownRenderer can lazy-
// resolve them via /api/v1/files/. Non-images use a plain link
// because not every renderer can embed e.g. PDFs inline.
function renderMarkdown(path: string, contentType: string, displayName: string): string {
  if (contentType.startsWith('image/')) {
    return `![[${path}]]`;
  }
  // Strip directory + hash prefix from displayed name for readability.
  const fileName = path.split('/').pop() || displayName;
  const human = fileName.replace(/^[0-9a-f]{8}-/, '');
  return `[${human}](/api/v1/files/${path})`;
}

// Replace a placeholder span with its final content. The placeholder
// is unique per upload (we tag it with a random nonce) so two
// concurrent uploads can't step on each other.
function replacePlaceholder(view: EditorView, placeholder: string, replacement: string) {
  const doc = view.state.doc.toString();
  const idx = doc.indexOf(placeholder);
  if (idx < 0) return;
  view.dispatch({
    changes: { from: idx, to: idx + placeholder.length, insert: replacement }
  });
}

// Process a list of files: insert one placeholder per file at the
// current cursor (joined by newlines), then upload in parallel and
// swap each placeholder with the resulting markdown.
async function handleFiles(view: EditorView, files: File[]) {
  if (files.length === 0) return;
  // Filter to allowed types client-side too — we don't want to
  // blast random binary uploads at the server.
  const allowed = files.filter((f) =>
    f.type.startsWith('image/') ||
    f.type.startsWith('audio/') ||
    f.type.startsWith('video/') ||
    f.type === 'application/pdf'
  );
  if (allowed.length === 0) return;

  // Build placeholders. Each placeholder gets a unique nonce so
  // search-and-replace later doesn't collide if two uploads happen
  // close together. Inserting all placeholders in one dispatch keeps
  // the document a single edit step (one undo to remove all).
  const placeholders = allowed.map((_, i) =>
    `![uploading-${Math.random().toString(36).slice(2, 10)}-${i}]`
  );
  const sel = view.state.selection.main;
  const insertText = placeholders.join('\n');
  view.dispatch({
    changes: { from: sel.from, to: sel.to, insert: insertText } as ChangeSpec,
    selection: EditorSelection.cursor(sel.from + insertText.length)
  });

  // Upload in parallel; settle independently so a slow file doesn't
  // hold up the others.
  await Promise.all(allowed.map(async (file, i) => {
    try {
      const res = await uploadFile(file);
      replacePlaceholder(view, placeholders[i],
        renderMarkdown(res.path, res.contentType, file.name));
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      replacePlaceholder(view, placeholders[i], `[upload failed: ${msg}]`);
    }
  }));
}

export const imagePasteAndDrop = EditorView.domEventHandlers({
  paste(event, view) {
    if (view.state.readOnly) return false;
    const items = event.clipboardData?.items;
    if (!items) return false;
    const files: File[] = [];
    for (let i = 0; i < items.length; i++) {
      const it = items[i];
      if (it.kind === 'file') {
        const f = it.getAsFile();
        if (f) files.push(f);
      }
    }
    if (files.length === 0) return false;
    event.preventDefault();
    void handleFiles(view, files);
    return true;
  },
  drop(event, view) {
    if (view.state.readOnly) return false;
    const dt = event.dataTransfer;
    if (!dt || dt.files.length === 0) return false;
    // Move the cursor to the drop location so the inserted markdown
    // lands where the user dropped — otherwise we'd splice at
    // wherever the editor's selection happened to be, which feels
    // disconnected from a drop gesture.
    const pos = view.posAtCoords({ x: event.clientX, y: event.clientY });
    if (pos !== null) {
      view.dispatch({ selection: EditorSelection.cursor(pos) });
    }
    event.preventDefault();
    const files = Array.from(dt.files);
    void handleFiles(view, files);
    return true;
  }
});
