// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"strings"
	"testing"
)

var known = []Service{{ID: "blog", Site: "changkun.de", Name: "Blog"}, {ID: "links", Site: "changkun.de", Name: "Short links"}}

const goodIncident = `---
title: The blog is down
components: [blog, links]
started: 2026-11-04T10:00:00Z
resolved:
updates:
  - time: 2026-11-04T10:05:00Z
    status: Investigating
    text: Looking.
  - time: 2026-11-04T10:30:00Z
    status: Identified
    text: Found it.
---
First paragraph.

Second paragraph.
`

func TestParseIncident(t *testing.T) {
	inc, err := ParseIncident("a.md", []byte(goodIncident), known)
	if err != nil {
		t.Fatal(err)
	}
	if inc.Title != "The blog is down" || !inc.Open() || len(inc.Components) != 2 {
		t.Errorf("incident = %+v", inc)
	}
	if !inc.Started.Equal(at("2026-11-04T10:00:00Z")) {
		t.Errorf("started = %v", inc.Started)
	}
	if len(inc.Updates) != 2 || inc.Updates[0].Status != "Identified" {
		t.Errorf("updates should be newest first: %+v", inc.Updates)
	}
	if inc.Summary != "First paragraph.\n\nSecond paragraph." {
		t.Errorf("summary = %q", inc.Summary)
	}
}

func TestParseIncidentResolved(t *testing.T) {
	src := strings.Replace(goodIncident, "resolved:\n", "resolved: 2026-11-04T11:00:00Z\n", 1)
	inc, err := ParseIncident("a.md", []byte(src), known)
	if err != nil {
		t.Fatal(err)
	}
	if inc.Open() || !inc.Resolved.Equal(at("2026-11-04T11:00:00Z")) {
		t.Errorf("incident = %+v", inc)
	}
}

func TestParseIncidentCRLF(t *testing.T) {
	src := strings.ReplaceAll(goodIncident, "\n", "\r\n")
	if _, err := ParseIncident("a.md", []byte(src), known); err != nil {
		t.Fatal(err)
	}
}

func TestParseIncidentErrors(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"no front matter", "just text", "missing front matter"},
		{"unclosed", "---\ntitle: x\n", "not closed"},
		{"no title", "---\nstarted: 2026-11-04T10:00:00Z\n---\n", "title is required"},
		{"no started", "---\ntitle: x\n---\n", "started is required"},
		{"unknown field", "---\ntitle: x\nstarted: 2026-11-04T10:00:00Z\ncomponets: [blog]\n---\n", "componets"},
		{"the old services key", "---\ntitle: x\nstarted: 2026-11-04T10:00:00Z\nservices: [blog]\n---\n", "services"},
		{"unknown component", "---\ntitle: x\nstarted: 2026-11-04T10:00:00Z\ncomponents: [nope]\n---\n", `unknown component "nope"`},
		{"resolved before started", "---\ntitle: x\nstarted: 2026-11-04T10:00:00Z\nresolved: 2026-11-04T09:00:00Z\n---\n", "before started"},
		{"bad time", "---\ntitle: x\nstarted: yesterday\n---\n", "yesterday"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseIncident("a.md", []byte(tt.src), known)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestLoadIncidentsFixtures(t *testing.T) {
	svcs := []Service{{ID: "blog"}, {ID: "links"}, {ID: "go-imports"}}
	got, err := LoadIncidents("../../testdata/incidents", svcs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d incidents, want 2 (the underscore file is skipped)", len(got))
	}
	if _, err := LoadIncidents("../../testdata/nope", svcs); err != nil {
		t.Errorf("missing dir: %v", err)
	}
}

func TestSplitIncidents(t *testing.T) {
	now := at("2026-11-04T12:00:00Z")
	mk := func(title, started, resolved string) Incident {
		i := Incident{Title: title, Started: at(started)}
		if resolved != "" {
			i.Resolved = at(resolved)
		}
		return i
	}
	all := []Incident{
		mk("old", "2026-07-01T00:00:00Z", "2026-07-01T01:00:00Z"),
		mk("edge", "2026-08-05T00:00:00Z", "2026-08-06T12:00:00Z"),
		mk("recent", "2026-11-01T00:00:00Z", "2026-11-01T01:00:00Z"),
		mk("newer", "2026-11-03T00:00:00Z", "2026-11-03T01:00:00Z"),
		mk("open-old", "2026-05-01T00:00:00Z", ""),
		mk("open", "2026-11-04T10:00:00Z", ""),
	}
	open, past := SplitIncidents(all, now)
	titles := func(s []Incident) (out []string) {
		for _, i := range s {
			out = append(out, i.Title)
		}
		return
	}
	if got := strings.Join(titles(open), ","); got != "open,open-old" {
		t.Errorf("open = %s (an open incident always shows, newest first)", got)
	}
	if got := strings.Join(titles(past), ","); got != "newer,recent,edge" {
		t.Errorf("past = %s", got)
	}
}
