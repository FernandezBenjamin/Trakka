# Contributing to Trakka

Thanks for your interest in Trakka! This guide summarizes the process for proposing a
contribution. For everything else (detailed architecture, conventions, security rules), refer to
[CLAUDE.md](../CLAUDE.md) at the root of the repository — it's the single, authoritative
reference for this project.

## Before you start

- Read [CLAUDE.md](../CLAUDE.md) in full, especially the "Architecture", "Security rules", and
  "Project conventions" sections.
- For a bug or feature idea, open an [issue](../../issues) first (see the available templates)
  before starting to code, especially for a substantial change — this avoids working in a
  direction that won't be accepted.
- This project aims to stay **ultra-lightweight** (< 20MB RAM at runtime) with **no unnecessary
  dependencies**: standard library Go wherever possible, no frontend dependency or build step.
  Every contribution must respect that goal.

## Process

1. **Fork** the repository, then clone your fork.
2. **Create a branch** dedicated to your change, based on `main`, with a clear name:
   `git checkout -b feat/feature-name` (or `fix/`, `docs/`, `chore/`...).
3. **Develop** while following the project's conventions:
   - Go backend: see "Backend (Go)" in CLAUDE.md (standard library first, SQL exclusively
     parameterized inside `internal/db`, the `internal/handlers` ↔ `database/sql` boundary, etc.).
   - PWA frontend: see "Frontend (PWA, `static/`)" in CLAUDE.md (vanilla JS, no build step, never
     `innerHTML` with interpolated data, any URL rendered as `<a href>` re-validated client-side).
   - Every rule listed under "Security rules" (SQL, URLs, JSON encoding, HTTP headers,
     passwords/sessions, CSRF, SSRF) is a non-negotiable constraint.
4. **Check locally** before pushing (see also the "Running the same checks locally" section of
   CLAUDE.md):
   ```bash
   gofmt -l .                 # must print nothing
   go vet ./...
   go build -o trakka ./cmd/server
   go test ./...
   golangci-lint run ./...    # config: .golangci.yml
   ```
5. **Commit** with a clear message describing *why* the change was made, not just *what* changed.
6. **Open a Pull Request** against `main`, filling in the
   [PR template](PULL_REQUEST_TEMPLATE.md) — summary, type of change, linked issue if applicable,
   and a checked validation checklist.
7. **CI must pass** (`lint`, `test`, `security-scan`, and `build-scan-push` on non-PR branches) —
   see the "CI/CD & Sécurité" section of CLAUDE.md for the details of each job.

## Updating CLAUDE.md

If your contribution changes an existing convention, adds a security rule, or modifies behavior
documented in CLAUDE.md (architecture, API, database schema...), update that file in the same PR
— it's the project's living documentation and shouldn't fall behind the code.

## Code of Conduct

By participating in this project, you agree to abide by the
[Code of Conduct](CODE_OF_CONDUCT.md).
