// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"math"
	"strconv"
	"time"
)

// State is how a check, or a service, is doing.
type State int

const (
	// NoData means there is no check to judge.
	NoData State = iota
	// Up means healthy and fast enough.
	Up
	// Degraded means answering, but slower than SlowAfter.
	Degraded
	// Down means not answering as expected.
	Down
)

func (s State) String() string {
	switch s {
	case Up:
		return "up"
	case Degraded:
		return "degraded"
	case Down:
		return "down"
	}
	return "nodata"
}

// rawState judges one check alone.
func rawState(r Result) State {
	switch {
	case !r.OK:
		return Down
	case time.Duration(r.Ms)*time.Millisecond > SlowAfter:
		return Degraded
	}
	return Up
}

// confirm applies the two-in-a-row rule: a bad check counts only when it is
// part of a streak of at least ConfirmAfter bad checks in a row, slow or
// failed alike, and a lone bad check is a blip that reads as Up. Within a
// confirmed streak a check keeps its own state, except that a failure reads
// as Down only when the streak holds at least ConfirmAfter failures; fewer
// read as Degraded.
func confirm(raw []State) []State {
	out := make([]State, len(raw))
	for i := 0; i < len(raw); {
		if raw[i] == Up {
			out[i] = Up
			i++
			continue
		}
		j, downs := i, 0
		for j < len(raw) && raw[j] != Up {
			if raw[j] == Down {
				downs++
			}
			j++
		}
		for k := i; k < j; k++ {
			switch {
			case j-i < ConfirmAfter:
				out[k] = Up
			case raw[k] == Down && downs < ConfirmAfter:
				out[k] = Degraded
			default:
				out[k] = raw[k]
			}
		}
		i = j
	}
	return out
}

// Bar is one UTC day of one service.
type Bar struct {
	Date   string
	Runs   int
	Uptime float64 // fraction of checks that were not confirmed down
	State  State   // NoData, Up (green), Degraded (amber) or Down (red)
}

// redBelow is the uptime under which a day's bar is red. A shorter outage
// or any confirmed slowness is amber.
const redBelow = 0.99

// ServiceStatus is the current state of one service and its history.
type ServiceStatus struct {
	Service Service
	Current State
	Last    Result  // the newest check; zero with Checks == 0
	Uptime  float64 // over every check in the window; 0 with Checks == 0
	Checks  int
	Bars    []Bar // HistoryDays bars, oldest first, the last one is today
}

// check is one check of one service, judged with the two-in-a-row rule.
type check struct {
	time time.Time
	r    Result
	st   State
}

// timeline returns every check of svc in days, oldest first.
func timeline(days []Day, svc Service) []check {
	var cs []check
	var raw []State
	for _, d := range days {
		for _, run := range d.Runs {
			for _, r := range run.Results {
				if r.Service == svc.ID {
					cs = append(cs, check{time: run.Time, r: r})
					raw = append(raw, rawState(r))
				}
			}
		}
	}
	for i, st := range confirm(raw) {
		cs[i].st = st
	}
	return cs
}

// Summarize judges one service over the window that ends on now's day.
func Summarize(days []Day, svc Service, now time.Time) ServiceStatus {
	cs := timeline(days, svc)

	type tally struct{ runs, down, bad int }
	byDay := map[string]*tally{}
	var total, totalDown int
	for _, c := range cs {
		date := c.time.UTC().Format(dayLayout)
		t := byDay[date]
		if t == nil {
			t = &tally{}
			byDay[date] = t
		}
		t.runs++
		total++
		if c.st == Down {
			t.down++
			totalDown++
		}
		if c.st != Up {
			t.bad++
		}
	}

	st := ServiceStatus{Service: svc, Checks: total}
	if len(cs) > 0 {
		st.Current = cs[len(cs)-1].st
		st.Last = cs[len(cs)-1].r
		st.Uptime = float64(total-totalDown) / float64(total)
	}
	for i := HistoryDays - 1; i >= 0; i-- {
		date := now.UTC().AddDate(0, 0, -i).Format(dayLayout)
		b := Bar{Date: date}
		if t := byDay[date]; t != nil {
			b.Runs = t.runs
			b.Uptime = float64(t.runs-t.down) / float64(t.runs)
			switch {
			case b.Uptime < redBelow:
				b.State = Down
			case t.bad > 0:
				b.State = Degraded
			default:
				b.State = Up
			}
		}
		st.Bars = append(st.Bars, b)
	}
	return st
}

// Outage is a service that is down now: its newest checks are a confirmed
// streak of failures.
type Outage struct {
	Service Service
	Since   time.Time // the first check of the streak
	At      time.Time // the newest check
	Last    Result    // what the newest check found
}

// Outages lists the services that are down as of their newest check, in the
// order of services.
func Outages(days []Day, services []Service) []Outage {
	var out []Outage
	for _, s := range services {
		cs := timeline(days, s)
		i := len(cs)
		for i > 0 && cs[i-1].st == Down {
			i--
		}
		if i == len(cs) {
			continue
		}
		last := cs[len(cs)-1]
		out = append(out, Outage{Service: s, Since: cs[i].time, At: last.time, Last: last.r})
	}
	return out
}

// Overall is the page's headline state: Down when any service is down,
// else Degraded when any is degraded, else Up. With no checks at all it is
// NoData.
func Overall(ss []ServiceStatus) State {
	out := NoData
	for _, s := range ss {
		switch {
		case s.Current == Down:
			return Down
		case s.Current == Degraded:
			out = Degraded
		case s.Current == Up && out == NoData:
			out = Up
		}
	}
	return out
}

// LastChecked is the time of the newest run, or the zero time.
func LastChecked(days []Day) time.Time {
	var t time.Time
	for _, d := range days {
		for _, r := range d.Runs {
			if r.Time.After(t) {
				t = r.Time
			}
		}
	}
	return t
}

// Percent formats an uptime fraction for display. It rounds down so a
// service that was ever down never reads as 100%.
func Percent(f float64) string {
	p := math.Floor(f*10000+1e-9) / 100
	return strconv.FormatFloat(p, 'f', -1, 64) + "%"
}
