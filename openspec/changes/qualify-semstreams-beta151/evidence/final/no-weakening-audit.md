# No-weakening audit

The accepted comparison baseline is repository revision
`95d6a6161374f45465f92bd491ba5b64fed30572` plus the signed beta.149
qualification evidence. The authoritative beta.151 external run is
`2026-07-18T17-09-45Z`. The earlier `17-06-05Z` run is procedurally superseded
because it began before final task 5.4 approval.

## Technical-writer comparison

- ETS source remains clean at commit
  `d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4` and tree
  `875847e194ffdb2aef2b617c9a9a0d9add037331`.
- The fixture tree is unchanged at Git tree
  `bce56a34afe9e01874c6171203db8d785a28804b`.
- `gateway/cs-api/openapi.yaml` is unchanged at SHA-256
  `183d72922aca3079fae33d7c332b0c04212b0cc002169bb65d33bf0eaebcba90`.
- `gateway/cs-api/conformance.go` is unchanged at SHA-256
  `8d4345841e2e232af3fed6bc2fb7b0f1ae8d1b5426be94e0cdd261bdfb3319bd`.
- Compose configuration, workflow selection, product tests, and UI source are
  unchanged.
- The beta.151 harness change strengthens source authority: it requires exact
  tag/commit identity, rejects dirty or unreadable vendors, and refreshes only
  validated narrow paths. It does not alter mutation policy, test selection,
  skip behavior, or result accounting.
- The isolated TestNG parser function is byte-identical to the accepted
  baseline at SHA-256
  `1bd6b360cd46803af7cd8984ee5e54d56c3c4460c4c84ae7b0355163af4e08ac`.
- New tests cover pin/source identity and structural/trusted-RMW behavior; no
  accepted product test was deleted or relaxed.
- Signed beta.147 and beta.149 evidence manifests verify unchanged.
- Frontend/Svelte is not applicable because no UI or public CS API contract
  changed.

## Raw result and independent review

The raw TestNG report resolves to root attributes `137/137/0/0`, with 137
non-configuration passes, zero failures, zero skips, and 23 configuration
passes. The reviewer independently proved that all 137 test names and all
configuration names exactly match the accepted beta.149 run.

The final reviewer attestation is `../review/go/review.json`, SHA-256
`e7feab916ec22e84dc9073b90fae227777491d57c5f44b6c69c47ed9c24b78f9`.
It approves task 6.2 and records `externalAuthorityWeakening=false`.

Result: beta.151 achieved exactly `137 passed, 0 failed, 0 skipped` without
reducing test authority, scope, fixtures, claims, or result accounting. This is
candidate qualification, not production authorization.
