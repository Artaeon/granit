package widgets

import (
	"errors"
	"fmt"
	"sync"
)

// ErrUnknownWidget is returned by Get when no widget is registered
// for the given ID.
var ErrUnknownWidget = errors.New("widgets: unknown widget ID")

// Registry holds the set of widgets the Daily Hub can lay out.
// Built-in widgets register at init() via RegisterBuiltins(); Lua
// plugins (future) register their own. Same ID re-registered
// overwrites — last-registered wins, same as the profiles
// registry.
type Registry struct {
	mu      sync.RWMutex
	widgets map[string]Widget
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{widgets: make(map[string]Widget)}
}

// Register adds a widget by its ID(). Reregistering an ID is
// allowed (Lua plugins overriding built-ins, etc.) — the new
// widget replaces the old.
func (r *Registry) Register(w Widget) error {
	if w == nil {
		return errors.New("widgets: cannot register nil widget")
	}
	id := w.ID()
	if id == "" {
		return errors.New("widgets: widget has empty ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.widgets[id] = w
	return nil
}

// Get returns the widget for the given ID. Used by the Daily Hub
// when laying out cells from the active profile's DashboardSpec.
func (r *Registry) Get(id string) (Widget, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.widgets[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownWidget, id)
	}
	return w, nil
}

// IDs returns every registered widget ID — useful for the
// "Available widgets" view in a custom-profile editor.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.widgets))
	for id := range r.widgets {
		out = append(out, id)
	}
	return out
}

// RegisterBuiltins puts every compiled-in widget into the
// registry. Called from the Model boot path.
func RegisterBuiltins(r *Registry) error {
	for _, w := range builtinWidgets() {
		if err := r.Register(w); err != nil {
			return err
		}
	}
	return nil
}

// builtinWidgets is the list of v1 built-ins. New widget files
// drop their constructor in here.
func builtinWidgets() []Widget {
	return []Widget{
		newTodayJotWidget(),
		newTodayTasksWidget(),
		newTodayOverdueWidget(),
		newTriageCountWidget(),
		newTodayCalendarWidget(),
		newGoalProgressWidget(),
		newHabitStreakWidget(),
		newRecentNotesWidget(),
		newScriptureWidget(),
		newBusinessPulseWidget(),
	}
}
