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
const fs = require('fs');
const path = require('path');

// Build version (git sha) stamped into the zip by CI; shown in every alert so
// you can tell exactly which deployed build fired.
const VERSION = (() => {
  try { return fs.readFileSync(path.join(__dirname, 'VERSION'), 'utf8').trim().slice(0, 12); }
  catch { return process.env.FN_VERSION || 'dev'; }
})();

const WEBHOOK = process.env.DISCORD_WEBHOOK_URL;
const PROJECT = process.env.PROJECT_ID;
const KILL_RATIO = Number(process.env.KILL_RATIO || '1.0');

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
    `@here\n` +
    `🚨🚨🚨 **KILL SWITCH TRIGGERING** 🚨🚨🚨 \`v${VERSION}\`\n` +
    `This is NOT a budget alert — the hard stop is firing RIGHT NOW.\n` +
    `Budget *${name}* hit **${Math.round(ratio * 100)}%** (${ccy} ${cost} / ${ccy} ${budget}), ` +
    `at/over the ${Math.round(KILL_RATIO * 100)}% kill threshold.\n` +
    `**DETACHING billing from project \`${PROJECT}\` NOW — ALL project resources will stop and some will be deleted after GCP's grace period.**`
  );

  try {
    const client = await auth.getClient();
    await client.request({
      url: `https://cloudbilling.googleapis.com/v1/projects/${PROJECT}/billingInfo`,
      method: 'PUT',
      data: { billingAccountName: '' },
    });
    await discord(
      `☠️ **KILL SWITCH COMPLETE** \`v${VERSION}\` — billing DETACHED from project \`${PROJECT}\`.\n` +
      `The project is now unbilled and shutting down. Re-link the billing account to recover.`
    );
  } catch (e) {
    await discord(
      `⚠️ **KILL SWITCH FAILED** \`v${VERSION}\` to detach billing for \`${PROJECT}\`: ${e.message}\n` +
      `Detach billing MANUALLY now.`
    );
    throw e;
  }
});
