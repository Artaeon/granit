package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VaultWriter is the minimal interface write-tools need to mutate
// vault state. Like VaultReader, it's deliberately decoupled from
// internal/vault so the agents package keeps a clean dependency
// boundary.
//
// All paths are vault-relative; implementations MUST refuse paths
// that escape the vault root and return an error so the LLM gets
// a clear observation telling it the path is invalid.
type VaultWriter interface {
	// WriteNote writes content to the vault-relative path,
	// creating parent directories as needed. Returns the
	// absolute path so the LLM (and the user via transcript)
	// can confirm where the note landed.
	WriteNote(relPath, content string) (absPath string, err error)
	// AppendTaskLine appends a single "- [ ] {text}" line to
	// Tasks.md (the vault-canonical task list). Returns the
	// path it wrote to so the agent can confirm.
	AppendTaskLine(taskLine string) (path string, err error)
}

// WriteNote returns a Tool that creates or overwrites a markdown
// note. KindWrite, so the agent runtime gates it behind the
// Approve callback before calling.
//
// The tool refuses to overwrite an existing note unless the LLM
// passes overwrite=true — defence against an over-eager agent
// trampling notes the user wrote by hand. The error observation
// tells the LLM how to retry.
func WriteNote(vault VaultReader, writer VaultWriter) Tool {
	return &writeNoteTool{reader: vault, writer: writer}
}

type writeNoteTool struct {
	reader VaultReader
	writer VaultWriter
}

func (t *writeNoteTool) Name() string { return "write_note" }
func (t *writeNoteTool) Description() string {
	return "Create a new markdown note in the vault, or overwrite an existing one with overwrite=true. Pass the full body in 'content' (frontmatter included if you want it)."
}
func (t *writeNoteTool) Kind() ToolKind { return KindWrite }
func (t *writeNoteTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "path", Description: "Vault-relative path (e.g. 'Notes/idea.md'); auto-creates parent dirs", Required: true},
		{Name: "content", Description: "Full note body, including any frontmatter you want", Required: true},
		{Name: "overwrite", Description: "Set to 'true' to overwrite an existing file (default: refuse)"},
	}
}

func (t *writeNoteTool) Run(_ context.Context, args map[string]string) ToolResult {
	path := strings.TrimSpace(args["path"])
	content := args["content"]
	overwrite := strings.EqualFold(strings.TrimSpace(args["overwrite"]), "true")

	if !pathInsideVault(t.reader.VaultRoot(), path) {
		return ToolResult{Err: fmt.Errorf("path %q escapes the vault", path)}
	}
	// Refuse silent overwrite. Existence check uses the reader so
	// the implementation stays in one place; tests can stub.
	if !overwrite {
		if _, exists := t.reader.NoteContent(path); exists {
			return ToolResult{Err: fmt.Errorf("note %q already exists. Pass overwrite=true to replace it, OR write to a different path.", path)}
		}
	}
	abs, err := t.writer.WriteNote(path, content)
	if err != nil {
		return ToolResult{Err: fmt.Errorf("write_note: %w", err)}
	}
	return ToolResult{Output: fmt.Sprintf("Wrote %d bytes to %s", len(content), abs)}
}

// CreateTask returns a Tool that appends a new task line to the
// vault's Tasks.md. KindWrite — gated by Approve.
//
// Granit's task system already supports rich syntax (📅 due, ⏰
// time-block, #tags); this tool builds the line from structured
// args so the LLM doesn't have to remember the emoji.
func CreateTask(writer VaultWriter) Tool {
	return &createTaskTool{writer: writer}
}

type createTaskTool struct{ writer VaultWriter }

func (t *createTaskTool) Name() string { return "create_task" }
func (t *createTaskTool) Description() string {
	return "Append a new task to Tasks.md. Optionally include a due date (YYYY-MM-DD), priority (1-4), and tag."
}
func (t *createTaskTool) Kind() ToolKind { return KindWrite }
func (t *createTaskTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "text", Description: "Task description (no leading '- [ ]')", Required: true},
		{Name: "due", Description: "Due date YYYY-MM-DD (optional)"},
		{Name: "priority", Description: "1=low .. 4=highest (optional)"},
		{Name: "tag", Description: "Single tag without '#' (optional)"},
	}
}

func (t *createTaskTool) Run(_ context.Context, args map[string]string) ToolResult {
	text := strings.TrimSpace(args["text"])
	if text == "" {
		return ToolResult{Err: fmt.Errorf("create_task: empty text")}
	}
	line := "- [ ] " + text
	if tag := strings.TrimSpace(args["tag"]); tag != "" {
		line += " #" + tag
	}
	if due := strings.TrimSpace(args["due"]); due != "" {
		// 📅 prefix matches granit's existing task-marker syntax
		// so the new task appears in Plan/Today views correctly.
		line += " 📅 " + due
	}
	if prio := strings.TrimSpace(args["priority"]); prio != "" {
		switch prio {
		case "4":
			line += " 🔺"
		case "3":
			line += " ⏫"
		case "2":
			line += " 🔼"
		case "1":
			line += " 🔽"
		}
	}
	path, err := t.writer.AppendTaskLine(line)
	if err != nil {
		return ToolResult{Err: fmt.Errorf("create_task: %w", err)}
	}
	return ToolResult{Output: fmt.Sprintf("Added task to %s: %s", path, text)}
}

