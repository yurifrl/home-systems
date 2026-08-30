#!/usr/bin/env python3
"""Subscribes to the billing-budget Pub/Sub topic and polls the Cloud Billing
API for the project's billing-attachment state, exposing both as Prometheus
gauges. Pull-based: no inbound exposure, no cluster dependency for the
independent Discord alert (that path is the GCP function).

Gauges:
  gcp_billing_cost{budget,currency}     current spend
  gcp_billing_amount{budget,currency}   budget limit
  gcp_billing_attached{project}         1 if a billing account is linked

Spend keeps rising to the month-to-date peak even after the kill switch
detaches billing, so threshold alerts on cost alone would page forever; they
are gated on gcp_billing_attached to silence once billing is detached.
"""
import base64
import json
import os
import threading
import time

from google.cloud import pubsub_v1
from prometheus_client import Gauge, start_http_server

PROJECT = os.environ["GCP_PROJECT"]
SUBSCRIPTION = os.environ["PUBSUB_SUBSCRIPTION"]
PORT = int(os.environ.get("PORT", "9109"))
POLL_INTERVAL = int(os.environ.get("BILLING_ATTACHED_POLL_SECONDS", "300"))

COST = Gauge("gcp_billing_cost", "Current spend for a budget", ["budget", "currency"])
LIMIT = Gauge("gcp_billing_amount", "Budget limit", ["budget", "currency"])
ATTACHED = Gauge(
    "gcp_billing_attached",
    "1 if a billing account is linked to the project, 0 otherwise",
    ["project"],
)


def handle(message):
    try:
        n = json.loads(message.data)
    except json.JSONDecodeError:
        # Budget payload is JSON in message.data; the base64 wrapper only exists
        # in the push envelope, not the pull client (already decoded).
        n = json.loads(base64.b64decode(message.data))
    name = n.get("budgetDisplayName", "unknown")
    ccy = n.get("currencyCode", "")
    COST.labels(name, ccy).set(float(n.get("costAmount", 0)))
    LIMIT.labels(name, ccy).set(float(n.get("budgetAmount", 0)))
    message.ack()


def poll_attached():
    """Refresh gcp_billing_attached from the Cloud Billing API.

    Uses the same service account as the Pub/Sub pull; the SA needs
    roles/billing.projectManager on the project (billing.projects.get). The
    role is grantable at project level and stays effective when billing is
    detached, which is exactly the state this gauge must observe.

    AuthorizedSession refreshes the token transparently per request. On
    failure the last known gauge value is kept: alert gating tolerates a
    stale observation far better than flapping. The gauge is only written on
    a successful observation, so a hard-broken exporter leaves it absent and
    the alerts keep firing (conservative default).
    """
    # Imported here: google-auth ships with google-cloud-pubsub, and auth
    # failure must never take down the subscriber thread.
    from google.auth import default
    from google.auth.transport.requests import AuthorizedSession

    session = None
    while session is None:
        try:
            creds, _ = default(
                scopes=["https://www.googleapis.com/auth/cloud-platform"]
            )
            session = AuthorizedSession(creds)
        except Exception as e:  # noqa: BLE001
            print(f"billing attached auth failed: {e}", flush=True)
            time.sleep(POLL_INTERVAL)

    url = f"https://cloudbilling.googleapis.com/v1/projects/{PROJECT}/billingInfo"
    while True:
        try:
            r = session.get(url, timeout=30)
            r.raise_for_status()
            attached = bool(r.json().get("billingEnabled"))
            ATTACHED.labels(PROJECT).set(1 if attached else 0)
            print(f"billing attached={attached}", flush=True)
        except Exception as e:  # noqa: BLE001
            print(f"billing attached poll failed: {e}", flush=True)
        time.sleep(POLL_INTERVAL)


def main():
    start_http_server(PORT)
    # Refresh the attached state in the background so the gauge exists at the
    # first scrape (absent would make alert expressions unknown, not 0).
    threading.Thread(target=poll_attached, daemon=True).start()
    sub = pubsub_v1.SubscriberClient()
    path = sub.subscription_path(PROJECT, SUBSCRIPTION)
    future = sub.subscribe(path, callback=handle)
    print(f"listening on {path}, metrics on :{PORT}", flush=True)
    try:
        future.result()
    except KeyboardInterrupt:
        future.cancel()


if __name__ == "__main__":
    main()
