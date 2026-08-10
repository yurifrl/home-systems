// Gen2 Cloud Function: turns a GCP budget threshold notification (delivered via
// Pub/Sub) into a Discord message. Deployed by Crossplane
// (k8s/charts/crossplane-gcp/templates/billing.yaml); source is zipped and
// uploaded to GCS by .github/workflows/build-billing-function.yaml.
//
// Budget Pub/Sub payload:
// https://cloud.google.com/billing/docs/how-to/budgets-programmatic-notifications
const functions = require('@google-cloud/functions-framework');
const fs = require('fs');
const path = require('path');

// Build version (git sha) stamped into the zip by CI; shown in the alert.
const VERSION = (() => {
  try { return fs.readFileSync(path.join(__dirname, 'VERSION'), 'utf8').trim().slice(0, 12); }
  catch { return process.env.FN_VERSION || 'dev'; }
})();

const WEBHOOK = process.env.DISCORD_WEBHOOK_URL;

functions.cloudEvent('notify', async (event) => {
  const data = event.data?.message?.data;
  if (!data) return;
  const n = JSON.parse(Buffer.from(data, 'base64').toString());

  // GCP publishes cost refreshes several times a day; only alert when a budget
  // threshold (25/50/80/100%) is actually crossed.
  if (!n.alertThresholdExceeded) return;

  const name = n.budgetDisplayName || 'budget';
  const cost = Number(n.costAmount ?? 0);
  const budget = Number(n.budgetAmount ?? 0);
  const ccy = n.currencyCode || '';
  const pct = budget ? Math.round((cost / budget) * 100) : 0;
  const threshold = ` (crossed ${Math.round(n.alertThresholdExceeded * 100)}%)`;

  if (!WEBHOOK) {
    console.error('DISCORD_WEBHOOK_URL unset');
    return;
  }

  const content =
    `GCP spend: *${name}* at ${ccy} ${cost.toFixed(2)} / ${ccy} ${budget.toFixed(2)} = ${pct}%${threshold} \`v${VERSION}\``;

  const res = await fetch(WEBHOOK, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
  if (!res.ok) {
    throw new Error(`discord ${res.status}: ${await res.text()}`);
  }
});
