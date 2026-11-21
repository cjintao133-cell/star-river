# OpenSSF Best Practices — Silver 逐条对照清单

> 用途：在 https://www.bestpractices.dev 提交 Silver 自评时，每条对照本表填 "Met + URL/路径 + 一句理由"。
> 基线：Passing 徽章已在（持续 maintain）。下表为 Silver 增量项。

| 准则 (必须=MUST) | 状态 | 证据（仓库文件 / 链接） |
|---|---|---|
| achieve_passing（已有 passing） | Met | passing 徽章页 |
| contribution_requirements | Met | CONTRIBUTING.md（编码规范 + per-commit gate） |
| governance | Met | GOVERNANCE.md §Decision model |
| code_of_conduct | Met | CODE_OF_CONDUCT.md |
| roles_responsibilities | Met | GOVERNANCE.md §Roles and responsibilities |
| access_continuity | Met | GOVERNANCE.md §Access continuity (bus-factor mitigation) |
| bus_factor（SHOULD≥2） | 部分 | GOVERNANCE.md §Bus factor（当前=1，已给缓解方案） |
| documentation_roadmap | Met | ROADMAP.md |
| documentation_architecture | Met | docs/architecture.md |
| documentation_security | Met | docs/security-requirements.md |
| documentation_quick_start | Met | docs/usage.md |
| documentation_current | Met | docs/* 与 README 持续同步 |
| documentation_achievements | Met | README.md 顶部徽章区（含本徽章） |
| maintenance_or_update | Met | docs/upgrade-policy.md |
| report_tracker | Met | GitHub Issues |
| vulnerability_report_credit | N/A | 近 12 个月无已修复的已知漏洞；策略见 SECURITY.md |
| vulnerability_response_process | Met | SECURITY.md（披露流程/时限） |
| coding_standards | Met | CONTRIBUTING.md「Coding standards (explicit)」 |
| coding_standards_enforced | Met | CI：gofmt + staticcheck（.github/workflows/ci.yml） |
| build_standard_variables（CC/CFLAGS/LDFLAGS） | Met | native/Makefile（尊重外部 CC/CFLAGS/LDFLAGS） |
| build_repeatable（逐位可复现） | Met | goreleaser：-trimpath / -buildid= / mod_timestamp；C：固定工具链 |
| installation_common | Met | native/Makefile: install / uninstall |
| installation_standard_variables（DESTDIR） | Met | Makefile: PREFIX/BINDIR/DESTDIR |
| installation_development_quick | Met | CONTRIBUTING.md（一条命令装全 + 跑门禁） |
| external_dependencies | Met | go.mod / go.sum（机器可读） |
| dependency_monitoring | Met | CI govulncheck + Dependabot |
| updateable_reused_components | Met | Go modules |
| automated_integration_testing | Met | CI：每次 push/PR 跑 go test |
| regression_tests_added50 | Met/N-A | 政策见 CONTRIBUTING「Test policy」；近 6 月 bug 少 |
| test_statement_coverage80 | Met | CI 覆盖率门禁，实测总覆盖率 81.4% ≥ 80% |
| test_policy_mandated | Met | CONTRIBUTING.md「Test policy (mandatory)」 |
| tests_documented_added | Met | CONTRIBUTING.md（change proposal 要求加测试） |
| warnings_strict | Met | C: -Wall -Wextra -Werror；Go: vet + staticcheck |
| implement_secure_design | Met | docs/architecture.md + docs/security-requirements.md |
| crypto_weaknesses | Met | AES-256-GCM（kernel/store），无 SHA-1/弱算法 |
| crypto_credential_agility | Met | SecretStore/Keystore，密钥独立存放、可轮换不重编译 |
| crypto_used_network | Met | 出网统一走 kernel/netx，TLS 1.2+，不跟随重定向 |
| crypto_tls12 | Met | Go net/http，TLS 1.2+，默认验证证书 |

## 还差 / 要在网站上填的
- **report_url**：填漏洞私有报告地址 `https://github.com/cjintao133-cell/star-river/security/advisories/new`（或 SECURITY 页）。
- **documentation_achievements**：拿到 Silver 后 48h 内把 Silver 徽章加到 README 顶部。
- **签名发布（signed_releases / Sigstore）**：建议下一步加 Sigstore keyless 签名，利 Gold 与 SLSA。

## 下一步（更高级、单人可达）
- SLSA Build L3（GitHub Actions provenance）
- Sigstore 签名 + SBOM（SPDX/CycloneDX）
- Scorecard 冲 9+（分支保护、依赖锁定、Token 权限）
