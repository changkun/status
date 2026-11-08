// SPDX-FileCopyrightText: 2026 Latere AI
// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

// Command status probes the components of changkun.de, golang.design and
// poly.red, builds the status page, and decides on alert issues.
//
//	status probe [-history dir]
//	status build [-history dir] [-incidents dir] [-out dir] [-now time]
//	status alerts [-history dir] [-open file] [-now time]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"changkun.de/x/status/internal/status"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "status:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: status probe|build|alerts [flags]")
	}
	switch args[0] {
	case "probe":
		return probe(args[1:], stdout)
	case "build":
		return build(args[1:], stdout)
	case "alerts":
		return alerts(args[1:], stdout)
	}
	return fmt.Errorf("unknown command %q, want probe, build or alerts", args[0])
}

// probe checks every service, appends the run to history and trims history
// to the retention window. A failing service is a recorded result, not an
// error: only a failure to record makes probe fail.
func probe(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	dir := fs.String("history", "history", "directory of per-day history files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	now := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*status.Timeout)
	defer cancel()
	run := status.Probe(ctx, status.NewClient(), status.Services, now)
	for _, r := range run.Results {
		verdict := "ok"
		if !r.OK {
			verdict = "FAIL " + r.Error
		}
		fmt.Fprintf(stdout, "%-18s %5d ms  %s\n", r.Service, r.Ms, verdict)
	}
	store := status.Store{Dir: *dir}
	if err := store.Append(run); err != nil {
		return fmt.Errorf("record run: %w", err)
	}
	gone, err := store.Trim(now, status.HistoryDays)
	if err != nil {
		return fmt.Errorf("trim history: %w", err)
	}
	if len(gone) > 0 {
		fmt.Fprintf(stdout, "trimmed %d old day files\n", len(gone))
	}
	return nil
}

// alerts prints, as a JSON list, the changes that make the open alert
// issues match the components that are down: see status.Reconcile. -open is
// the open issues as `gh issue list --json number,body` writes them.
func alerts(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("alerts", flag.ContinueOnError)
	hist := fs.String("history", "history", "directory of per-day history files")
	open := fs.String("open", "", "JSON list of the open alert issues (default: none)")
	at := fs.String("now", "", "decide as of this RFC 3339 time (default: now)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	now, err := parseNow(*at)
	if err != nil {
		return err
	}
	days, err := status.Store{Dir: *hist}.Load(now, status.HistoryDays)
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}
	var issues []status.OpenIssue
	if *open != "" {
		b, err := os.ReadFile(*open)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &issues); err != nil {
			return fmt.Errorf("-open: %w", err)
		}
	}
	acts := status.Reconcile(status.Outages(days, status.Services), issues, status.Services, now)
	if acts == nil {
		acts = []status.Action{}
	}
	return json.NewEncoder(stdout).Encode(acts)
}

// parseNow parses a -now flag, which defaults to the current time.
func parseNow(at string) (time.Time, error) {
	if at == "" {
		return time.Now(), nil
	}
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return time.Time{}, fmt.Errorf("-now: %w", err)
	}
	return t, nil
}

// build renders the site from history and incidents.
func build(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	hist := fs.String("history", "history", "directory of per-day history files")
	inc := fs.String("incidents", "incidents", "directory of incident files")
	out := fs.String("out", "site", "output directory")
	at := fs.String("now", "", "render as of this RFC 3339 time (default: now)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	now, err := parseNow(*at)
	if err != nil {
		return err
	}
	if err := status.Build(*hist, *inc, *out, status.Services, now); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s/index.html\n", *out)
	return nil
}
