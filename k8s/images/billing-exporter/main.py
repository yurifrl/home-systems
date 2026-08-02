#!/usr/bin/env python3
"""Subscribes to the billing-budget Pub/Sub topic and exposes the latest cost
per budget as Prometheus gauges. Pull-based: no inbound exposure, no cluster
dependency for the independent Discord alert (that path is the GCP function).

Gauges:
  gcp_billing_cost{budget,currency}    current spend
  gcp_billing_amount{budget,currency}  budget limit
"""
import base64
import json
import os

from google.cloud import pubsub_v1
from prometheus_client import Gauge, start_http_server

PROJECT = os.environ["GCP_PROJECT"]
SUBSCRIPTION = os.environ["PUBSUB_SUBSCRIPTION"]
PORT = int(os.environ.get("PORT", "9109"))

COST = Gauge("gcp_billing_cost", "Current spend for a budget", ["budget", "currency"])
LIMIT = Gauge("gcp_billing_amount", "Budget limit", ["budget", "currency"])


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


def main():
    start_http_server(PORT)
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
