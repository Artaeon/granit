package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/tasks"
)

// TriageQueue is the inbox-zero overlay. Steps through every
// task in TriageInbox state one at a time and lets the user
// promote each with a single keystroke. The whole point of the
// planning loop is to make this fast — no menus, no confirmation
// modals, no mouse.
//
// Keys (chosen for left-hand reach so the right hand stays on
// the trackpoint/mouse for nothing — power user, keyboard-first):
//
//   t / enter   →  triaged (decided to do, no date yet)
//   s           →  scheduled (TriageScheduled — pick a date later
//                  in the calendar; sets ScheduledStart to today
//                  for now so it surfaces in today.tasks)
//   d           →  dropped (not doing this)
//   z           →  snoozed (pushes off for 7 days by default)
//   space       →  skip (don't change state, move to next)
//   o           →  open the source note (closes triage first)
//   q / esc     →  close the queue
//   k / ↑       →  back one task (in case of fat-finger)
//   j / ↓       →  same as space
type TriageQueue struct {
	OverlayBase
	store    *tasks.TaskStore
	inbox    []tasks.Task // snapshot taken on Open; refreshed after each action
	cursor   int
	openReq  string // path to open after close, set by 'o'
	openOK   bool
	closeReq bool
	statusMsg string

	// picker is the inline date chooser activated by `s` and
	// `z`. When active it consumes keys exclusively until the
	// user picks an option or Esc-cancels. pendingPick remembers
	// which action triggered it so we know what to do with the
	// chosen duration.
	picker      quickPicker
	pendingPick string // "schedule" or "snooze"
}

// NewTriageQueue returns an empty queue. The store is injected
// at construction; Open snapshots the current inbox.
func NewTriageQueue(store *tasks.TaskStore) TriageQueue {
	return TriageQueue{store: store}
}

// Open snapshots every task currently in TriageInbox state and
// activates the overlay. Cursor lands on the first task.
func (q *TriageQueue) Open() {
	q.Activate()
	q.cursor = 0
	q.openOK = false
	q.openReq = ""
	q.closeReq = false
	q.statusMsg = ""
	q.inbox = q.snapshotInbox()
}

// snapshotInbox grabs every TriageInbox-state task. Sorted by
// CreatedAt asc (oldest first) so the user is asked about the
// stuff that's been waiting the longest.
func (q *TriageQueue) snapshotInbox() []tasks.Task {
	if q.store == nil {
		return nil
	}
	tasks := q.store.Filter(func(t tasks.Task) bool {
		return t.Triage == "" || t.Triage == "inbox"
	})
	// store.All returns ULID-sorted (= creation-sorted) so
	// Filter returns in the same order — no extra sort needed.
	return tasks
}

// Update handles keys.
func (q *TriageQueue) Update(msg tea.Msg) (TriageQueue, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return *q, nil
	}
	if len(q.inbox) == 0 {
		// Empty inbox — only Esc/q close.
		switch keyMsg.String() {
		case "esc", "q", "ctrl+c":
			q.Close()
		}
		return *q, nil
	}
	current := q.inbox[q.cursor]

	// Picker takes priority — when up, all keys go to it.
	if q.picker.IsActive() {
		if q.picker.Update(msg) {
			if opt, ok := q.picker.Result(); ok {
				switch q.pendingPick {
				case "schedule":
					q.applySchedule(current.ID, opt.Value)
				case "snooze":
					q.applySnooze(current.ID, opt.Value)
				}
				q.pendingPick = ""
				q.advance()
			}
		}
		return *q, nil
	}

	switch keyMsg.String() {
	case "esc", "q", "ctrl+c":
		q.Close()
	case "t", "enter":
		q.applyTriage(current.ID, "triaged")
		q.advance()
	case "s":
		q.pendingPick = "schedule"
		q.picker.Open("Schedule when?", scheduleOptions())
	case "d":
		q.applyTriage(current.ID, "dropped")
		q.advance()
	case "z":
		q.pendingPick = "snooze"
		q.picker.Open("Snooze until?", snoozeOptions())
	case " ", "space", "j", "down":
		q.advance()
	case "k", "up":
		if q.cursor > 0 {
			q.cursor--
		}
	case "o":
		q.openReq = current.NotePath
		q.openOK = true
		q.Close()
	}
	return *q, nil
}

// applyTriage updates the task's triage state via the store.
// Errors land in the status field — never block the loop, the
// user wants to keep moving. Sets a positive feedback message
// on success too so each action gives a confident echo.
func (q *TriageQueue) applyTriage(id, state string) {
	if q.store == nil {
		return
	}
	if err := q.store.Triage(id, tasks.TriageState(state)); err != nil {
		q.statusMsg = "triage failed: " + err.Error()
		return
	}
	q.statusMsg = "→ " + state
}

// applySchedule marks the task as scheduled and sets
// ScheduledStart to now+offset. Offset 0 means "today" — the
// task appears immediately in today.tasks. Future offsets put
// it on that day's calendar (and today.tasks reads
// ScheduledStart, so the task shows up on its scheduled day
// without needing a separate trigger).
func (q *TriageQueue) applySchedule(id string, offset time.Duration) {
	if q.store == nil {
		return
	}
	when := time.Now().Add(offset)
	if err := q.store.Schedule(id, when, 30*time.Minute); err != nil {
		q.statusMsg = "schedule failed: " + err.Error()
		return
	}
	if err := q.store.Triage(id, tasks.TriageScheduled); err != nil {
		q.statusMsg = "triage failed: " + err.Error()
		return
	}
	q.statusMsg = "→ scheduled " + scheduleEcho(offset)
}

