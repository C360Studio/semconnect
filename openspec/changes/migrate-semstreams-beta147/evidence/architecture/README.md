# Architecture evidence

This directory is the frozen phase-1 handoff for `migrate-semstreams-beta147`.

- `provenance-manifest.json` records all 55 files in the seven transferred trees from SemStreams commit
  `c8f0b92edf5ad5b491d5f4e81891bec817fae3cd`.
- `semantic-ledger.json` freezes all 32 graph corrections and the no-alias, no-dual-read disposition.
- `compatibility-matrix.md` freezes public behavior and its evidence gates.
- `cutover-manifest.schema.json` defines the immutable execution contract.
- `cutover-manifest.template.json` is intentionally non-destructive and P0-blocked.
- `task-contract.snapshot.json` signs normalized task definitions while excluding live checkbox progression.
- `architecture-handoff.json` signs the developer handoff over `evidence-checksums.sha256`.
- `architecture-remediation-handoff.json` signs the GO-REV-004 remediation for reviewer rereview.

The live `tasks.md` file is deliberately absent from `evidence-checksums.sha256`: its checkboxes are mutable execution
state. The validator normalizes only recognized task checkbox markers, then compares the complete remaining document
with the signed task-contract digest. Changes to task identifiers, owners, wording, commands, order, or handoffs fail;
ordinary `[ ]` to `[x]` progression does not.

Regenerate and validate provenance from an extracted checkout of the exact source commit:

```sh
architecture=openspec/changes/migrate-semstreams-beta147/evidence/architecture
"$architecture/generate-provenance-manifest.sh" /path/to/semstreams-c8f0b92 "$architecture/provenance-manifest.json"
"$architecture/validate-evidence.py" /path/to/semstreams-c8f0b92
openspec validate migrate-semstreams-beta147
```

The handoff approves implementation only. Production execution remains P0-blocked until a deployment-specific
manifest validates, becomes immutable, contains literal destructive scope and retained-state evidence, and carries
the required architect, product-owner, operator, and two-person destructive-review approvals.
