package tui

import (
	"os"
	"path/filepath"
	"strings"
)

// Tasks.md is the canonical, vault-root markdown file that holds the user's
// task list. Multiple subsystems mutate it (TaskManager, GoalsMode, IdeasBoard,
// RecurringTasks, the Plan-My-Day scheduler), so all access goes through the
// helpers in this file. Centralising the path and the writer means:
//
//   - The path string "Tasks.md" lives in exactly one place; renaming the
//     vault-side file is a single edit.
//   - Every writer uses atomicWriteNote, so a crash mid-save can never leave
//     a truncated tasks file behind.
//   - "Append a new task line" has a single implementation that handles the
//     missing-file case (seeds with a "# Tasks" header) consistently.

// tasksFilePath returns the canonical path to Tasks.md inside vaultRoot.
// Returns an empty string if vaultRoot is empty so callers can short-circuit
// without producing nonsense paths like "/Tasks.md".
func tasksFilePath(vaultRoot string) string {
	if vaultRoot == "" {
		return ""
	}
	return filepath.Join(vaultRoot, "Tasks.md")
}

// readTasksFile loads Tasks.md and returns its raw bytes. A missing file is
// returned as (nil, nil) so callers can treat it the same as "no tasks yet".
func readTasksFile(vaultRoot string) ([]byte, error) {
	path := tasksFilePath(vaultRoot)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// writeTasksFile atomically replaces Tasks.md with the given bytes. Use this
// for full-rewrite operations (archive, schedule annotation) where the caller
// has already produced the new file contents.
func writeTasksFile(vaultRoot string, data []byte) error {
	path := tasksFilePath(vaultRoot)
	if path == "" {
		return nil
	}
	return atomicWriteNote(path, string(data))
}

// appendTaskLine adds a single task line to Tasks.md. The taskLine should be
// the bare line content (no surrounding newlines) — appendTaskLine ensures the
// file ends in a newline before the new line is appended, so callers don't
// have to think about trailing-newline normalisation.
//
// If Tasks.md does not exist yet it is created with a "# Tasks" header so the
// first task drops into a sensibly structured file.
func appendTaskLine(vaultRoot, taskLine string) error {
	if vaultRoot == "" || taskLine == "" {
		return nil
	}
	existing, err := readTasksFile(vaultRoot)
	if err != nil {
		return err
	}
	var buf strings.Builder
	if len(existing) == 0 {
		buf.WriteString("# Tasks\n\n")
	} else {
		buf.Write(existing)
		// Guarantee a single newline before the new line so we don't end up
		// concatenating onto the previous task ("- [ ] foo- [ ] bar").
		if !strings.HasSuffix(string(existing), "\n") {
			buf.WriteByte('\n')
		}
	}
	buf.WriteString(strings.TrimRight(taskLine, "\n"))
	buf.WriteByte('\n')
	return writeTasksFile(vaultRoot, []byte(buf.String()))
}