// scheduleEcho formats the offset for the post-action status
// echo: "today" / "tomorrow" / "+3d" depending on offset.
func scheduleEcho(offset time.Duration) string {
	day := 24 * time.Hour
	switch {
	case offset == 0:
		return "today"
	case offset == day:
		return "tomorrow"
	default:
		return humanDuration(offset)
	}
}

// applySnooze advances the task's hidden-until time by dur and
// marks it Snoozed. The widget logic that filters "today's
// tasks" should respect this state in a follow-up; for now the
// state is just persisted.
func (q *TriageQueue) applySnooze(id string, dur time.Duration) {
	if q.store == nil {
		return
	}
	until := time.Now().Add(dur)
	if err := q.store.UpdateMeta(id, func(t *tasks.Task) {
		t.Triage = tasks.TriageSnoozed
		t.ScheduledStart = &until
	}); err != nil {
		q.statusMsg = "snooze failed: " + err.Error()
		return
	}
	q.statusMsg = "→ snoozed " + humanDuration(dur)
}

// humanDuration formats a duration for triage feedback. Power
// users want "+7d" not "168h0m0s" or "1 week."
func humanDuration(d time.Duration) string {
	hrs := int(d.Hours())
	switch {
	case hrs >= 24*7:
		return fmt.Sprintf("+%dw", hrs/(24*7))
	case hrs >= 24:
		return fmt.Sprintf("+%dd", hrs/24)
	case hrs >= 1:
		return fmt.Sprintf("+%dh", hrs)
	default:
		return fmt.Sprintf("+%dm", int(d.Minutes()))
	}
}

// advance moves to the next task. When we run off the end the
// queue auto-closes (inbox-zero achieved).
func (q *TriageQueue) advance() {
	q.cursor++
	if q.cursor >= len(q.inbox) {
		q.statusMsg = "Inbox zero — done."
		q.closeReq = true
	}
}

// PendingOpen returns the note path the user asked to open
// (consumed-once). Caller invokes m.loadNote after the overlay
// closes.
func (q *TriageQueue) PendingOpen() (string, bool) {
	if !q.openOK {
		return "", false
	}
	p := q.openReq
	q.openOK = false
	q.openReq = ""
	return p, true
}

// AutoClose reports whether the queue finished naturally
// (cursor walked past the last task). Caller closes the overlay
// AND can show a status confirmation.
func (q *TriageQueue) AutoClose() bool {
	if q.closeReq {
		q.closeReq = false
		q.Close()
		return true
	}
	return false
}

// View renders the focused task + key legend.
func (q *TriageQueue) View() string {
	if len(q.inbox) == 0 {
		return q.renderEmpty()
	}
	current := q.inbox[q.cursor]

	header := lipgloss.NewStyle().Bold(true).Render("Triage Inbox")
	progress := lipgloss.NewStyle().Faint(true).Render(
		fmt.Sprintf(" ▸ %d / %d", q.cursor+1, len(q.inbox)))

	body := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(2, 4).
		Width(70).
		Render(triageTaskBlock(current))

	keys := lipgloss.NewStyle().Faint(true).Render(
		"t/enter triage  ·  s schedule  ·  d drop  ·  z snooze  ·\n" +
			"space skip  ·  k back  ·  o open  ·  q close")

	out := header + progress + "\n\n" + body + "\n\n" + keys
	// Picker takes the bottom slot when up — replaces the
	// keymap hint area. Power user already knows the keys; the
	// picker shows the new menu.
	if q.picker.IsActive() {
		out = header + progress + "\n\n" + body + "\n\n" + q.picker.View()
	} else if q.statusMsg != "" {
		// Status echoes use a calmer color than the error red.
		out += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(q.statusMsg)
	}
	return out
}

func (q *TriageQueue) renderEmpty() string {
	header := lipgloss.NewStyle().Bold(true).Render("Triage Inbox")
	body := lipgloss.NewStyle().
		Padding(2, 4).
		Foreground(lipgloss.Color("10")).
		Render("Inbox zero. Nothing to triage.")
	hint := lipgloss.NewStyle().Faint(true).Render("q / esc to close")
	return header + "\n\n" + body + "\n\n" + hint
}

// triageTaskBlock renders the task body — text, source, tags,
// any due date. Vertically dense so the user reads it fast.
func triageTaskBlock(t tasks.Task) string {
	text := lipgloss.NewStyle().Bold(true).Render(t.Text)
	src := lipgloss.NewStyle().Faint(true).Render("from " + t.NotePath + ":" + fmt.Sprintf("%d", t.LineNum))
	out := text + "\n" + src

	var meta []string
	if t.DueDate != "" {
		meta = append(meta, "📅 "+t.DueDate)
	}
	if len(t.Tags) > 0 {
		meta = append(meta, "#"+strings.Join(t.Tags, " #"))
	}
	if t.Priority > 0 {
		meta = append(meta, fmt.Sprintf("priority %d", t.Priority))
	}
	if len(meta) > 0 {
		out += "\n\n" + lipgloss.NewStyle().Faint(true).Render(strings.Join(meta, "  ·  "))
	}
	return out
}
