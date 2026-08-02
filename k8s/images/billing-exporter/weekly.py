#!/usr/bin/env python3
"""Weekly GCP spend summary -> Discord.

Reads current spend from VictoriaMetrics (populated by main.py's exporter) and
posts a summary to the Discord webhook. Run on a schedule by a CronJob; no GCP
credentials needed. Stdlib only.
"""
import json
import os
import urllib.parse
import urllib.request

VM_URL = os.environ.get("VM_URL", "http://vmsingle-vmks.monitoring.svc:8428")
WEBHOOK = os.environ["DISCORD_WEBHOOK_URL"]


def query(expr):
    url = f"{VM_URL}/api/v1/query?" + urllib.parse.urlencode({"query": expr})
    with urllib.request.urlopen(url, timeout=15) as r:
        return json.load(r)["data"]["result"]


def total(expr):
    return sum(float(s["value"][1]) for s in query(expr))


def main():
    cost = total("gcp_billing_cost")
    limit = total("gcp_billing_amount")
    ccy = "BRL"
    for s in query("gcp_billing_cost"):
        ccy = s["metric"].get("currency", ccy)
        break
    pct = round(cost / limit * 100) if limit else 0
    if limit:
        content = (
            f"Weekly GCP spend update: {ccy} {cost:.2f} of {ccy} {limit:.2f} "
            f"month-to-date ({pct}% of budget)."
        )
    else:
        # No budget data yet (exporter has not received a budget notification).
        content = f"Weekly GCP spend update: {ccy} {cost:.2f} month-to-date (no budget data yet)."

    req = urllib.request.Request(
        WEBHOOK,
        data=json.dumps({"content": content}).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=15) as r:
        print("discord", r.status)


if __name__ == "__main__":
    main()
