// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func oneRun(ts string) Run {
	return Run{Time: at(ts), Results: []Result{{Service: "a", OK: true, Code: 200, Ms: 10}}}
}

func TestAppendCreatesAndExtendsDayFile(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "history")}
	for _, ts := range []string{"2026-11-04T10:05:00Z", "2026-11-04T10:00:00Z", "2026-11-05T00:00:00Z"} {
		if err := s.Append(oneRun(ts)); err != nil {
			t.Fatal(err)
		}
	}
	days, err := s.Load(at("2026-11-05T12:00:00Z"), HistoryDays)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 || days[0].Date != "2026-11-04" || len(days[0].Runs) != 2 || len(days[1].Runs) != 1 {
		t.Fatalf("days = %+v", days)
	}
	if !days[0].Runs[0].Time.Before(days[0].Runs[1].Time) {
		t.Error("runs of a day are not in time order")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "2026-11-04.json.tmp")); !os.IsNotExist(err) {
		t.Error("temporary file left behind")
	}
}

func TestAppendRejectsCorruptDayFile(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(s.Dir, "2026-11-04.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Append(oneRun("2026-11-04T10:00:00Z")); err == nil {
		t.Fatal("Append over a corrupt file succeeded; it would have discarded history")
	}
}

func TestTrimKeepsNinetyDays(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	now := at("2026-11-04T12:00:00Z")
	// 2026-08-07 is the 90th day counting today; 08-06 is the 91st.
	for _, d := range []string{"2026-08-05", "2026-08-06", "2026-08-07", "2026-08-08", "2026-11-04"} {
		if err := s.Append(oneRun(d + "T00:00:00Z")); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"notes.txt", "x.json", "README.md"} {
		if err := os.WriteFile(filepath.Join(s.Dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gone, err := s.Trim(now, HistoryDays)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2026-08-05", "2026-08-06"}; !slices.Equal(gone, want) {
		t.Errorf("trimmed %v, want %v", gone, want)
	}
	left, _ := s.dates()
	if want := []string{"2026-08-07", "2026-08-08", "2026-11-04"}; !slices.Equal(left, want) {
		t.Errorf("left %v, want %v", left, want)
	}
	for _, name := range []string{"notes.txt", "x.json", "README.md"} {
		if _, err := os.Stat(filepath.Join(s.Dir, name)); err != nil {
			t.Errorf("Trim touched %s: %v", name, err)
		}
	}
}

func TestLoadSkipsDaysOutsideWindow(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, d := range []string{"2026-08-06", "2026-08-07", "2026-11-04", "2026-11-06"} {
		if err := s.Append(oneRun(d + "T00:00:00Z")); err != nil {
			t.Fatal(err)
		}
	}
	days, err := s.Load(at("2026-11-04T12:00:00Z"), HistoryDays)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range days {
		got = append(got, d.Date)
	}
	if want := []string{"2026-08-07", "2026-11-04"}; !slices.Equal(got, want) {
		t.Errorf("loaded %v, want %v", got, want)
	}
}

func TestLoadMissingDir(t *testing.T) {
	days, err := Store{Dir: filepath.Join(t.TempDir(), "none")}.Load(at("2026-11-04T00:00:00Z"), HistoryDays)
	if err != nil || len(days) != 0 {
		t.Errorf("Load = %v, %v, want none", days, err)
	}
}
