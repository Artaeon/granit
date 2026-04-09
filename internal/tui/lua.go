package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// LuaEngine runs Lua scripts that can interact with the vault and editor.
// Scripts are loaded from <vault>/.granit/lua/ and ~/.config/granit/lua/.
type LuaEngine struct {
	vaultPath string
	scripts   []LuaScript
}

// LuaScript represents a single Lua script file.
type LuaScript struct {
	Name string // display name (filename without .lua)
	Path string // absolute path to the .lua file
}

// LuaResult holds the output of a Lua script execution.
type LuaResult struct {
	Message string // status/output message
	Content string // if non-empty, replace editor content
	Insert  string // if non-empty, insert at cursor
	Error   error
}

// NewLuaEngine creates a new Lua scripting engine.
func NewLuaEngine(vaultPath string) *LuaEngine {
	le := &LuaEngine{vaultPath: vaultPath}
	le.LoadScripts()
	return le
}

// LoadScripts discovers .lua files in the standard script directories.
func (le *LuaEngine) LoadScripts() {
	le.scripts = nil

	// Vault-local scripts: <vault>/.granit/lua/
	vaultDir := filepath.Join(le.vaultPath, ".granit", "lua")
	le.scripts = append(le.scripts, scanLuaDir(vaultDir)...)

	// Global scripts: ~/.config/granit/lua/
	if home, err := os.UserHomeDir(); err == nil {
		globalDir := filepath.Join(home, ".config", "granit", "lua")
		le.scripts = append(le.scripts, scanLuaDir(globalDir)...)
	}
}

// GetScripts returns all discovered Lua scripts.
func (le *LuaEngine) GetScripts() []LuaScript {
	return le.scripts
}

// RunScript executes a Lua script with the given context.
func (le *LuaEngine) RunScript(script LuaScript, notePath, noteContent string, noteMeta map[string]string) LuaResult {
	L := lua.NewState(lua.Options{SkipOpenLibs: false})
	defer L.Close()

	// Set a 5-second execution limit
	done := make(chan LuaResult, 1)
	go func() {
		result := le.executeScript(L, script, notePath, noteContent, noteMeta)
		done <- result
	}()

	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		L.Close()
		return LuaResult{Error: fmt.Errorf("script timed out after 5 seconds")}
	}
}

func (le *LuaEngine) executeScript(L *lua.LState, script LuaScript, notePath, noteContent string, noteMeta map[string]string) LuaResult {
	var result LuaResult

	// Register the 'granit' module with vault/note access functions
	granitMod := L.NewTable()

	// granit.note_path — current note path
	L.SetField(granitMod, "note_path", lua.LString(notePath))

	// granit.note_content — current note content
	L.SetField(granitMod, "note_content", lua.LString(noteContent))

	// granit.vault_path — vault root
	L.SetField(granitMod, "vault_path", lua.LString(le.vaultPath))

	// granit.note_name — basename without extension
	noteName := strings.TrimSuffix(filepath.Base(notePath), ".md")
	L.SetField(granitMod, "note_name", lua.LString(noteName))

	// granit.date() — current date
	L.SetField(granitMod, "date", L.NewFunction(func(L *lua.LState) int {
		format := L.OptString(1, "2006-01-02")
		L.Push(lua.LString(time.Now().Format(format)))
		return 1
	}))

	// granit.time() — current time
	L.SetField(granitMod, "time", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(time.Now().Format("15:04:05")))
		return 1
	}))

	// granit.read_note(name) — read another note's content
	L.SetField(granitMod, "read_note", L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		path := filepath.Join(le.vaultPath, name)
		// Validate path stays within vault (prevent path traversal)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(le.vaultPath)) {
			L.Push(lua.LNil)
			L.Push(lua.LString("path traversal not allowed"))
			return 2
		}
		data, err := os.ReadFile(path)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(data)))
		return 1
	}))

	// granit.list_notes() — list all .md files in vault
	L.SetField(granitMod, "list_notes", L.NewFunction(func(L *lua.LState) int {
		tbl := L.NewTable()
		_ = filepath.Walk(le.vaultPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".md") {
				rel, _ := filepath.Rel(le.vaultPath, path)
				tbl.Append(lua.LString(rel))
			}
			return nil
		})
		L.Push(tbl)
		return 1
	}))

	// granit.write_note(name, content) — write/overwrite a note
	L.SetField(granitMod, "write_note", L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		content := L.CheckString(2)
		if !strings.HasSuffix(name, ".md") {
			name += ".md"
		}
		path := filepath.Join(le.vaultPath, name)
		// Validate path stays within vault (prevent path traversal)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(le.vaultPath)) {
			L.Push(lua.LFalse)
			L.Push(lua.LString("path traversal not allowed"))
			return 2
		}
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			L.Push(lua.LFalse)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// granit.frontmatter — table of frontmatter key-value pairs
	if noteMeta != nil {
		metaTable := L.NewTable()
		for k, v := range noteMeta {
			L.SetField(metaTable, k, lua.LString(v))
		}
		L.SetField(granitMod, "frontmatter", metaTable)
	}

	// Output functions
	var messages []string

	// granit.msg(text) — add a status message
	L.SetField(granitMod, "msg", L.NewFunction(func(L *lua.LState) int {
		msg := L.CheckString(1)
		messages = append(messages, msg)
		return 0
	}))

	// granit.set_content(text) — replace editor content
	L.SetField(granitMod, "set_content", L.NewFunction(func(L *lua.LState) int {
		result.Content = L.CheckString(1)
		return 0
	}))

	// granit.insert(text) — insert text at cursor
	L.SetField(granitMod, "insert", L.NewFunction(func(L *lua.LState) int {
		result.Insert = L.CheckString(1)
		return 0
	}))

	// Register module
	L.SetGlobal("granit", granitMod)

	// Execute the script
	if err := L.DoFile(script.Path); err != nil {
		result.Error = err
		return result
	}

	if len(messages) > 0 {
		result.Message = strings.Join(messages, " | ")
	}

	return result
}

// scanLuaDir finds all .lua files in a directory.
func scanLuaDir(dir string) []LuaScript {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var scripts []LuaScript
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".lua") {
			name := strings.TrimSuffix(entry.Name(), ".lua")
			scripts = append(scripts, LuaScript{
				Name: name,
				Path: filepath.Join(dir, entry.Name()),
			})
		}
	}
	return scripts
}
