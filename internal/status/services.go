// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

// Package status probes the public endpoints of changkun.de, golang.design
// and poly.red from outside the server that runs them, keeps the results as
// per-day history, renders the public status page, and decides which
// outages to raise as alerts.
package status

import "time"

// Service is one component the page reports on: one public endpoint of a
// site.
type Service struct {
	// ID is the stable key stored in history. Never reuse or rename one:
	// history under the old key would no longer match.
	ID string
	// Site groups components on the page, in the order they first appear.
	Site string
	// Name is what the page shows within the site.
	Name string
	// URL is fetched with GET and no credentials. Redirects are not
	// followed, so the URL must answer for itself.
	URL string
	// Expect is the status code a healthy answer carries.
	Expect int
	// Contains, when set, must appear in the first 64 KiB of the body.
	Contains string
}

// Thresholds and windows. Each is stated once here and used everywhere.
const (
	// Timeout bounds one check.
	Timeout = 10 * time.Second
	// SlowAfter is the latency above which an otherwise healthy check is
	// slow. A component is degraded after two slow checks in a row.
	SlowAfter = 3 * time.Second
	// ConfirmAfter is how many checks in a row must be bad before a
	// component is reported as down or degraded.
	ConfirmAfter = 2
	// HistoryDays is how many days of history are kept and shown.
	HistoryDays = 90
	// StaleAfter is the age of the newest check past which the page says
	// checks are delayed. GitHub may run a schedule late.
	StaleAfter = 30 * time.Minute
)

// goImport is what a go-import page carries.
const goImport = `name="go-import"`

// Services lists what is probed, in the order the page shows it. To add a
// component, add a line here. Only use endpoints that answer without
// credentials, and none that count as a visit: redir's link list is read
// through its data API, which records nothing, where the page itself would
// record every check.
var Services = []Service{
	{ID: "website", Site: "changkun.de", Name: "Website", URL: "https://changkun.de/", Expect: 200, Contains: "Changkun Ou"},
	{ID: "blog", Site: "changkun.de", Name: "Blog", URL: "https://changkun.de/blog/", Expect: 200},
	{ID: "research", Site: "changkun.de", Name: "Research", URL: "https://changkun.de/research/", Expect: 200, Contains: "Changkun Ou"},
	{ID: "books", Site: "changkun.de", Name: "Books", URL: "https://changkun.de/books/", Expect: 200, Contains: "<title>Books"},
	{ID: "modern-cpp", Site: "changkun.de", Name: "Modern C++", URL: "https://changkun.de/modern-cpp/", Expect: 200},
	{ID: "bobook", Site: "changkun.de", Name: "Bayesian Optimization", URL: "https://changkun.de/bobook/", Expect: 200, Contains: "<title>Bayesian Optimization"},
	{ID: "alignbook", Site: "changkun.de", Name: "AI Alignment", URL: "https://changkun.de/alignbook/", Expect: 200, Contains: "<title>AI Alignment"},
	{ID: "unverifiable-world", Site: "changkun.de", Name: "Unverifiable World", URL: "https://changkun.de/unverifiable-world/", Expect: 200, Contains: "An Unverifiable World"},
	{ID: "swedish-tutorial", Site: "changkun.de", Name: "Swedish Tutorial", URL: "https://changkun.de/swedish-tutorial/", Expect: 200, Contains: "瑞典语零基础教程"},
	{ID: "german-tutorial", Site: "changkun.de", Name: "German Tutorial", URL: "https://changkun.de/german-tutorial/", Expect: 200, Contains: "德语零基础教程"},
	{ID: "library", Site: "changkun.de", Name: "Library", URL: "https://changkun.de/library/", Expect: 200, Contains: ">Library</a>"},
	{ID: "links", Site: "changkun.de", Name: "Short links", URL: "https://changkun.de/s/?mode=index&ps=1", Expect: 200, Contains: `"data"`},
	{ID: "go-imports", Site: "changkun.de", Name: "Go imports", URL: "https://changkun.de/x/redir?go-get=1", Expect: 200, Contains: goImport},
	{ID: "urlstat", Site: "changkun.de", Name: "urlstat", URL: "https://changkun.de/urlstat/dashboard", Expect: 200},
	{ID: "midgard", Site: "changkun.de", Name: "Midgard", URL: "https://changkun.de/midgard/ping", Expect: 200},

	{ID: "gd-website", Site: "golang.design", Name: "Website", URL: "https://golang.design/", Expect: 200, Contains: "golang.design"},
	{ID: "gd-under-the-hood", Site: "golang.design", Name: "Go under the hood", URL: "https://golang.design/under-the-hood/", Expect: 200},
	{ID: "gd-go-questions", Site: "golang.design", Name: "Go questions", URL: "https://golang.design/go-questions/", Expect: 200},
	{ID: "gd-history", Site: "golang.design", Name: "Go history", URL: "https://golang.design/history/", Expect: 200},
	{ID: "gd-research", Site: "golang.design", Name: "Research", URL: "https://golang.design/research/", Expect: 200},
	{ID: "gd-miner", Site: "golang.design", Name: "Miner", URL: "https://golang.design/miner/", Expect: 200},
	{ID: "gd-links", Site: "golang.design", Name: "Short links", URL: "https://golang.design/s/?mode=index&ps=1", Expect: 200, Contains: `"data"`},
	{ID: "gd-go-imports", Site: "golang.design", Name: "Go imports", URL: "https://golang.design/x/redir?go-get=1", Expect: 200, Contains: goImport},
	{ID: "gd-gossa", Site: "golang.design", Name: "SSA playground", URL: "https://golang.design/gossa/", Expect: 200},

	{ID: "polyred", Site: "poly.red", Name: "Go import", URL: "https://poly.red/?go-get=1", Expect: 200, Contains: goImport},
	{ID: "rankui-study", Site: "poly.red", Name: "RankUI study", URL: "https://poly.red/study/", Expect: 200},
}