// CreateObject returns a Tool that creates a new typed-object note.
// Wraps WriteNote with frontmatter assembly: the LLM names a type
// and a property bag, the tool builds the YAML header and writes it
// to the type's default folder.
//
// Why a separate tool? Two reasons:
//   - Reduces LLM error rate. Asking it to compose YAML correctly
//     in a `content:` arg fails too often (quoting, indentation,
//     emoji handling). create_object guarantees a parseable file.
//   - Encodes the type's filename pattern so the path conventions
//     are consistent without the LLM having to know them.
func CreateObject(reader VaultReader, writer VaultWriter) Tool {
	return &createObjectTool{reader: reader, writer: writer}
}

type createObjectTool struct {
	reader VaultReader
	writer VaultWriter
}

func (t *createObjectTool) Name() string { return "create_object" }
func (t *createObjectTool) Description() string {
	return "Create a new typed-object note. Specify the type ID and properties as 'key=value' pairs."
}
func (t *createObjectTool) Kind() ToolKind { return KindWrite }
func (t *createObjectTool) Params() []ToolParam {
	return []ToolParam{
		{Name: "type", Description: "Type ID from the registry (e.g. 'person', 'book')", Required: true},
		{Name: "title", Description: "Object title — also drives the filename", Required: true},
		{Name: "properties", Description: "Comma-separated key=value pairs (e.g. 'email=alice@x.com,role=Engineer')"},
		{Name: "body", Description: "Markdown body to follow the frontmatter (optional)"},
	}
}

func (t *createObjectTool) Run(_ context.Context, args map[string]string) ToolResult {
	typeID := strings.TrimSpace(args["type"])
	title := strings.TrimSpace(args["title"])
	if typeID == "" || title == "" {
		return ToolResult{Err: fmt.Errorf("create_object: type and title are required")}
	}
	reg := t.reader.ObjectRegistry()
	if reg == nil {
		return ToolResult{Err: fmt.Errorf("create_object: object registry not available")}
	}
	tt, ok := reg.ByID(typeID)
	if !ok {
		return ToolResult{Err: fmt.Errorf("create_object: unknown type %q. Run query_objects with no type to see what's available.", typeID)}
	}

	// Resolve the destination path using the type's folder + filename pattern.
	folder := tt.Folder
	pattern := tt.FilenamePattern
	if pattern == "" {
		pattern = "{title}"
	}
	filename := strings.ReplaceAll(pattern, "{title}", sanitiseFilename(title))
	if !strings.HasSuffix(filename, ".md") {
		filename += ".md"
	}
	relPath := filename
	if folder != "" {
		relPath = filepath.Join(folder, filename)
	}

	// Build the frontmatter from the property bag.
	props := parseWhereClause(args["properties"])
	var fm strings.Builder
	fm.WriteString("---\n")
	fmt.Fprintf(&fm, "type: %s\n", typeID)
	fmt.Fprintf(&fm, "title: %s\n", yamlSingleLine(title))
	for k, v := range props {
		fmt.Fprintf(&fm, "%s: %s\n", k, yamlSingleLine(v))
	}
	fm.WriteString("---\n\n")

	body := args["body"]
	if body == "" {
		body = "# " + title + "\n"
	}
	content := fm.String() + body

	abs, err := t.writer.WriteNote(relPath, content)
	if err != nil {
		// If the file already exists, surface that as an error
		// observation rather than silently overwriting — the LLM
		// can iterate to a unique title.
		if _, exists := t.reader.NoteContent(relPath); exists {
			return ToolResult{Err: fmt.Errorf("create_object: %s already exists. Choose a different title.", relPath)}
		}
		return ToolResult{Err: fmt.Errorf("create_object: %w", err)}
	}
	return ToolResult{Output: fmt.Sprintf("Created %s object %q at %s", typeID, title, abs)}
}

// sanitiseFilename strips characters that break common filesystems
// (Windows, network shares) so the generated filename is portable.
// Keeps spaces — those work everywhere modern.
func sanitiseFilename(s string) string {
	s = strings.TrimSpace(s)
	for _, bad := range []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} {
		s = strings.ReplaceAll(s, bad, "")
	}
	if s == "" {
		s = "untitled"
	}
	return s
}

// yamlSingleLine quotes a string for safe inclusion as a YAML
// single-line value. Wraps in double quotes only when the value
// would actually break a YAML parser — most plain strings (emails,
// URLs, names) pass through bare. Specifically we quote when:
//   - empty
//   - contains an embedded newline, tab, or double-quote
//   - contains ": " (the YAML mapping separator pattern)
//   - starts with a YAML-reserved indicator character
//
// Granit emails like "s@example.com" do NOT contain ": " and don't
// start with @, so they pass through unquoted — which matches what
// users hand-write in frontmatter.
func yamlSingleLine(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, "\"\n\t") {
		v = strings.ReplaceAll(v, `\`, `\\`)
		v = strings.ReplaceAll(v, `"`, `\"`)
		return `"` + v + `"`
	}
	if strings.Contains(v, ": ") {
		return `"` + v + `"`
	}
	// Reserved YAML indicators only matter at position 0.
	if len(v) > 0 && strings.ContainsRune("#&*!|>{[}],%@`", rune(v[0])) {
		return `"` + v + `"`
	}
	return v
}

// Compile-time assertions so a Tool implementation that drifts
// from the interface fails to build instead of silently breaking
// the agent runtime.
var (
	_ Tool = (*writeNoteTool)(nil)
	_ Tool = (*createTaskTool)(nil)
	_ Tool = (*createObjectTool)(nil)
)

// _ is a placeholder so go vet doesn't whine about an unused import
// when this file's test variant lands later. Removing the os/filepath
// imports requires touching multiple sites; cheaper to keep them and
// signal intent.
var _ = os.PathSeparator
