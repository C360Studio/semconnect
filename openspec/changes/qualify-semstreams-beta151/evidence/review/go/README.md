# Go review: beta.151 independent qualification approved

The independent Go review approves tasks 5.1 through 5.4. Every executable SemStreams identity aligns to
`v1.0.0-beta.151`, annotated tag object `784f22dc8d549d7781b88a2878bb679112aad494`, peeled commit
`ac75c322140fb2a6b55759d07a79874b4cb4d9cc`, and source tree
`120eeb353afb7d07aa1b3180de05f75494bac1a8`. An exact remote query also confirmed the beta.149 and beta.150
provenance and returned no beta.515 ref.

All nine beta.149-to-beta.151 commits were independently enumerated and dispositioned. PR #554's structural gate
is live on semconnect create/update-with-triples. PR #567's trusted owner RMW is live on updates. Graph-index replay
is configured. PR #561's rule behavior and the changed rule, research, agentic, clustering, example, and tooling
producers are not configured or directly invoked. Shared agentic interfaces remain transitively compiled, but none
of the changed producers appears in the production dependency graph or executable configuration.

The review found and closed three evidence-integrity blockers. First, the vendor cleanliness helper swallowed a
failing `git status` and could misclassify an unverifiable checkout as clean; the corrected helper fails closed and
the corrupt-index regression proves the old failure mode. Second, the developer bundle algorithm was ambiguous and
initially unreproducible; the refreshed handoff now specifies its working directory, exact seven-file list, path
normalization, order, locale, exclusions, and command. The reviewer reproduced bundle `bb81bbba...` exactly.

The safe vendor refresh was reviewed at its destructive boundary for both build inputs. Each path accepts only its
exact harness-owned target under `conformance/.vendor`, clones to a sibling temporary directory, verifies the requested
clean commit, and only then replaces that narrow target. The SemStreams source is clean at its attested commit/tree.
The ETS source is clean at unchanged commit `d9caf33f...` and tree `875847e...`; its refresh preserves a real,
non-shallow Git checkout for Maven SCM metadata. No external run began before this hardening was reviewed.

The downstream live-NATS structural test passed three consecutive independent runs. Valid create/update and claimed
NoBirthStub routing persisted canonical state. Invalid predicate, entity-ID, and entity-reference updates returned
classified invalid errors without changing entity bytes, entity revision, or bucket revision. Resident poison
returned `graph_state_reset_required` without laundering or revision change, and remove-no-op performed no write.
Focused upstream tests additionally proved invalid-create and whole-batch atomic rejection, rejection metrics and
wire headers, every poison RMW lane, trusted decode semantics, rule persistence, graph-index restart parity, and
service lifecycle idempotency.

Unit, race, integration, default vet, integration vet, build, tidy-diff, module verification, shell syntax, strict
OpenSpec, and diff checks all pass. The ETS pin, fixture intent, OpenAPI, conformance claims, mutation flags, test
selection, and result parser are unchanged. No product source, compatibility alias, relaxed identity rule, dual
read/write, predicate rewrite, cleanup lane, or legacy branch was added.

The reviewer reproduced the operator manifest at 81 entries and SHA-256 `e40e3b52...c4db`; every entry validates,
and the operator handoff hashes to `dbff45e4...c2ff`. The build log, clean SemStreams and ETS source commits, source
tree, reviewed source hashes, image IDs, and container IDs all agree. The build occurred after the signed developer
handoff. The retained-state scanner reads the direct `KV_ENTITY_STATES` stream sequence span, fails closed on stream
or decoder errors, and uses beta.151 `graph.UnmarshalEntityState`. Independent arithmetic proves 67 retained
records across sequences 21 through 118, 31 gaps equal to JetStream's deleted count, 63 decoded PUT records, four
DEL tombstones, 33 subjects, 29 current PUT keys, no unknown operations, and zero violations. A new live scan after
the sealed handoff produced the same report except capture time.

TeamEngine and the cs-api writer stopped before backend SIGTERM at frozen revision 118. Docker metadata binds the
normal signal to the exact backend container/image; it exited zero in about 80 ms with `OOMKilled=false`, no Docker
error, no restart, and no forced kill. The bounded ten-line window ends in `SemStreams shutdown complete`, contains
no ERROR, and contains none of the beta.149 double-stop signatures. The filter does match the archived beta.149
failure, so zero matches on beta.151 are meaningful. Structural and foreign metric filters also match the exact
beta.151 source names; positive structural rejection comes from the signed isolated live-NATS proof rather than
log silence, and current retained edge sequence 113 proves the exercised hosted-child lane was claimed and survived
restart.

Pre/post entity state, observations, and ObjectStore inventories are exact. The spatial index changed only by its
expected restart rebuild revision while bytes, subjects, and both spatial API payloads remained equal. All twelve
normalized API payloads match byte-for-byte. Both archived readiness polls and a fresh reviewer poll reached stable
118/118 in two authoritative samples. Entity state remains at revision 118 in the still-running reviewer stack, so
no hidden graph write occurred between freeze, restart readiness, evidence sealing, and review.

This approval authorized the program manager to begin the fresh external qualification gate. The 16:40 TeamEngine
result (`137/0/0`) remains informative only. The first proposed final run, `2026-07-18T17-06-05Z`, also produced a
real `137/0/0`, but its volume and containers were created before task 5.4 was signed. The reviewer rejected that
run as final authority and required a new post-approval run rather than waiving the ordering gate.

The replacement run `2026-07-18T17-09-45Z` began after approval and created a new Compose network, NATS volume, and
four containers at 17:13:44Z. Its clean source is exact beta.151 commit `ac75c322...`, tree `120eeb35...`, with ETS
commit `d9caf33f...` and tree `875847e...`. The build exports bind the running backend, IUT, and TeamEngine images.
Graph-index readiness reached a stable 80/80 in two active samples, and the foreign-edge bake passed with one real
hosted-child lane, zero unclaimed, and zero dropped.

Independent XML parsing counted 137 unique non-configuration methods: 137 PASS, zero FAIL, zero SKIP, and zero
unknown statuses. The 137 test names and 23 configuration-method names exactly match the accepted beta.149 report.
The ETS tree and pin are unchanged and clean; fixtures, OpenAPI, conformance declarations, Compose configuration,
and workflow are byte-identical to the accepted baseline. The harness from `# Seed CS-API fixtures` through result
parsing and exit behavior is byte-identical. The only earlier harness changes are the intended beta.151 pin checks
and fail-closed clean-vendor materialization. Mutation tests remain explicitly enabled, no include/exclude or skip
selector exists, and all report/log hashes are recorded in the signed JSON review.

Task 6.2 is approved without weakening. The immutable cutover manifest and named architect, product-owner, and
operator approvals remain open. Production remains unauthorized.
