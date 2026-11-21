# Governance

Star River is a small, single-maintainer open-source project today. This document makes the
decision model, roles, and continuity plan explicit (required for the OpenSSF Best Practices
Silver badge).

## Decision model

- **Model:** lead-maintainer ("benevolent maintainer") with open discussion. Non-trivial changes
  are proposed as GitHub issues or pull requests and discussed in public.
- **Rough consensus:** the maintainer seeks agreement from active contributors. When consensus is
  not reached, the Project Lead makes the final call and records the rationale in the issue/PR.
- **Appeals:** any decision can be re-opened by a reasoned issue; decisions are not permanent.

## Roles and responsibilities

| Role | Who | Responsibilities |
|---|---|---|
| Project Lead / Maintainer | cjintao133-cell | Final decisions, releases/tags, repository and CI administration, security triage, roadmap. |
| Contributor | anyone | Proposes and implements changes via PR, adds tests, follows CONTRIBUTING.md, signs off (DCO). |
| Security Reviewer (planned, external) | TBD | Independent review of crypto / cgo / signing code before any "audited" claim (see SECURITY.md). |
| User | anyone | Reports bugs and vulnerabilities through the documented channels. |

Roles are currently all held by the Lead. Additional maintainers may be added as contributors
demonstrate sustained, high-quality participation; this doc is updated when that happens.

## Contribution requirements

See [CONTRIBUTING.md](./CONTRIBUTING.md). All contributions are accepted under Apache-2.0
(inbound = outbound) and must be signed off per the Developer Certificate of Origin (DCO).

## Code of conduct

This project follows [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md). It is enforced by the Project Lead.

## Access continuity (bus-factor mitigation)

The project is designed to survive the loss of any single person:

- **Repository and release rights:** the source is public and mirrored (GitHub). Releases are
  reproducible from tagged source (see docs/upgrade-policy.md), so the community can fork and
  continue at any time.
- **Credentials:** recovery keys and account recovery contacts are kept in an offline lockbox with
  instructions so a designated successor can take over DNS, package, and repository administration
  if the Lead becomes unavailable. A fork of the repository remains fully buildable without any
  secrets (no proprietary build steps).
- **Goal:** create/close issues, accept changes, and cut a release within one week using only
  public source plus lockbox-held credentials.

## Bus factor

Current bus factor is **1**. The mitigation above (public, reproducible source + lockbox custody)
is the documented plan to keep the project recoverable. Growing the maintainer set is a roadmap goal.
