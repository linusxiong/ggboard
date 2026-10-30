# Dependency security review — 2026-10-30

Scope: `main` at `a097e72`, Go standard library and modules, the complete pnpm lockfile (including development
tools), and the Linux amd64 runtime container built from `deploy/Dockerfile`. This is a published-advisory review,
not a new line-by-line application audit or a scan of a production host. No production data or credentials were used.

## CVE-2026-98432: host action required

The [Linux CNA record](https://github.com/CVEProject/cvelistV5/blob/main/cves/2026/98xxx/CVE-2026-98432.json)
describes an IPv4/TCP use-after-free in the Linux kernel, not in Go, React or GGBoard. A deployment on an affected
kernel remains exposed, including through Docker and a reverse proxy: those do not replace the host's network stack.

The record lists upstream fixes in 5.10.272, 5.15.223, 6.1.190, 6.6.159, 6.12.113, 6.18.56 and 7.2.10,
and mainline 7.3. Use a supported distribution kernel containing the fix, reboot, then confirm the running kernel
with `uname -r`. Vendor backports must be checked against the vendor advisory; comparing version strings is insufficient.
The production kernel was not available for inspection. The repository changes below do **not** remediate this CVE
on an unpatched host. Operator guidance is in [SECURITY.md](../SECURITY.md#host-kernel-and-dependency-updates).

## npm findings and fixes

Initial `pnpm -C web audit --json`: **8 advisories** (6 high, 1 moderate, 1 low), all through `@lhci/cli`.
These are development-tool dependencies, not JavaScript shipped to panel users. Version matches do not prove that
GGBoard's particular tooling invocation reaches every vulnerable path; the affected versions were removed anyway.

| Dependency before | Advisory | Resolution |
| --- | --- | --- |
| `tmp` 0.0.33 / 0.1.0 | [CVE-2025-54798](https://github.com/advisories/GHSA-52f5-9888-hmc6), [CVE-2026-44705](https://github.com/advisories/GHSA-ph9p-34f9-6g65) | Override to 0.2.7 |
| `uuid` 8.3.2 | [CVE-2026-41907](https://github.com/advisories/GHSA-w5hq-g745-h8pq) | Override LHCI's dependency to 11.1.1, retaining its CommonJS API |
| `extract-zip` 2.0.1 | [CVE-2026-56876](https://github.com/advisories/GHSA-jmr9-qjv8-65gv), [CVE-2026-19693](https://github.com/advisories/GHSA-7pqw-9j4j-h8q3) | No published fix; remove the dependency by updating Puppeteer to 25.16.0 / browsers 3.2.6 |
| `zod` 3.25.76 | [CVE-2023-54404](https://github.com/advisories/GHSA-mrhx-c7xc-m3w5) | Remove the old Chromium BiDi dependency chain via the same Puppeteer update |
| `basic-ftp` 5.3.1 | [CVE-2026-102990](https://github.com/advisories/GHSA-c475-qrg2-pj4r) | Override `get-uri`'s dependency to 6.2.3 |
| `body-parser` 1.20.8 | [CVE-2026-111181](https://github.com/advisories/GHSA-27r3-jxmh-p828) | Override to 1.20.9 |

`web/pnpm-workspace.yaml` records the overrides. A package extension supplies browsers 3.2.6 with its own
compatible `proxy-agent` 8.0.2; LHCI retains its CommonJS 6.x agent. Lighthouse remains 12.6.1, so this change does
not replace the scoring engine. Final audit: **0 advisories** across 687 dependencies; `pnpm peers check`: no issues.
No advisory is ignored or filtered out. CI now audits development dependencies as well as application dependencies.

## Go and runtime image

- Go 1.27.3, `govulncheck` 1.9.0, current Go vulnerability database: **0 reachable vulnerabilities, 0 vulnerable
  imported packages**. `GO-2026-5932` remains a module-level notice for
  [unmaintained `golang.org/x/crypto/openpgp`](https://pkg.go.dev/vuln/GO-2026-5932). That package is not imported;
  there is no fixed version. Removing `x/crypto` would remove the password hashing dependencies the application needs.
- Trivy 0.76.1 found four Debian advisories in `static-debian12:nonroot`: `DLA-4845-1` for `ca-certificates` and
  `DLA-4792-1`, `DLA-4854-1`, `DLA-4867-1` for `tzdata`. These are package advisories, not four additional CVEs.
  [Debian's certificate advisory](https://www.debian.org/lts/security/2026/dla-4845) removes trust affected by the
  eMudhra intermediate-CA compromise.
- Switch the runtime to `gcr.io/distroless/static-debian13:nonroot`, retaining the static Go binary, UID 65532,
  `/data`, entrypoint and ports. The scanned base digest is
  `sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99`.
  Its certificate package is `20250419+deb13u1`. Trivy reports **0 OS-package findings** in the base and final
  application image. The final binary has only the unreachable OpenPGP module notice described above.
- CI now runs `govulncheck` on source. Rebuild and redeploy to consume the new image; existing deployed images
  do not change when this commit is pushed.

## Validation

- Frozen pnpm installation, peer checks and full npm audit passed.
- Go full race suite (`make test`) and `go vet ./...` passed.
- Frontend type checking, ESLint and **387 tests in 68 files** passed. The first unrestricted run had four
  5-second timeouts during concurrent builds; the complete rerun with `--maxWorkers=2` passed without changing
  assertions or timeout limits.
- Clean Docker build passed, including all four Chromium-prerendered public pages. The final runtime image was scanned.
- Desktop and mobile Lighthouse autorun passed all existing 100/100/100/100 assertions for all four public pages
  (one run per URL for dependency compatibility).
- The final nonroot runtime container passed `/healthz` and the full API E2E suite (**29 groups**), including
  prerendered pages, subscriptions, payments, TOTP and account/session flows; its temporary data was discarded.

Limits: production kernel, deployed image digest, external database/reverse-proxy packages and other architectures
were not inspected. Database-backed scans and static reachability checks do not rule out unknown vulnerabilities.
