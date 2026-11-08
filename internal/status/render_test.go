// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

var fixtureServices = []Service{
	{ID: "website", Site: "changkun.de", Name: "Website"}, {ID: "blog", Site: "changkun.de", Name: "Blog"},
	{ID: "links", Site: "changkun.de", Name: "Short links"}, {ID: "go-imports", Site: "changkun.de", Name: "Go imports"},
	{ID: "gd-website", Site: "golang.design", Name: "Website"}, {ID: "gd-gossa", Site: "golang.design", Name: "SSA playground"},
	{ID: "polyred", Site: "poly.red", Name: "Go import"},
}

func renderFixture(t *testing.T, now string) string {
	t.Helper()
	n := at(now)
	days, err := Store{Dir: "../../testdata/history"}.Load(n, HistoryDays)
	if err != nil {
		t.Fatal(err)
	}
	incidents, err := LoadIncidents("../../testdata/incidents", fixtureServices)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Render(&buf, fixtureServices, days, incidents, n); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestRenderGolden(t *testing.T) {
	got := renderFixture(t, "2026-11-04T12:00:00Z")
	golden := filepath.Join("..", "..", "testdata", "index.golden.html")
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("rendered page differs from %s; run go test ./internal/status -update and review the diff", golden)
	}
}

func TestRenderContent(t *testing.T) {
	got := renderFixture(t, "2026-11-04T12:00:00Z")
	for _, want := range []string{
		`Some components are <span class="degraded">degraded</span>`, // links has been slow for three checks
		"Open incidents", "Short links are slow",
		"Past incidents", "The blog was unavailable",
		"Last checked <time datetime=\"2026-11-04T11:55:00Z\">Nov 4, 2026 11:55 UTC</time>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if n := strings.Count(got, `class="bar `); n != HistoryDays*len(fixtureServices) {
		t.Errorf("bars = %d, want %d", n, HistoryDays*len(fixtureServices))
	}
	if strings.Contains(got, "newest check is more than") {
		t.Error("fresh page shows the stale notice")
	}
}

func TestRenderStaleAndEmpty(t *testing.T) {
	if got := renderFixture(t, "2026-11-04T13:00:00Z"); !strings.Contains(got, "newest check is more than 30 minutes old") {
		t.Error("stale page has no delay notice")
	}
	var buf bytes.Buffer
	if err := Render(&buf, fixtureServices, nil, nil, at("2026-11-04T12:00:00Z")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Waiting for the first check", "No incidents in the past 90 days", "No data"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("empty page lacks %q", want)
		}
	}
}

func TestRenderEscapesIncidentText(t *testing.T) {
	inc := []Incident{{Title: "<script>alert(1)</script>", Started: at("2026-11-04T10:00:00Z"), Summary: "a & b"}}
	var buf bytes.Buffer
	if err := Render(&buf, fixtureServices, nil, inc, at("2026-11-04T12:00:00Z")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<script>") || !strings.Contains(buf.String(), "a &amp; b") {
		t.Error("incident text is not escaped")
	}
}

func TestPageLoadsNothingExternal(t *testing.T) {
	got := renderFixture(t, "2026-11-04T12:00:00Z")
	css, err := webFS.ReadFile("web/style.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range []string{got, string(css)} {
		if strings.Contains(doc, "<script") || strings.Contains(doc, "@import") || strings.Contains(doc, "http://") {
			t.Error("page pulls in a script, import or plain http resource")
		}
	}
	if strings.Contains(got, "—") {
		t.Error("page copy has an em dash")
	}
}

func TestBuildWritesSite(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "CNAME"), []byte("status.changkun.de\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Build("../../testdata/history", "../../testdata/incidents", out, fixtureServices, at("2026-11-04T12:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", "style.css", "CNAME", "fonts/inter-400-latin.woff2", "fonts/instrument-serif-italic-latin.woff2"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(out, "index.html.tmpl")); err == nil {
		t.Error("template was copied to the site")
	}
}

func TestBuildFailsOnBadIncident(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte("no front matter"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Build("../../testdata/history", dir, t.TempDir(), fixtureServices, at("2026-11-04T12:00:00Z"))
	if err == nil || !strings.Contains(err.Error(), "bad.md") {
		t.Errorf("err = %v, want it to name bad.md", err)
	}
}
