// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"fmt"
	"regexp"
	"sort"
	"time"
)

// PageURL is where the page is published.
const PageURL = "https://changkun.github.io/status/"

// OpenIssue is an open alert issue, as `gh issue list --json number,body`
// lists it.
type OpenIssue struct {
	Number int    `json:"number"`
	Body   string `json:"body"`
}

// Action is one change to the alert issues.
type Action struct {
	Action string `json:"action"`          // "open" or "close"
	Issue  int    `json:"issue,omitempty"` // the issue to close
	Title  string `json:"title,omitempty"` // the issue to open
	Body   string `json:"body"`            // the new issue's text, or the closing comment
}

// marker ties an issue to its component. Every alert issue carries it, and
// GitHub does not show it.
var marker = regexp.MustCompile(`<!-- status:component=([A-Za-z0-9._-]+) -->`)

func markerOf(id string) string { return "<!-- status:component=" + id + " -->" }

// Reconcile returns the actions that leave exactly one open alert issue for
// each component that is down: it opens one for an outage that has none, and
// closes those whose component recovered, is no longer checked, or already
// has an older issue. Issues without a marker are not alerts and are left
// alone. Closes come first, by issue number, then opens, in the order of
// services.
func Reconcile(outages []Outage, open []OpenIssue, services []Service, now time.Time) []Action {
	down := map[string]bool{}
	for _, o := range outages {
		down[o.Service.ID] = true
	}
	known := map[string]bool{}
	for _, s := range services {
		known[s.ID] = true
	}
	open = append([]OpenIssue(nil), open...)
	sort.Slice(open, func(i, j int) bool { return open[i].Number < open[j].Number })

	var acts []Action
	tracked := map[string]bool{}
	for _, is := range open {
		m := marker.FindStringSubmatch(is.Body)
		if m == nil {
			continue
		}
		id := m[1]
		var why string
		switch {
		case tracked[id]:
			why = "Closing as a duplicate: an older issue tracks this outage."
		case down[id]:
			tracked[id] = true
			continue
		case !known[id]:
			why = "Closing: this component is no longer checked."
		default:
			why = fmt.Sprintf("Checks pass again as of %s.", stamp(now))
		}
		acts = append(acts, Action{Action: "close", Issue: is.Number, Body: why})
	}
	for _, o := range outages {
		if tracked[o.Service.ID] {
			continue
		}
		acts = append(acts, Action{Action: "open", Title: title(o.Service), Body: body(o)})
	}
	return acts
}

func title(s Service) string { return fmt.Sprintf("%s: %s is down", s.Site, s.Name) }

func body(o Outage) string {
	found := o.Last.Error
	if found == "" {
		found = fmt.Sprintf("status %d", o.Last.Code)
	}
	return fmt.Sprintf(`%s on %s has failed every check since %s.

- URL: %s
- Last check, %s: %s

This issue closes itself once the checks pass again. See %s.

%s`, o.Service.Name, o.Service.Site, stamp(o.Since), o.Service.URL, stamp(o.At), found, PageURL, markerOf(o.Service.ID))
}
