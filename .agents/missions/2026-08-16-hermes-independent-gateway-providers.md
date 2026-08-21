# Independent LiteLLM and OmniRoute providers in Hermes

## Product outcome
Hermes users can select models from LiteLLM and OmniRoute independently in the dashboard.

## Scope
Hermes supports independent LiteLLM and OmniRoute custom providers with secure credentials, network connectivity, and visible model catalogs.

## Evidence
- Hermes dashboard model refresh displays LiteLLM: 707 models and OmniRoute: 1,426 models.
- Live Hermes pod authenticated to OmniRoute /v1/models and received 1,426 models.
- ArgoCD: OmniRoute Synced/Healthy; Hermes Synced/Healthy after dashboard restart.
