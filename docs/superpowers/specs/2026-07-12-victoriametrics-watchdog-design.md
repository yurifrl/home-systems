# VictoriaMetrics Watchdog Design

## Goal

Detect VictoriaMetrics storage and ingestion failures without depending on VictoriaMetrics rule evaluation, while reusing Alertmanager for Discord delivery and deduplication.

## Architecture

Add one Kubernetes `CronJob` at `k8s/charts/support-cluster/templates/monitoring/victoriametrics-watchdog.yaml`, deployed in the `monitoring` namespace. Every minute it checks `vmsingle` and `vmagent` directly, then posts alerts to the existing Alertmanager v2 API at `vmalertmanager-vmks:9093/api/v2/alerts`.

The watchdog uses an existing curl image and an inline shell script. It adds no chart, custom image, ConfigMap, RBAC, persistence, or secret.

External Gatus remains responsible for detecting failures of Alertmanager or the in-cluster watchdog itself.

## Alerts

The watchdog manages these conditions:

- Warning when VictoriaMetrics free storage is below 15%.
- Critical when free storage is below 5%.
- Critical when `vm_storage_is_read_only` equals 1.
- Critical when the `vmsingle` endpoint is unavailable.
- Critical when vmagent remote-write is failing.

Alerts carry `environment=production` and use Alertmanager's existing Discord receiver. Each run refreshes active alerts with a future `endsAt`; healthy checks send an expired `endsAt` so Alertmanager resolves them. This keeps the job stateless and lets Alertmanager handle grouping and repeat notifications.

## Failure Behavior

A failed check must produce a critical alert rather than be treated as zero usage. Network and parsing failures use a bounded curl timeout so jobs cannot overlap indefinitely. CronJob concurrency is forbidden.

If Alertmanager is unavailable, the job fails visibly in Kubernetes; external Gatus is the independent notification path for that failure domain.

## Verification

Render the Helm chart and verify the CronJob manifest. Run the check script against controlled metric fixtures for healthy, warning, critical, read-only, unavailable, remote-write failure, and recovery cases. In-cluster verification must confirm Alertmanager receives and resolves the watchdog alerts without duplicate Discord notifications.
