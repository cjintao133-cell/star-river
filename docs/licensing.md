# License

## In short
- **The whole repository is under [Apache License 2.0](../LICENSE).** This includes every `kernel/*` package,
  `native/`, `cmd/`, `scripts/`, `.github/`, and the memory/emotion/proactivity "capability plugins"
  (`kernel/hippocampus`, `kernel/endocrine`, `kernel/proactive`).
- There is **no "closed-source area"**. The earlier "three-package closed" plan is **abandoned**; everything is open.
- Apache-2.0 includes a patent grant, trademark clause, and attribution requirements — ecosystem-friendly and
  adoptable by enterprises.

## Why fully open source
- Forgetting curves, emotion dimensions, vector recall, etc. are public knowledge; keeping them closed gives no real
  moat and only discourages adoption and contribution.
- A framework's value is in being **adopted, trusted, and sustainably maintained**, not in hiding a snippet of code.

## Layering constraint (engineering rule, not licensing)
Although fully open, we **keep the dependency-direction constraint** (clean source + pluggability):
- **Core packages (`kernel/bus`, `kernel/adapter`, `kernel/store`, `kernel/orchestrator`, `kernel/tool`,
  `kernel/core/*`, etc.) must not import capability-plugin packages** (`hippocampus` / `endocrine` / `proactive`).
- Capability plugins may import the core.
- Enforced continuously by `scripts/check-open-split.sh` (the core compiles standalone and doesn't depend on plugins).

## Contributions
Under Apache-2.0, contributions are licensed under the same terms by default (inbound = outbound).
