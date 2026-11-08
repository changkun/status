// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Day is one history file: every run of one UTC day.
type Day struct {
	Date string `json:"date"`
	Runs []Run  `json:"runs"`
}

const dayLayout = "2006-01-02"

// Store keeps history as one JSON file per UTC day, named YYYY-MM-DD.json.
type Store struct{ Dir string }

func (s Store) path(date string) string { return filepath.Join(s.Dir, date+".json") }

// Append adds run to the file of its UTC day, creating it if needed.
func (s Store) Append(run Run) error {
	date := run.Time.UTC().Format(dayLayout)
	d, err := s.readDay(date)
	if err != nil {
		return err
	}
	d.Runs = append(d.Runs, run)
	sort.SliceStable(d.Runs, func(i, j int) bool { return d.Runs[i].Time.Before(d.Runs[j].Time) })
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	tmp := s.path(date) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(date))
}

// readDay returns the day's file, or an empty day when there is none.
func (s Store) readDay(date string) (Day, error) {
	b, err := os.ReadFile(s.path(date))
	if errors.Is(err, fs.ErrNotExist) {
		return Day{Date: date}, nil
	}
	if err != nil {
		return Day{}, err
	}
	var d Day
	if err := json.Unmarshal(b, &d); err != nil {
		return Day{}, fmt.Errorf("%s: %w", s.path(date), err)
	}
	return d, nil
}

// dates lists the day files in the store, oldest first. Files that are not
// named for a date are ignored.
func (s Store) dates() ([]string, error) {
	ents, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.IsDir() {
			continue
		}
		if _, err := time.Parse(dayLayout, name); err != nil {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// cutoff is the first UTC day inside the window that ends on now's day.
func cutoff(now time.Time, days int) string {
	return now.UTC().AddDate(0, 0, -(days - 1)).Format(dayLayout)
}

// Trim deletes the day files older than the last days days counting now's
// day, and returns the dates it deleted.
func (s Store) Trim(now time.Time, days int) ([]string, error) {
	ds, err := s.dates()
	if err != nil {
		return nil, err
	}
	var gone []string
	for _, d := range ds {
		if d >= cutoff(now, days) {
			continue
		}
		if err := os.Remove(s.path(d)); err != nil {
			return gone, err
		}
		gone = append(gone, d)
	}
	return gone, nil
}

// Load returns the days inside the window that ends on now's day, oldest
// first.
func (s Store) Load(now time.Time, days int) ([]Day, error) {
	ds, err := s.dates()
	if err != nil {
		return nil, err
	}
	var out []Day
	for _, date := range ds {
		if date < cutoff(now, days) || date > now.UTC().Format(dayLayout) {
			continue
		}
		d, err := s.readDay(date)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}
