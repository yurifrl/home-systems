# crossplane-gcp

GCP infrastructure managed as **native Crossplane** resources, with all secret
values (bucket names/UUIDs, project ID, SA emails, WIF principals) kept out of
this **public** repo.

## Architecture: public chart + private values

```
home-systems (PUBLIC)                  home-systems-values (PRIVATE)
  k8s/charts/crossplane-gcp/             gcp/values.yaml   <- real names live here
    templates/*  (generic, no names)
    values.yaml  (fake placeholders)
                  \                         /
                   ArgoCD merges at sync time ($values)
                              |
                     native Crossplane MRs in-cluster
```

The ArgoCD app `k8s/applications/crossplane-gcp.yaml` pulls two sources:
- **PUBLIC** — this chart (templates + placeholder `values.yaml`).
- **PRIVATE** — `home-systems-values`, referenced as `$values`, providing the
  real `gcp/values.yaml`.

Nothing secret is ever committed to the public repo.

## One-time setup

### 1. Providers
`k8s/applications/crossplane-providers.yaml` installs the Upbound official GCP
provider family (`storage`, `cloudplatform`, `iam`) pinned to `v2.5.4`.

> Cluster runs **Crossplane 1.20.0** (the v1.x bridge release). The v2.x
> provider packages install here for **cluster-scoped** managed resources.
> Namespace-scoped MRs would require upgrading to Crossplane v2.

### 2. GCP credentials (via 1Password + External Secrets)
Create a GCP service account with permissions to manage the target resources,
download its key JSON, and store it in 1Password:

- Item key: `crossplane-gcp` (vault `kubernetes`)
- Field labelled `creds` containing the **raw SA key JSON**

The providers chart's `ExternalSecret` syncs it to Secret `gcp-creds`
(key `creds`) in `crossplane-system`, which the `ProviderConfig` consumes.

### 3. Register the private repo with ArgoCD
ArgoCD needs read access to `home-systems-values`. Add a repo-creds Secret
(ideally via External Secrets from 1Password, matching the cluster pattern):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: home-systems-values-repo
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
stringData:
  type: git
  url: git@github.com:yurifrl/home-systems-values.git
  sshPrivateKey: |
    <deploy key with read access>
```

## Private repo layout (`home-systems-values`)

```
home-systems-values/
  gcp/
    values.yaml      # merged over this chart's values.yaml
```

Example `gcp/values.yaml` (REAL values — private only):

```yaml
providerConfig:
  projectID: <your-gcp-project-id>

deletionPolicy: Orphan      # keep Orphan while importing live infra

buckets:
  - name: nixos-images
    externalName: syscd-nixos-images-63a5e728-d596-4dab-b6de-62ad65489892
    location: us-east1
    storageClass: ARCHIVE
    forceDestroy: true
    uniformBucketLevelAccess: false
    publicAccessPrevention: inherited
  - name: backups
    externalName: syscd-backups-4b2339d2-ad04-4ba1-a184-dab578a2545d
    location: us-east1
    storageClass: ARCHIVE
    uniformBucketLevelAccess: true
    publicAccessPrevention: enforced
    retentionPolicy:
      retentionPeriod: 2592000

serviceAccounts:
  - name: foundry-sa
    accountId: foundry-gcs-access-sa
    displayName: Foundry GCS Access Service Account

projectServices:
  - name: iam
    service: iam.googleapis.com

workloadIdentity:
  enabled: true
  poolId: github-pool
  providerId: github-provider
  attributeCondition: 'attribute.repository == "yurifrl/nixos"'
  attributeMapping:
    google.subject: assertion.sub
    attribute.repository: assertion.repository
    attribute.ref: assertion.ref
  oidc:
    issuerUri: https://token.actions.githubusercontent.com
```

## Importing existing infrastructure (do NOT recreate)

These resources already exist (created by Terraform in `../terraform/google`).
Each managed resource carries `crossplane.io/external-name`, which tells
Crossplane to **adopt the existing object** rather than create a new one:

- `Bucket` -> external-name = the real bucket name
- `ServiceAccount` -> external-name = the `accountId`
- `ProjectService` -> external-name = the service (e.g. `iam.googleapis.com`)
- `WorkloadIdentityPool` / `...Provider` -> external-name = the pool/provider id

**Recommended import procedure**

1. Keep `deletionPolicy: Orphan` (default here) so nothing live can be deleted.
2. Sync; Crossplane runs an Observe and binds to the existing resource.
3. Confirm each MR reports `SYNCED=True READY=True` and shows no spurious diff:
   ```bash
   kubectl get managed
   kubectl describe bucket nixos-images
   ```
4. Reconcile any drift the provider reports (fields the import revealed).
5. Once trusted, optionally flip `deletionPolicy` to `Delete`, and remove the
   corresponding resources from the Terraform module (terraform state rm) so the
   two systems don't both own them.

## Not covered here

- `google_service_account_key` (`ServiceAccountKey`) — writes the private key to
  a k8s Secret. Add deliberately if you want it cluster-managed.
- `google_service_account_iam_binding` — map to `ServiceAccountIAMMember`
  (per-member, non-authoritative) to avoid an authoritative binding clobbering
  other members. Add when needed.
- Cloudflare — intentionally excluded (handled separately; native Crossplane
  Cloudflare providers are immature).
- Proxmox — left in Terraform (API-token/root + GPU-passthrough caveats).
