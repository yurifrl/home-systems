// Gen2 Cloud Function: turns a GCP budget threshold notification (delivered via
// Pub/Sub) into a Discord message. Deployed by Crossplane
// (k8s/charts/crossplane-gcp/templates/billing.yaml); source is zipped and
// uploaded to GCS by .github/workflows/build-billing-function.yaml.
//
// Budget Pub/Sub payload:
// https://cloud.google.com/billing/docs/how-to/budgets-programmatic-notifications
const functions = require('@google-cloud/functions-framework');

const WEBHOOK = process.env.DISCORD_WEBHOOK_URL;

functions.cloudEvent('notify', async (event) => {
  const data = event.data?.message?.data;
  if (!data) return;
  const n = JSON.parse(Buffer.from(data, 'base64').toString());

  const name = n.budgetDisplayName || 'budget';
  const cost = Number(n.costAmount ?? 0);
  const budget = Number(n.budgetAmount ?? 0);
  const ccy = n.currencyCode || '';
  const pct = budget ? Math.round((cost / budget) * 100) : 0;
  const threshold = n.alertThresholdExceeded
    ? ` (crossed ${Math.round(n.alertThresholdExceeded * 100)}%)`
    : '';

  if (!WEBHOOK) {
    console.error('DISCORD_WEBHOOK_URL unset');
    return;
  }

  const content =
    `GCP spend: *${name}* at ${ccy} ${cost.toFixed(2)} / ${ccy} ${budget.toFixed(2)} = ${pct}%${threshold}`;

  const res = await fetch(WEBHOOK, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
  if (!res.ok) {
    throw new Error(`discord ${res.status}: ${await res.text()}`);
  }
});
