# No-weakening audit

The accepted comparison baseline is repository revision
`95d6a6161374f45465f92bd491ba5b64fed30572`, which contains the signed beta.147
migration. The qualifying candidate changes only the SemStreams dependency and
conformance backend pins, normalizes `go.sum`, and adds the executable
pin-alignment guard. Signed beta.147 evidence is unchanged.

## Independent comparison

The technical writer reproduced these comparisons:

- `conformance/run.sh`, `.github/workflows/conformance.yml`, and compose
  behavior: unchanged.
- `conformance/fixtures/`: unchanged; no fixture intent was relaxed.
- `gateway/cs-api/openapi.yaml`: unchanged at SHA-256
  `183d72922aca3079fae33d7c332b0c04212b0cc002169bb65d33bf0eaebcba90`.
- `gateway/cs-api/conformance.go`: unchanged at SHA-256
  `8d4345841e2e232af3fed6bc2fb7b0f1ae8d1b5426be94e0cdd261bdfb3319bd`.
- Existing tests: unchanged. The only new test file is
  `conformance/pin_alignment_test.go`, SHA-256
  `35a080307d8cf8b6dc598f681d40ae7ccd5e8be523e57f00c28e94b5cdd1c0ef`.
- ETS vendor: clean at commit
  `d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4`, tree
  `875847e194ffdb2aef2b617c9a9a0d9add037331`.
- No include/exclude group selector or skip flag was introduced.
- `ui/`: unchanged; there is no frontend behavior to weaken.
- `openspec/changes/migrate-semstreams-beta147/` and ADR-S003: unchanged.

The accepted harness hash remains
`5342d3d76cb5941fbf6fa69ea9ffd371df492df4f7344e26900cb11626a743a3`.
The beta.147 pin hash `299f70e95933f321a42d70db5c4e381df54db7eb7253c7368666c296f1495a47`
changes only as intended to the beta.149 pin hash
`bf219bfdb0845b01ed4c8f6429280f39d8b2e75a51c3dd42d64fb17bb54e3ba0`.

## Independent review

The Go reviewer separately repeated the ETS/vendor and source comparisons,
verified mutation tests remained enabled, and approved tasks 4.1 through 4.4.
The final reviewer attestation is
`../review/go/review.json`, SHA-256
`f57198e78e2bf750f1a3199c0e04a2e52eb550d874f9247fbd8726505700c1b6`.

Result: the exact `137 passed, 0 failed, 0 skipped` outcome was obtained without
changing the tested CS API behavior, its claims, its fixtures, or the suite's
selection logic. This proves candidate parity; it does not authorize production.
