---
name: ourtube-channels
description: Use when adding, removing, or editing YouTube channels for the ourtube app. Triggers on requests like "add youtube channel <url/handle>", "subscribe ourtube to X", or editing the channel list in k8s/applications/ourtube.yaml.
---

# Ourtube Channels

Ourtube's channel list is declared in `k8s/applications/ourtube.yaml` under
`spec.sources[0].helm.valuesObject.channels`. Each entry is `name` + `channelId`
(the `UC...` id, not the `@handle`). ArgoCD auto-syncs the app, so a committed
edit is the only step.

## Add a channel

1. Resolve the `@handle` (or channel URL) to its `UC...` id:
   ```bash
   curl -sL "https://www.youtube.com/@HANDLE" \
     | grep -oE '"externalId":"UC[^"]+"' | head -1
   ```
2. Append an entry under `channels:` in `k8s/applications/ourtube.yaml`:
   ```yaml
            - name: Human Readable Name
              channelId: UCxxxxxxxxxxxxxxxxxxxxxx
   ```
3. Commit + push. ArgoCD syncs ourtube automatically.

Removing a channel = delete its two lines. Editing = change `name`/`channelId`.
