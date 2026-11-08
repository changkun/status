// SPDX-FileCopyrightText: 2026 Changkun Ou
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.HandleFunc("/unavailable", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ok", http.StatusFound) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) { time.Sleep(300 * time.Millisecond) })
	mux.HandleFunc("/ua", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.UserAgent(), "changkun-status/") {
			w.WriteHeader(400)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewClient()
	client.Timeout = 100 * time.Millisecond

	tests := []struct {
		name    string
		svc     Service
		ok      bool
		code    int
		errPart string
	}{
		{"expected status", Service{URL: srv.URL + "/ok", Expect: 200}, true, 200, ""},
		{"body check passes", Service{URL: srv.URL + "/ok", Expect: 200, Contains: "ok"}, true, 200, ""},
		{"body check fails", Service{URL: srv.URL + "/ok", Expect: 200, Contains: "ready"}, false, 200, "body does not contain"},
		{"wrong status", Service{URL: srv.URL + "/unavailable", Expect: 200}, false, 503, "expected status 200, got 503"},
		{"redirect is not followed", Service{URL: srv.URL + "/redirect", Expect: 200}, false, 302, "got 302"},
		{"redirect can be the expectation", Service{URL: srv.URL + "/redirect", Expect: 302}, true, 302, ""},
		{"timeout", Service{URL: srv.URL + "/slow", Expect: 200}, false, 0, "deadline"},
		{"sends a user agent", Service{URL: srv.URL + "/ua", Expect: 200}, true, 200, ""},
		{"connection refused", Service{URL: "http://127.0.0.1:1/", Expect: 200}, false, 0, "refused"},
		{"bad url", Service{URL: "http://[::1", Expect: 200}, false, 0, "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.svc.ID = "x"
			r := Check(context.Background(), client, tt.svc)
			if r.OK != tt.ok || r.Code != tt.code || !strings.Contains(r.Error, tt.errPart) {
				t.Errorf("result = %+v, want ok=%v code=%d error containing %q", r, tt.ok, tt.code, tt.errPart)
			}
			if r.Service != "x" {
				t.Errorf("Service = %q", r.Service)
			}
		})
	}
}

func TestProbeKeepsServiceOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/b" {
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()
	svcs := []Service{
		{ID: "a", URL: srv.URL + "/a", Expect: 200},
		{ID: "b", URL: srv.URL + "/b", Expect: 200},
		{ID: "c", URL: srv.URL + "/c", Expect: 404},
	}
	run := Probe(context.Background(), NewClient(), svcs, at("2026-11-04T10:00:00.7+02:00"))
	if !run.Time.Equal(at("2026-11-04T08:00:00Z")) {
		t.Errorf("Time = %v, want UTC truncated to the second", run.Time)
	}
	for i, id := range []string{"a", "b", "c"} {
		if run.Results[i].Service != id {
			t.Errorf("result %d is %s, want %s", i, run.Results[i].Service, id)
		}
	}
	if !run.Results[0].OK || !run.Results[1].OK || run.Results[2].OK {
		t.Errorf("results = %+v", run.Results)
	}
	if run.Results[1].Ms < 50 {
		t.Errorf("Ms = %d, want at least 50", run.Results[1].Ms)
	}
}

func TestServicesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Services {
		if s.ID == "" || s.Site == "" || s.Name == "" || s.Expect == 0 || !strings.HasPrefix(s.URL, "https://") {
			t.Errorf("incomplete service %+v", s)
		}
		if seen[s.ID] {
			t.Errorf("duplicate id %s", s.ID)
		}
		seen[s.ID] = true
	}
}
