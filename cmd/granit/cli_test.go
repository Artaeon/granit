package main

import (
	"os"
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// resolveVaultPath
// ---------------------------------------------------------------------------

func TestResolveVaultPath_FromArgsIndex(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "list", "/tmp/myvault"}

	if got := resolveVaultPath(2); got != "/tmp/myvault" {
		t.Errorf("expected '/tmp/myvault', got %q", got)
	}
}

func TestResolveVaultPath_FromEnv(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "list"}
	t.Setenv("GRANIT_VAULT", "/tmp/envvault")

	if got := resolveVaultPath(2); got != "/tmp/envvault" {
		t.Errorf("expected '/tmp/envvault', got %q", got)
	}
}

func TestResolveVaultPath_FallsBackToCwd(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "list"}
	t.Setenv("GRANIT_VAULT", "")

	if got := resolveVaultPath(2); got != "." {
		t.Errorf("expected '.', got %q", got)
	}
}

func TestResolveVaultPath_ArgWinsOverEnv(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "list", "/from/arg"}
	t.Setenv("GRANIT_VAULT", "/from/env")

	if got := resolveVaultPath(2); got != "/from/arg" {
		t.Errorf("expected '/from/arg' to win, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// hasFlag
// ---------------------------------------------------------------------------

func TestHasFlag_Present(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "list", "--json", "--quiet"}

	if !hasFlag("--json") {
		t.Error("expected --json to be present")
	}
	if !hasFlag("--quiet") {
		t.Error("expected --quiet to be present")
	}
}

func TestHasFlag_Absent(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "list"}

	if hasFlag("--json") {
		t.Error("expected --json to be absent")
	}
}

func TestHasFlag_PartialNameNotMatched(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "list", "--jsonish"}

	if hasFlag("--json") {
		t.Error("'--jsonish' should not match '--json'")
	}
}

// ---------------------------------------------------------------------------
// getFlagValue
// ---------------------------------------------------------------------------

func TestGetFlagValue_EqualsForm(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "--file=tasks.md", "buy milk"}

	if got := getFlagValue("--file"); got != "tasks.md" {
		t.Errorf("expected 'tasks.md', got %q", got)
	}
}

func TestGetFlagValue_SpaceForm(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "--file", "tasks.md", "buy milk"}

	if got := getFlagValue("--file"); got != "tasks.md" {
		t.Errorf("expected 'tasks.md', got %q", got)
	}
}

func TestGetFlagValue_Missing(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "buy milk"}

	if got := getFlagValue("--file"); got != "" {
		t.Errorf("expected empty for missing flag, got %q", got)
	}
}

func TestGetFlagValue_FlagAtEndNoValue(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "buy milk", "--file"}

	// Flag at end with no value should return empty (no panic)
	if got := getFlagValue("--file"); got != "" {
		t.Errorf("expected empty for flag with no following value, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// getPositionalArgs
// ---------------------------------------------------------------------------

func TestGetPositionalArgs_Mixed(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "buy", "milk", "--priority", "high", "--tag=urgent"}

	got := getPositionalArgs(2)
	want := []string{"buy", "milk"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestGetPositionalArgs_OnlyFlags(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "--priority", "high"}

	got := getPositionalArgs(2)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestGetPositionalArgs_OnlyPositionals(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"granit", "todo", "buy", "milk", "today"}

	got := getPositionalArgs(2)
	want := []string{"buy", "milk", "today"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestGetPositionalArgs_EqualsFlagDoesNotConsumeNext(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	// --tag=urgent has its value built in, so the next arg should be positional.
	os.Args = []string{"granit", "todo", "--tag=urgent", "buy"}

	got := getPositionalArgs(2)
	want := []string{"buy"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}
