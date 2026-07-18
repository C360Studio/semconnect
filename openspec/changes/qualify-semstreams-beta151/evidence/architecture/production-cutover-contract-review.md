# Superseded production contract review

This file previously described a retained-state migration and destructive cutover. The product owner superseded that
model on 2026-07-18: semconnect is pre-v1 and its first production deployment starts on a clean NATS volume.

The active production contract is the greenfield amendment in ADR-S003 and the current beta.151 OpenSpec design,
specification, and tasks. It requires one Compose bundle, typed empty-NATS preflight, canonical collection/item
readiness, normal restart persistence, and product-owner/operator decisions over the task 6.3 manifest. It contains no
migration, deletion, translation, old-state inventory, maintenance-window, or old-deployment rollback requirement.

This note preserves why the earlier review is absent from final evidence. It is not an approval or production gate.
