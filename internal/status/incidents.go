package status

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Update is one dated note on an incident.
type Update struct {
	Time   time.Time `yaml:"time"`
	Status string    `yaml:"status"` // investigating, identified, monitoring or resolved
	Text   string    `yaml:"text"`
}

// Incident is one file under incidents/. The front matter carries the
// fields; the text after it is the summary.
type Incident struct {
	Title      string    `yaml:"title"`
	Components []string  `yaml:"components"` // component IDs, from services.go
	Started    time.Time `yaml:"started"`
	Resolved   time.Time `yaml:"resolved"` // zero while open
	Updates    []Update  `yaml:"updates"`
	Summary    string    `yaml:"-"`
	File       string    `yaml:"-"`
}

// Open reports whether the incident has no resolved time.
func (i Incident) Open() bool { return i.Resolved.IsZero() }

// ParseIncident reads one incident file: YAML front matter between two
// "---" lines, then the summary.
func ParseIncident(name string, src []byte, known []Service) (Incident, error) {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(src, []byte("---\n"))
	if !ok {
		return Incident{}, fmt.Errorf("%s: missing front matter", name)
	}
	front, body, ok := bytes.Cut(rest, []byte("\n---"))
	if !ok {
		return Incident{}, fmt.Errorf("%s: front matter is not closed with ---", name)
	}
	var inc Incident
	dec := yaml.NewDecoder(bytes.NewReader(front))
	dec.KnownFields(true)
	if err := dec.Decode(&inc); err != nil {
		return Incident{}, fmt.Errorf("%s: %w", name, err)
	}
	inc.File = name
	inc.Summary = strings.TrimSpace(string(body))
	switch {
	case strings.TrimSpace(inc.Title) == "":
		return Incident{}, fmt.Errorf("%s: title is required", name)
	case inc.Started.IsZero():
		return Incident{}, fmt.Errorf("%s: started is required", name)
	case !inc.Resolved.IsZero() && inc.Resolved.Before(inc.Started):
		return Incident{}, fmt.Errorf("%s: resolved is before started", name)
	}
	for _, id := range inc.Components {
		if !hasService(known, id) {
			return Incident{}, fmt.Errorf("%s: unknown component %q", name, id)
		}
	}
	sort.SliceStable(inc.Updates, func(a, b int) bool { return inc.Updates[a].Time.After(inc.Updates[b].Time) })
	return inc, nil
}

func hasService(ss []Service, id string) bool {
	for _, s := range ss {
		if s.ID == id {
			return true
		}
	}
	return false
}

// LoadIncidents reads every .md file in dir except README.md and files whose
// name starts with an underscore. A missing dir is no incidents. The first
// invalid file fails the load, so a typo cannot hide an incident.
func LoadIncidents(dir string, known []Service) ([]Incident, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Incident
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || filepath.Ext(n) != ".md" || n == "README.md" || strings.HasPrefix(n, "_") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		inc, err := ParseIncident(n, b, known)
		if err != nil {
			return nil, err
		}
		out = append(out, inc)
	}
	return out, nil
}

// SplitIncidents returns the open incidents and the incidents resolved
// within the last HistoryDays days of now, each newest first.
func SplitIncidents(all []Incident, now time.Time) (open, past []Incident) {
	from := now.AddDate(0, 0, -HistoryDays)
	for _, i := range all {
		switch {
		case i.Open():
			open = append(open, i)
		case !i.Resolved.Before(from):
			past = append(past, i)
		}
	}
	byStart := func(s []Incident) {
		sort.SliceStable(s, func(a, b int) bool { return s[a].Started.After(s[b].Started) })
	}
	byStart(open)
	byStart(past)
	return open, past
}
