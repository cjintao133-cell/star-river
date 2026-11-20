# Launch checklist — getting seen & trusted

Two different goals, two different levers. Do **A** to be *found*, **B** to be *trusted* once found.

---

## A. Be found (distribution)

### 1. GitHub repo setup (see `docs/release-process` for the push)
- **Description** (one line, English): `Embeddable AI-assistant kernel in Go+C: context assembly, memory, emotion, tools/MCP, multimodal, encrypted local storage. BYOK, zero backend.`
- **Topics** (Settings → Topics) — pick ~10:
  `ai`, `llm`, `golang`, `local-first`, `on-device`, `memory`, `ai-agents`, `mcp`, `embeddable`, `privacy`
- **Website**: (optional) a landing page or your profile.

### 2. Awesome-list submissions (the real discovery path)
Open a PR to add the repo under the right section of each (read each list's CONTRIBUTING first):
- `avelino/awesome-go` (section: AI / Machine Learning)
- `e2b-dev/awesome-ai-agents` or `Jenqyang/Awesome-AI-Agents`
- `yzfly/Awesome-MCP-ZH` and `punkpeye/awesome-mcp-servers` (if exposing MCP)
- `SeraphimSerapis/awesome-local-ai`
- `caramaschiHG/awesome-ai-agents-2026`
- `awesome-selfhosted` (if it fits their criteria)

> Tips: one-line neutral description, no hype; link the `hello-kernel` demo so reviewers can run it in 5s.

### 3. Launch posts (1 good post > 100 badges)
- **Hacker News**: "Show HN: Star River Microkernel – embeddable AI-assistant kernel (Go+C, local-first)" — post the 5s demo + honest boundaries.
- **Reddit**: r/LocalLLaMA, r/golang, r/SideProject, r/opensource.
- **V2EX / 即刻 / 少数派** (中文圈).
- Lead with the **demo**, then the **honest positioning** ("not a runtime, not a memory add-on — an embeddable kernel"), then ask for feedback.

### 4. Make the "first 3 seconds" count
- The README top: one line + the 5s demo. (Done.)
- A short GIF of `go run ./examples/hello-kernel` is worth more than a feature list.

---

## B. Be trusted (OpenSSF / badges)

### Already in place (this repo)
- [x] `LICENSE` (Apache-2.0), `README`, `CONTRIBUTING`, `CODE_OF_CONDUCT`, `SECURITY.md`
- [x] Issue/PR templates, `dependabot.yml`
- [x] CI (`ci.yml`): gofmt, vet, build, test, `-race`, layering guard, **govulncheck**, staticcheck, demo smoke
- [x] CodeQL (`codeql.yml`), OpenSSF Scorecard (`scorecard.yml`)
- [x] C hot paths have ASan/Valgrind validation design (see `native/`)

### You must do (needs your account / repo live)
1. **Create the public repo & push** (see release process), so badges can link to a live URL.
2. **GitHub settings**: enable **Private vulnerability reporting**, **Dependabot alerts**, **branch protection** on `main`
   (require PR + status checks), and **signed tags/releases**.
3. **OpenSSF Best Practices Badge**: register at https://www.bestpractices.dev and fill in — start with **Baseline-1**,
   then pursue **Passing**. Every criterion is self-certified with evidence URLs; the repo files above cover most of it.
4. **Scorecard badge**: after `scorecard.yml` runs once, add the badge to README.

### Honest limits (do not overclaim)
- The badge is **self-certified best-practices compliance**, **not a security audit**.
- The 8 crypto/JNI/vector areas in `KNOWN-ISSUES.md` still need **external human review** — no self-serve badge replaces that.
- Say "follows OpenSSF best practices + govulncheck/ASan clean", **never** "security-certified/audited".
