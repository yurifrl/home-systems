// Gen2 Cloud Function: the billing KILL SWITCH. Triggered by the same budget
// Pub/Sub topic as billing-discord. When a budget crosses KILL_RATIO (default
// 100%), it DETACHES the billing account from the whole project — GCP's only
// hard-stop mechanism. This is destructive and PROJECT-WIDE (not per bucket):
// all compute stops and some resources are deleted after GCP's grace period.
//
// Ordering is deliberate: alert Discord BEFORE detaching (the function itself
// loses billing once detached), then confirm.
const functions = require('@google-cloud/functions-framework');
const { GoogleAuth } = require('google-auth-library');

const WEBHOOK = process.env.DISCORD_WEBHOOK_URL;
const PROJECT = process.env.PROJECT_ID;
const KILL_RATIO = Number(process.env.KILL_RATIO || '1.0');
// Safety: when true, the switch alerts but does NOT detach billing. Flip to
// false in YAML only once you've verified the alert path.
const DRY_RUN = (process.env.KILL_DRY_RUN || 'true') === 'true';

const auth = new GoogleAuth({ scopes: 'https://www.googleapis.com/auth/cloud-platform' });

async function discord(content) {
  if (!WEBHOOK) return;
  await fetch(WEBHOOK, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
}

functions.cloudEvent('killswitch', async (event) => {
  const data = event.data?.message?.data;
  if (!data) return;
  const n = JSON.parse(Buffer.from(data, 'base64').toString());
  const cost = Number(n.costAmount ?? 0);
  const budget = Number(n.budgetAmount ?? 0);
  if (!budget) return;
  const ratio = cost / budget;
  if (ratio < KILL_RATIO) return;

  const name = n.budgetDisplayName || 'budget';
  const ccy = n.currencyCode || '';
  await discord(
    `@here KILL SWITCH${DRY_RUN ? ' (DRY RUN)' : ''}: budget *${name}* hit ${Math.round(ratio * 100)}% ` +
    `(${ccy} ${cost} / ${ccy} ${budget}). ` +
    (DRY_RUN
      ? `dryRun=true, billing NOT detached.`
      : `Detaching billing from project ${PROJECT} now — ALL project resources will stop.`)
  );

  if (DRY_RUN) return;

  try {
    const client = await auth.getClient();
    await client.request({
      url: `https://cloudbilling.googleapis.com/v1/projects/${PROJECT}/billingInfo`,
      method: 'PUT',
      data: { billingAccountName: '' },
    });
    await discord(`Billing DETACHED from project ${PROJECT}. Re-link billing to recover.`);
  } catch (e) {
    await discord(`KILL SWITCH FAILED to detach billing for ${PROJECT}: ${e.message}. Detach manually.`);
    throw e;
  }
});
