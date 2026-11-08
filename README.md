# Status

The source of [changkun.github.io/status](https://changkun.github.io/status/): the current state and 90-day history of changkun.de, golang.design and poly.red, component by component.

It runs on GitHub, not on the server it checks, so it keeps working when that server is down. A scheduled workflow probes every component from GitHub's runners every five minutes, records the results, opens a GitHub issue for each outage, and publishes a static page to GitHub Pages.

## What it checks

Each component is one HTTPS GET with a 10 second timeout and no redirects followed. A check passes when the status code matches and, if set, the body contains the expected text. The page groups components by site and shows each one's state, its 90-day bars and its last response time. The list is in [`internal/status/services.go`](internal/status/services.go).

| Site | Components |
| --- | --- |
| changkun.de | Website, Blog, Research, Modern C++, Library, Ideas API, Short links, Go imports, urlstat, Midgard |
| golang.design | Website, Go under the hood, Go questions, Go history, Research, Miner, Short links, Go imports, SSA playground |
| poly.red | Go import, RankUI study |

The short-link checks read redir's data API rather than the link page, so a check is not counted as a visit.

Two checks in a row must fail before a component shows as down. A check that passes but takes longer than 3 seconds is slow, and two slow checks in a row show a component as degraded. A single bad check is a blip and is not counted. A day's bar is red when uptime was under 99%, amber when it had any confirmed down or slow checks, and green otherwise. A site shows the state of its worst component.

## Alerts

When a component is down, the workflow opens an issue labelled `outage`, titled for example "golang.design: SSA playground is down", with the URL, when the outage began, and what the last check found. When the checks pass again, it closes the issue with a comment. GitHub sends the usual issue notifications, so watching the repository is what subscribes you to alerts.

Each alert issue ends with a hidden marker naming its component, and the workflow keeps exactly one open issue per component that is down. Other issues are left alone. An outage takes two checks five minutes apart to confirm, and GitHub may start a scheduled run late, so an alert arrives about 10 to 20 minutes after a component goes down.

## Add a component

Add one line to `Services` in `internal/status/services.go`: an `ID` (stable, never renamed, it keys the history and the alert issues), the `Site` it belongs to, a `Name` for the page, the `URL`, the `Expect`ed status code, and an optional `Contains` body check. Use only endpoints that answer without credentials and do not count a check as a visit. Run `go test ./...`. Removing a component closes its open alert issue on the next run.

## Write an incident

Add a file to `incidents/`, for example `incidents/2026-11-04-blog-errors.md`, and push it to `main`. Copy `incidents/_template.md`. Files that start with `_` and `README.md` are ignored.

```markdown
---
title: The blog returns errors
components: [blog]
started: 2026-11-04T10:00:00Z
resolved:
updates:
  - time: 2026-11-04T10:10:00Z
    status: Investigating
    text: We are looking into it.
---
Some posts fail to load.
```

`components` are the IDs from `services.go`. Leave `resolved` empty while the incident is open and set it when it ends. Open incidents show at the top of the page. Resolved ones show for 90 days. Times are UTC. A file with a mistake fails the build with its name, and the page keeps showing the last good version until it is fixed.

## How it works

`.github/workflows/status.yml` runs on a five minute schedule (GitHub may delay it), on manual dispatch, and on a push to `main` that changes incidents, code or scripts.

1. `update` job: checks out the `history` branch beside `main`, runs `status probe` through `scripts/commit-history.sh`, then runs `status build` and uploads `site/` as the Pages artifact. It has `contents: write` only.
2. `alert` job: reads the history just pushed. `scripts/alert.sh` lists the open `outage` issues, asks `status alerts` which to open and close, and applies that with `gh`. It has `contents: read` and `issues: write` only, and runs beside the deploy, so a GitHub API error never holds back the page.
3. `deploy` job: publishes the artifact with `actions/deploy-pages`. It has `pages: write` and `id-token: write` only, and no `environment:`, so a publish every few minutes does not record a GitHub deployment or notify deployment subscribers.

History is one file per UTC day, `YYYY-MM-DD.json`, on the `history` branch, so `main` stays clean. `probe` drops files older than 90 days. If a push is rejected because another run pushed first, `commit-history.sh` rebases and tries again. If the rebase conflicts, it takes the remote branch as it is and probes again. It never force-pushes. The branch's git history still holds old days, so it grows slowly; squash it by hand if it ever matters.

`status build` renders `index.html` from history and incidents with `html/template`, and copies `style.css` and the fonts (Inter and Instrument Serif, both under the SIL Open Font License) from `internal/status/web/`. The page loads nothing from other hosts.

### Set up once

In the repository settings, set Pages to deploy from GitHub Actions. The page is served at `changkun.github.io/status/`; it links its style and fonts relatively, so a custom domain needs only the Pages setting, a DNS record, and `PageURL` in `internal/status/alerts.go`.

GitHub turns off scheduled workflows in a public repository after 60 days without activity, and emails a warning first. If that happens, enable the workflow again under Actions.

## Develop

```sh
go test ./...
go run ./cmd/status probe -history /tmp/history
go run ./cmd/status build -history testdata/history -incidents testdata/incidents -out /tmp/site -now 2026-11-04T12:00:00Z
scripts/commit-history_test.sh
scripts/alert_test.sh
```

After a deliberate change to the page, run `go test ./internal/status -update` and review the diff of `testdata/index.golden.html`.

## License

Apache-2.0. Copyright 2026 Changkun Ou.
