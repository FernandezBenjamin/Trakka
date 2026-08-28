# Summary

<!-- Briefly explain what this PR does and why. -->

## Type of change

- [ ] 🐛 Fix (bug fix)
- [ ] ✨ Feature (new functionality)
- [ ] ♻️ Refactoring (no behavior change)
- [ ] 📚 Docs (documentation only)
- [ ] ⚙️ CI/CD (pipeline, workflows, tooling)

## Linked issues

<!-- e.g. Fixes #123 / Relates to #456 -->

## Validation checklist

- [ ] `go build -o trakka ./cmd/server` succeeds
- [ ] `go vet ./...` is clean
- [ ] `gofmt -l .` returns nothing
- [ ] `go test ./...` passes (including new tests for this change, if applicable)
- [ ] `golangci-lint run ./...` is clean (config: `.golangci.yml`)
- [ ] `CLAUDE.md` updated if this PR changes a convention, a security rule, or architecture it documents
- [ ] PWA/offline behavior verified if any `static/js/*.js` or `static/sw.js` files were touched (see [docs/PWA.md](../docs/PWA.md))
- [ ] No new SQL/XSS/SSRF vulnerability introduced (parameterized queries, `textContent`/`createElement` only, URL validation — see the "Security rules" section of `CLAUDE.md`)

## Additional notes

<!-- Screenshots, reviewer notes, known limitations, etc. -->
