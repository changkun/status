// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestOutages checks that a component is an outage exactly when it is down
// now, and that the outage starts at the first failure of its streak.
func TestOutages(t *testing.T) {
	start := at("2026-11-04T10:00:00Z")
	for _, tt := range []struct {
		name   string
		states []State
		since  int // index of the first failing check, or -1 for no outage
	}{
		{"running", []State{Up, Up, Up}, -1},
		{"one failure is a blip", []State{Up, Up, Down}, -1},
		{"down since the second check", []State{Up, Down, Down, Down}, 1},
		{"recovered", []State{Down, Down, Up}, -1},
		{"slow is not down", []State{Up, Degraded, Degraded}, -1},
		{"a new streak after a recovery", []State{Down, Down, Up, Down, Down}, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Outages([]Day{{Runs: runs(start, tt.states...)}}, []Service{svc})
			if tt.since < 0 {
				if len(got) != 0 {
					t.Errorf("outages = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("outages = %+v, want one", got)
			}
			o := got[0]
			wantSince := start.Add(5 * time.Minute * time.Duration(tt.since))
			wantAt := start.Add(5 * time.Minute * time.Duration(len(tt.states)-1))
			if o.Service.ID != "a" || !o.Since.Equal(wantSince) || !o.At.Equal(wantAt) || o.Last.Code != 503 {
				t.Errorf("outage = %+v, want since %v, at %v, code 503", o, wantSince, wantAt)
			}
		})
	}
}

var alertServices = []Service{
	{ID: "website", Site: "changkun.de", Name: "Website", URL: "https://changkun.de/"},
	{ID: "gd-gossa", Site: "golang.design", Name: "SSA playground", URL: "https://golang.design/gossa/"},
}

func outage(id string) Outage {
	for _, s := range alertServices {
		if s.ID == id {
			return Outage{Service: s, Since: at("2026-11-08T06:40:00Z"), At: at("2026-11-08T06:50:00Z"),
				Last: Result{Service: id, Code: 502, Error: "expected status 200, got 502"}}
		}
	}
	panic(id)
}

func issue(n int, id string) OpenIssue {
	return OpenIssue{Number: n, Body: "text\n\n" + markerOf(id)}
}

func TestReconcile(t *testing.T) {
	now := at("2026-11-08T07:00:00Z")
	for _, tt := range []struct {
		name    string
		outages []Outage
		open    []OpenIssue
		want    []string // "open <id>" or "close <n>"
	}{
		{"nothing down, nothing open", nil, nil, nil},
		{"a new outage opens an issue", []Outage{outage("gd-gossa")}, nil, []string{"open gd-gossa"}},
		{"an outage keeps its issue", []Outage{outage("gd-gossa")}, []OpenIssue{issue(7, "gd-gossa")}, nil},
		{"a recovery closes the issue", nil, []OpenIssue{issue(7, "gd-gossa")}, []string{"close 7"}},
		{"a component no longer checked closes", nil, []OpenIssue{issue(3, "upbot")}, []string{"close 3"}},
		{"the newer of two issues closes", []Outage{outage("website")},
			[]OpenIssue{issue(9, "website"), issue(4, "website")}, []string{"close 9"}},
		{"issues without a marker are not alerts", nil, []OpenIssue{{Number: 5, Body: "a question"}}, nil},
		{"closes first, then opens in the order of services",
			[]Outage{outage("website"), outage("gd-gossa")}, []OpenIssue{issue(2, "upbot")},
			[]string{"close 2", "open website", "open gd-gossa"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, a := range Reconcile(tt.outages, tt.open, alertServices, now) {
				switch a.Action {
				case "open":
					got = append(got, "open "+marker.FindStringSubmatch(a.Body)[1])
				case "close":
					got = append(got, "close "+strconv.Itoa(a.Issue))
				default:
					t.Errorf("unknown action %+v", a)
				}
			}
			if strings.Join(got, ", ") != strings.Join(tt.want, ", ") {
				t.Errorf("actions = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAlertText checks what the issue and the closing comment say: what is
// down, since when, and what the last check found.
func TestAlertText(t *testing.T) {
	now := at("2026-11-08T07:00:00Z")
	acts := Reconcile([]Outage{outage("gd-gossa")}, nil, alertServices, now)
	if len(acts) != 1 {
		t.Fatalf("actions = %+v", acts)
	}
	a := acts[0]
	if a.Title != "golang.design: SSA playground is down" {
		t.Errorf("title = %q", a.Title)
	}
	for _, want := range []string{
		"since Nov 8, 2026 06:40 UTC", "https://golang.design/gossa/",
		"Nov 8, 2026 06:50 UTC: expected status 200, got 502", PageURL, markerOf("gd-gossa"),
	} {
		if !strings.Contains(a.Body, want) {
			t.Errorf("body lacks %q:\n%s", want, a.Body)
		}
	}
	closed := Reconcile(nil, []OpenIssue{issue(7, "gd-gossa")}, alertServices, now)
	if len(closed) != 1 || !strings.Contains(closed[0].Body, "Checks pass again as of Nov 8, 2026 07:00 UTC") {
		t.Errorf("closing = %+v", closed)
	}
	gone := Reconcile(nil, []OpenIssue{issue(3, "upbot")}, alertServices, now)
	if len(gone) != 1 || !strings.Contains(gone[0].Body, "no longer checked") {
		t.Errorf("closing for a removed component = %+v", gone)
	}
}

// TestRenderGroupsBySite checks that each site is a section in the order of
// its first component, that its label counts what is not running, and that
// a running component shows its last response time.
func TestRenderGroupsBySite(t *testing.T) {
	start := at("2026-11-08T06:00:00Z")
	var rs []Run
	for i := range 3 {
		rs = append(rs, Run{Time: start.Add(time.Duration(i) * 5 * time.Minute), Results: []Result{
			{Service: "website", OK: true, Code: 200, Ms: 120},
			{Service: "gd-gossa", Code: 502, Ms: 80, Error: "expected status 200, got 502"},
		}})
	}
	var buf bytes.Buffer
	if err := Render(&buf, alertServices, []Day{{Date: "2026-11-08", Runs: rs}}, nil, at("2026-11-08T06:15:00Z")); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{
		`Some components are <span class="down">down</span>`,
		`<span class="state up">All running</span>`,
		`<span class="state down">1 down</span>`,
		`<span class="num state up">120 ms</span>`,
		`<span class="num state down">Down</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if i, j := strings.Index(got, ">changkun.de</a>"), strings.Index(got, ">golang.design</a>"); i < 0 || j < i {
		t.Errorf("sites out of order: changkun.de at %d, golang.design at %d", i, j)
	}
	if strings.Contains(got, "80 ms") {
		t.Error("a component that is down shows a response time")
	}
}
