// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"changkun.de/x/status/internal/status"
)

func TestBuildCommand(t *testing.T) {
	out := t.TempDir()
	var stdout bytes.Buffer
	err := run([]string{"build", "-history", "../../testdata/history", "-incidents", "../../testdata/incidents",
		"-out", out, "-now", "2026-11-04T12:00:00Z"}, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil || !strings.Contains(string(b), "Status | changkun.de") {
		t.Errorf("index.html: %v", err)
	}
	if !strings.Contains(stdout.String(), "index.html") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRunErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"nope"}, {"build", "-now", "yesterday"}, {"probe", "-bogus"}} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Errorf("run(%v) succeeded", args)
		}
	}
}

// TestAlertsCommand checks that alerts reads history and the open issues,
// and prints the actions as JSON: an issue for a component that failed
// twice, none while its issue is open, and an empty list rather than null.
func TestAlertsCommand(t *testing.T) {
	hist := t.TempDir()
	day := `{"date":"2026-11-08","runs":[
{"time":"2026-11-08T06:40:00Z","results":[{"service":"website","ok":false,"code":502,"ms":90}]},
{"time":"2026-11-08T06:45:00Z","results":[{"service":"website","ok":false,"code":502,"ms":90}]}]}`
	if err := os.WriteFile(filepath.Join(hist, "2026-11-08.json"), []byte(day), 0o644); err != nil {
		t.Fatal(err)
	}
	alerts := func(open string) []status.Action {
		t.Helper()
		args := []string{"alerts", "-history", hist, "-now", "2026-11-08T06:50:00Z"}
		if open != "" {
			f := filepath.Join(t.TempDir(), "open.json")
			if err := os.WriteFile(f, []byte(open), 0o644); err != nil {
				t.Fatal(err)
			}
			args = append(args, "-open", f)
		}
		var stdout bytes.Buffer
		if err := run(args, &stdout); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(stdout.String()) == "null" {
			t.Fatal("alerts printed null")
		}
		var acts []status.Action
		if err := json.Unmarshal(stdout.Bytes(), &acts); err != nil {
			t.Fatalf("%v: %s", err, stdout.String())
		}
		return acts
	}
	if acts := alerts(""); len(acts) != 1 || acts[0].Action != "open" || acts[0].Title != "changkun.de: Website is down" {
		t.Errorf("without issues: %+v", acts)
	}
	open := `[{"number":4,"body":"x\n\n<!-- status:component=website -->"}]`
	if acts := alerts(open); len(acts) != 0 {
		t.Errorf("with the issue open: %+v", acts)
	}
}
