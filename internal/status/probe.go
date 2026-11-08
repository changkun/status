// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Result is one check of one service.
type Result struct {
	Service string `json:"service"`
	OK      bool   `json:"ok"`
	Code    int    `json:"code,omitempty"`
	Ms      int    `json:"ms"`
	Error   string `json:"error,omitempty"`
}

// Run is every result of one probe run.
type Run struct {
	Time    time.Time `json:"time"`
	Results []Result  `json:"results"`
}

// maxBody bounds how much of a body a body check reads.
const maxBody = 64 << 10

// NewClient returns the HTTP client probes use: a fixed timeout and no
// redirects, so a redirect is a result and not something followed.
func NewClient() *http.Client {
	return &http.Client{
		Timeout: Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Check performs one check of s.
func Check(ctx context.Context, c *http.Client, s Service) (r Result) {
	r.Service = s.ID
	start := time.Now()
	defer func() { r.Ms = int(time.Since(start).Milliseconds()) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	req.Header.Set("User-Agent", "changkun-status/1 (+"+PageURL+")")
	resp, err := c.Do(req)
	if err != nil {
		r.Error = shortError(err)
		return r
	}
	defer resp.Body.Close()
	r.Code = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		r.Error = shortError(err)
		return r
	}
	switch {
	case resp.StatusCode != s.Expect:
		r.Error = fmt.Sprintf("expected status %d, got %d", s.Expect, resp.StatusCode)
	case s.Contains != "" && !strings.Contains(string(body), s.Contains):
		r.Error = fmt.Sprintf("body does not contain %q", s.Contains)
	default:
		r.OK = true
	}
	return r
}

// shortError drops the URL a net/http error repeats.
func shortError(err error) string {
	if ue, ok := err.(interface{ Unwrap() error }); ok {
		if inner := ue.Unwrap(); inner != nil {
			return inner.Error()
		}
	}
	return err.Error()
}

// Probe checks every service at once and returns one run stamped now.
// Results keep the order of services.
func Probe(ctx context.Context, c *http.Client, services []Service, now time.Time) Run {
	run := Run{Time: now.UTC().Truncate(time.Second), Results: make([]Result, len(services))}
	var wg sync.WaitGroup
	for i, s := range services {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run.Results[i] = Check(ctx, c, s)
		}()
	}
	wg.Wait()
	return run
}
