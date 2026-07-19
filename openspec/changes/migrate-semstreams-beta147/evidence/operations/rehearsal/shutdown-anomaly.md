# Beta.147 shutdown anomaly

The disposable no-write restart was not log-clean. At
`2026-07-18T02:05:46.765135969Z`, SemStreams emitted `Service stop failed`
because the heartbeat service was already stopped. At
`2026-07-18T02:05:46.765205803Z`, it emitted `Error stopping services` with the
same cause. The process reported graceful shutdown failure.

Evidence:

- `conformance/output/semstreams-backend-restart-2026-07-18T02-03-23Z.log`
- SHA-256:
  `38426276d0bb63d43b34c13e850aa86236a05664a0164a041a2ae49b18a14d2b`
- Lines: `118`

The same artifact records successful backend restart/bootstrap:

- `All services started successfully` at `2026-07-18T02:05:47.111458053Z`;
- `graph-query coordinator started` at `2026-07-18T02:05:51.651297805Z`.

The post-restart revision probe reached `118/118`. Authoritative and retained
resource counts did not change, and all eleven normalized query hashes matched
their pre-restart values. The spatial derived index kept two messages and two
subjects; its last sequence advanced from 22 to 27 during rebuild.

This evidence supports replay parity but does not make the shutdown logs
clean. The anomaly remains a production no-go condition until it is triaged
and explicitly accepted or fixed through the normal review process.

The archive-strength replay repeated the condition at
`2026-07-18T02:14:58.296894377Z` and `2026-07-18T02:14:58.297004836Z`.
Its 118-line log is
`conformance/output/semstreams-backend-restart-archived-2026-07-18T02-03-23Z.log`
with SHA-256
`b905725bb93b78c2cb2b1b3316907b4be9ce499ddf99dead655f51ec4fe0e0f9`.
The repeated error strengthens the production blocker; it is not a transient
log-capture artifact.
