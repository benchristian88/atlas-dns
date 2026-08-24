# Operational Status UI

**Monitoring -> Operational Status** is available at the stable
`/system/operational-status` route. Monitoring owns its navigation because the
page is a read/observe experience answering whether Atlas DNS Controller and
its collectors are operating correctly; it is not AdGuard Home configuration
or an HA lifecycle command surface.

The page shows overall controller health, API/PostgreSQL status, independent
node observation freshness, per-node Statistics and Query Log health, known
ingestion gaps, process workers, PostgreSQL pool use, and approximate storage
and retention bounds. It uses existing badges, banners, loading/error states,
cards, and responsive data tables. Error codes are safe stable summaries.

Operational Status uses the Standard page class consistently with the other
Administration routes. Its diagnostic grids reflow and its node/subsystem
tables own contained horizontal scrolling instead of widening the page.

Dashboard contains only a compact controller/Statistics/Query Log summary and
links to the page. Statistics and Query Log retain their established coverage
presentations and semantics.

The page includes a compact HA summary and a distinct DNS Service table so
management API reachability and actual DNS answers cannot be conflated. Detailed
maintenance, certificate, version, notification, and upgrade actions live on HA
Operations rather than expanding this diagnostic page.

Scheduled node API and DNS evidence follows the persisted node-health cadence.
Successful DNS evidence remains current for the greater of three configured
intervals or two minutes; missing that deadline becomes `stale`, while an
explicit failed probe remains failed until a later probe succeeds. The same DNS
deadline is used by Dashboard, HA summary, Operational Status, and maintenance
preflight so normal long polling intervals do not create contradictory health.

Core Services uses the shared divided panel anatomy and semantic
`SummaryTileGrid`. API state, PostgreSQL state/ping, schema version,
and pool use remain the same information model, presented as four inset tiles
with the standard compact status badges and responsive two-to-one column flow.
