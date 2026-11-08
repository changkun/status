// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

var svc = Service{ID: "a", Name: "A"}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// runs builds one run per state at 5 minute steps from start, for service a.
func runs(start time.Time, states ...State) []Run {
	var out []Run
	for i, s := range states {
		r := Result{Service: "a", OK: true, Code: 200, Ms: 100}
		switch s {
		case Down:
			r.OK, r.Code = false, 503
		case Degraded:
			r.Ms = int(SlowAfter/time.Millisecond) + 1
		}
		out = append(out, Run{Time: start.Add(time.Duration(i) * 5 * time.Minute), Results: []Result{r}})
	}
	return out
}

func TestDownOnlyAfterTwoFailuresInARow(t *testing.T) {
	now := at("2026-11-04T12:00:00Z")
	start := at("2026-11-04T10:00:00Z")
	tests := []struct {
		name   string
		states []State
		want   State
	}{
		{"all up", []State{Up, Up, Up}, Up},
		{"one failure is a blip", []State{Up, Up, Down}, Up},
		{"two failures in a row", []State{Up, Down, Down}, Down},
		{"stays down while failing", []State{Down, Down, Down, Down}, Down},
		{"recovers on the first good check", []State{Down, Down, Up}, Up},
		{"failures apart are blips", []State{Down, Up, Down}, Up},
		{"one slow check is not degraded", []State{Up, Degraded}, Up},
		{"two slow checks are degraded", []State{Up, Degraded, Degraded}, Degraded},
		{"slow then failing is degraded", []State{Degraded, Down}, Degraded},
		{"failing twice within a slow streak is down", []State{Down, Degraded, Down}, Down},
		{"a lone slow check after a failure is a blip", []State{Up, Down, Up, Degraded, Up}, Up},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Summarize([]Day{{Runs: runs(start, tt.states...)}}, svc, now).Current
			if got != tt.want {
				t.Errorf("Current = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSummarizeUptimePerDay(t *testing.T) {
	now := at("2026-11-04T12:00:00Z")
	// Day 1: 100 checks, 2 in a row down plus one lone failure: uptime 98%.
	// Day 2: 4 checks, all up.
	// Day 3 (today): one check, slow twice: amber, uptime 100%.
	var d1 []State
	for range 100 {
		d1 = append(d1, Up)
	}
	d1[10], d1[11], d1[50] = Down, Down, Down
	day1 := Day{Runs: runs(at("2026-11-02T00:00:00Z"), d1...)}
	day2 := Day{Runs: runs(at("2026-11-03T00:00:00Z"), Up, Up, Up, Up)}
	day3 := Day{Runs: runs(at("2026-11-04T00:00:00Z"), Degraded, Degraded)}

	st := Summarize([]Day{day1, day2, day3}, svc, now)
	if len(st.Bars) != HistoryDays {
		t.Fatalf("bars = %d, want %d", len(st.Bars), HistoryDays)
	}
	byDate := map[string]Bar{}
	for _, b := range st.Bars {
		byDate[b.Date] = b
	}
	if b := byDate["2026-11-02"]; b.Runs != 100 || b.Uptime != 0.98 || b.State != Down {
		t.Errorf("11-02 = %+v, want 100 runs, 0.98, down", b)
	}
	if b := byDate["2026-11-03"]; b.Uptime != 1 || b.State != Up {
		t.Errorf("11-03 = %+v, want 1, up", b)
	}
	if b := byDate["2026-11-04"]; b.Uptime != 1 || b.State != Degraded {
		t.Errorf("11-04 = %+v, want 1, degraded", b)
	}
	if b := byDate["2026-11-01"]; b.Runs != 0 || b.State != NoData {
		t.Errorf("11-01 = %+v, want no data", b)
	}
	if last := st.Bars[len(st.Bars)-1].Date; last != "2026-11-04" {
		t.Errorf("last bar = %s, want today", last)
	}
	if st.Bars[0].Date != "2026-08-07" {
		t.Errorf("first bar = %s, want 2026-08-07", st.Bars[0].Date)
	}
	want := float64(100+4+2-2) / float64(106)
	if st.Checks != 106 || st.Uptime != want {
		t.Errorf("Checks, Uptime = %d, %v, want 106, %v", st.Checks, st.Uptime, want)
	}
}

func TestShortOutageIsAmber(t *testing.T) {
	// 2 down of 288 checks is 99.3%: above the red line.
	states := make([]State, 288)
	for i := range states {
		states[i] = Up
	}
	states[100], states[101] = Down, Down
	st := Summarize([]Day{{Runs: runs(at("2026-11-04T00:00:00Z"), states...)}}, svc, at("2026-11-04T23:59:00Z"))
	if got := st.Bars[len(st.Bars)-1].State; got != Degraded {
		t.Errorf("state = %v, want amber", got)
	}
}

func TestOverall(t *testing.T) {
	mk := func(ss ...State) []ServiceStatus {
		var out []ServiceStatus
		for _, s := range ss {
			out = append(out, ServiceStatus{Current: s})
		}
		return out
	}
	tests := []struct {
		in   []ServiceStatus
		want State
	}{
		{nil, NoData},
		{mk(NoData), NoData},
		{mk(Up, Up), Up},
		{mk(Up, Degraded), Degraded},
		{mk(Degraded, Up, Down), Down},
		{mk(NoData, Up), Up},
	}
	for i, tt := range tests {
		if got := Overall(tt.in); got != tt.want {
			t.Errorf("case %d: Overall = %v, want %v", i, got, tt.want)
		}
	}
}

func TestPercentRoundsDown(t *testing.T) {
	tests := map[float64]string{
		1:           "100%",
		0.9999:      "99.99%",
		0.99999:     "99.99%",
		0.9998:      "99.98%",
		0.98:        "98%",
		0:           "0%",
		287.0 / 288: "99.65%",
	}
	for in, want := range tests {
		if got := Percent(in); got != want {
			t.Errorf("Percent(%v) = %s, want %s", in, got, want)
		}
	}
}

func TestLastChecked(t *testing.T) {
	days := []Day{{Runs: runs(at("2026-11-03T00:00:00Z"), Up)}, {Runs: runs(at("2026-11-04T01:00:00Z"), Up, Up)}}
	if got, want := LastChecked(days), at("2026-11-04T01:05:00Z"); !got.Equal(want) {
		t.Errorf("LastChecked = %v, want %v", got, want)
	}
	if !LastChecked(nil).IsZero() {
		t.Error("LastChecked(nil) is not zero")
	}
}

func TestStateString(t *testing.T) {
	var got []string
	for _, s := range []State{NoData, Up, Degraded, Down} {
		got = append(got, fmt.Sprint(s))
	}
	if want := []string{"nodata", "up", "degraded", "down"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
