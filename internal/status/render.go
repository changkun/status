// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed web
var webFS embed.FS

var page = template.Must(template.ParseFS(webFS, "web/index.html.tmpl"))

type barView struct{ Class, Tip string }

type serviceView struct {
	Name, Class, Label, Latency, Uptime string
	Bars                                []barView
}

// siteView is one site and its components.
type siteView struct {
	Name, Class, Label string
	Services           []serviceView
}

type updateView struct{ Status, Time, ISO, Text string }

type incidentView struct {
	Title, Started, StartedISO, Resolved, ResolvedISO, Components string
	Open                                                          bool
	Paragraphs                                                    []string
	Updates                                                       []updateView
}

type pageView struct {
	// Lead and Word make the headline; Word names the state and takes its
	// color.
	Lead, Word, Class           string
	Stale                       bool
	LastChecked, LastCheckedISO string
	Sites                       []siteView
	Open, Past                  []incidentView
}

const (
	stampLayout = "Jan 2, 2006 15:04 UTC"
	dateLayout  = "Jan 2, 2006"
)

func stamp(t time.Time) string { return t.UTC().Format(stampLayout) }

func iso(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// Render writes the status page for the given inputs. It is a pure function
// of its arguments: now is passed in, never read.
func Render(w io.Writer, services []Service, days []Day, incidents []Incident, now time.Time) error {
	var statuses []ServiceStatus
	for _, s := range services {
		statuses = append(statuses, Summarize(days, s, now))
	}
	v := pageView{}
	overall := Overall(statuses)
	v.Class = overall.String()
	switch overall {
	case Up:
		v.Lead, v.Word = "All components are ", "running"
	case Degraded:
		v.Lead, v.Word = "Some components are ", "degraded"
	case Down:
		v.Lead, v.Word = "Some components are ", "down"
		if allDown(statuses) {
			v.Lead = "All components are "
		}
	default:
		v.Lead = "Waiting for the first check"
	}
	if last := LastChecked(days); !last.IsZero() {
		v.LastChecked, v.LastCheckedISO = stamp(last), iso(last)
		v.Stale = now.Sub(last) > StaleAfter
	}
	// Sites in the order they first appear, each with its statuses.
	siteIndex := map[string]int{}
	var bySite [][]ServiceStatus
	for _, st := range statuses {
		sv := serviceView{Name: st.Service.Name, Class: st.Current.String(), Label: label(st.Current)}
		if st.Checks > 0 {
			sv.Uptime = Percent(st.Uptime)
			if st.Last.OK {
				sv.Latency = fmt.Sprintf("%d ms", st.Last.Ms)
			}
		}
		for _, b := range st.Bars {
			sv.Bars = append(sv.Bars, barView{Class: b.State.String(), Tip: tip(b)})
		}
		i, seen := siteIndex[st.Service.Site]
		if !seen {
			i = len(v.Sites)
			siteIndex[st.Service.Site] = i
			v.Sites = append(v.Sites, siteView{Name: st.Service.Site})
			bySite = append(bySite, nil)
		}
		v.Sites[i].Services = append(v.Sites[i].Services, sv)
		bySite[i] = append(bySite[i], st)
	}
	for i, ss := range bySite {
		v.Sites[i].Class, v.Sites[i].Label = Overall(ss).String(), siteLabel(ss)
	}
	open, past := SplitIncidents(incidents, now)
	for _, i := range open {
		v.Open = append(v.Open, incidentOf(i, services))
	}
	for _, i := range past {
		v.Past = append(v.Past, incidentOf(i, services))
	}
	return page.Execute(w, v)
}

func allDown(ss []ServiceStatus) bool {
	for _, s := range ss {
		if s.Current != Down {
			return false
		}
	}
	return true
}

// siteLabel counts a site's components that are not running, or says all
// are.
func siteLabel(ss []ServiceStatus) string {
	var down, slow, up int
	for _, s := range ss {
		switch s.Current {
		case Down:
			down++
		case Degraded:
			slow++
		case Up:
			up++
		}
	}
	var parts []string
	if down > 0 {
		parts = append(parts, fmt.Sprintf("%d down", down))
	}
	if slow > 0 {
		parts = append(parts, fmt.Sprintf("%d degraded", slow))
	}
	switch {
	case len(parts) > 0:
		return strings.Join(parts, ", ")
	case up > 0:
		return "All running"
	}
	return "No data"
}

func label(s State) string {
	switch s {
	case Up:
		return "Running"
	case Degraded:
		return "Degraded"
	case Down:
		return "Down"
	}
	return "No data"
}

func tip(b Bar) string {
	d, err := time.Parse(dayLayout, b.Date)
	date := b.Date
	if err == nil {
		date = d.Format(dateLayout)
	}
	if b.Runs == 0 {
		return date + ": no data"
	}
	return fmt.Sprintf("%s: %s uptime", date, Percent(b.Uptime))
}

func incidentOf(i Incident, all []Service) incidentView {
	v := incidentView{
		Title: i.Title, Open: i.Open(),
		Started: stamp(i.Started), StartedISO: iso(i.Started),
	}
	if !i.Open() {
		v.Resolved, v.ResolvedISO = stamp(i.Resolved), iso(i.Resolved)
	}
	var names []string
	for _, id := range i.Components {
		for _, s := range all {
			if s.ID == id {
				names = append(names, s.Site+" "+s.Name)
			}
		}
	}
	v.Components = strings.Join(names, ", ")
	for p := range strings.SplitSeq(i.Summary, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			v.Paragraphs = append(v.Paragraphs, p)
		}
	}
	for _, u := range i.Updates {
		v.Updates = append(v.Updates, updateView{Status: u.Status, Time: stamp(u.Time), ISO: iso(u.Time), Text: strings.TrimSpace(u.Text)})
	}
	return v
}

// Build renders the site into out: index.html, style.css and the fonts. It
// overwrites those files and leaves anything else in out, such as CNAME.
func Build(historyDir, incidentsDir, out string, services []Service, now time.Time) error {
	days, err := Store{Dir: historyDir}.Load(now, HistoryDays)
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}
	incidents, err := LoadIncidents(incidentsDir, services)
	if err != nil {
		return fmt.Errorf("load incidents: %w", err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(out, "index.html"))
	if err != nil {
		return err
	}
	if err := Render(f, services, days, incidents, now); err != nil {
		f.Close()
		return fmt.Errorf("render: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	return copyAssets(out)
}

func copyAssets(out string) error {
	return fs.WalkDir(webFS, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "web" || strings.HasSuffix(p, ".tmpl") {
			return err
		}
		dst := filepath.Join(out, strings.TrimPrefix(p, "web/"))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := webFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
}
