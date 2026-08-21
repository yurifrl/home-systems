# Graph Report - home-systems  (2026-08-21)

## Corpus Check
- cluster-only mode — file stats not available

## Summary
- 692 nodes · 765 edges · 102 communities (49 shown, 53 thin omitted)
- Extraction: 97% EXTRACTED · 3% INFERRED · 0% AMBIGUOUS · INFERRED: 24 edges (avg confidence: 0.65)
- Token cost: 2,376 input · 1,214 output

## Graph Freshness
- Built from commit: `8a3f2f1c`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- CLI Argument Parsing
- JSON Schema Metadata
- Kubernetes Service Schema
- Helm Chart Values Schema
- GCP External Secret Schema
- Network Policy Schema
- Kubernetes Service Ports Schema
- Node.js Package Dependencies
- Persistence Volume Schema
- Pod Affinity and Strategy
- Renovate Configuration
- Infrastructure Postmortems
- Secret Credentials Schema
- Image Preflight Schema
- SSH Endpoint Schema
- ConfigMap Integration Schema
- Workstation Seed Scripts
- Proxmox VM Defaults
- Windows VM Schema
- Bootstrap Replica Schema
- Webhook Configuration Schema
- Billing Killswitch Logic
- API Secret Configuration
- CORS and Host Schema
- Ingress Service Schema
- Agent Gateway Design
- Networking Postmortems
- VM Passthrough Schema
- VM Provisioning Schema
- Proxmox VM Schema
- Proxmox Node Schema
- Resource Deletion Policy
- Windows Builder Schema
- Egress Network Policy
- Core Infrastructure Stack
- Device Listener Logic
- MQTT Device Listener
- Billing Discord Integration
- Provider Config Schema
- Pod Management Policy
- Nostos Project Roadmap
- Feature Guardrails Schema
- Storage and Metrics Postmortems
- Nostos Operator Documentation
- AppDaemon Lamp Control
- MQTT Watcher Script
- Nostos Provisioner Implementation
- Weekly Reporting Script
- Remote Agent Specifications
- OOM Check Utility
- Operational Hardening Plans
- AppDaemon Hello World
- Nostos PXE Implementation
- ArgoCD Core Applications
- Monitoring and CNI Stack
- Main Python Handler
- Nostos Dashboard Specs
- ArgoCD Support Charts
- Storage and Hermes Apps
- ArgoCD Application Management
- Tailscale Operator Postmortem
- Herdr Update Postmortem
- Git Post-Checkout Hook
- Git Post-Merge Hook
- Git Pre-Commit Hook
- Git Pre-Push Hook
- Git Commit Message Hook
- OpenSpec AI Skills
- GitHub Actions Workflows
- AppDaemon Docker Config
- Git Setup Script
- Bind9 DNS Configuration
- DNS Testing Script
- SLO and CoreDNS Rules
- Webhook Testing Script
- Nostos Dashboard Model
- Agent Task Management
- Hermes Application Chart
- Istio Service Mesh
- Database Key Script
- Istio Ambient Mode
- Ourtube Skill
- Chart Release Workflow
- Hermes Backup Design
- Proxmox Node Template
- Release Resolver Spec
- Netboot Target Spec
- PXE Task Tracking
- Cluster Control Spec
- Node Registry Spec
- Nostos CLI Spec
- Nostos TUI Spec
- PXE Provisioning Spec
- Secrets Backend Spec
- Provisioner Interface Spec
- TPI Provisioning Spec
- Provisioner Task Tracking
- CLI Output Spec
- Dashboard Task Tracking
- Remote Agents Proposal
- Remote Agent Implementation
- Automation Workflows

## God Nodes (most connected - your core abstractions)
1. `enabled` - 14 edges
2. `main()` - 9 edges
3. `enum` - 9 edges
4. `required` - 8 edges
5. `need()` - 7 edges
6. `required` - 7 edges
7. `set_field()` - 6 edges
8. `enabledManagers` - 5 edges
9. `port` - 5 edges
10. `ingress` - 5 edges

## Surprising Connections (you probably didn't know these)
- `Bind9 ConfigMap` --semantically_similar_to--> `Bind9 Test Compose`  [INFERRED] [semantically similar]
  k8s/charts/bind9/templates/configmap.yaml → hack/bind9-test/compose.yml
- `Gatus Dead-Man's-Switch` --references--> `VictoriaMetrics Single (vmsingle)`  [INFERRED]
  nixos/modules/gatus/config.yaml → .agents/postmortems/2026-07-11-vmsingle-storage-readonly/PM.md
- `Argo CD x Crossplane Hardening Plan` --conceptually_related_to--> `Postmortem Skill`  [INFERRED]
  docs/argo-crossplane-hardening.md → .claude/skills/postmortem/SKILL.md
- `VictoriaMetrics Watchdog CronJob` --references--> `VictoriaMetrics Single (vmsingle)`  [EXTRACTED]
  k8s/charts/support-cluster/templates/monitoring/victoriametrics-watchdog.yaml → .agents/postmortems/2026-07-11-vmsingle-storage-readonly/PM.md
- `nic-offload-fix DaemonSet` --references--> `pc01`  [EXTRACTED]
  k8s/charts/support-cluster/templates/nic-offload-fix.yaml → .agents/postmortems/2026-07-05-pc01-vxlan-tx-checksum-offload/PM.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **pc01 Specific Incidents** — agents_postmortems_2026_07_05_pc01_vxlan_tx_checksum_offload_pm, agents_postmortems_2026_08_18_pc01_containerd_pleg_dead_pm [EXTRACTED 0.85]
- **Monitoring Self-Blinding Incidents** — agents_postmortems_2026_07_11_vmsingle_storage_readonly_pm, agents_postmortems_2026_07_18_cnpg_supabase_down_no_alert_pm, agents_postmortems_2026_08_10_longhorn_daemonsets_excluded_from_dell01_pm [EXTRACTED 0.90]
- **Control Plane Resource Exhaustion** — agents_postmortems_2026_07_13_apiserver_crd_cache_oom_pm, agents_postmortems_2026_08_16_crossplane_finalizer_retry_storm_pm [EXTRACTED 0.95]
- **Proxmox/Talos Provisioning Stack** — docs_nostos_guide, k8s_crossplane_proxmox_values, docs_proxmox_crossplane_implementation [EXTRACTED 0.95]
- **Crossplane Infrastructure Management** — k8s_applications_crossplane [EXTRACTED 1.00]
- **Istio Ambient Mesh Components** — k8s_applications_istio_base, k8s_applications_istio_ztunnel [EXTRACTED 1.00]
- **Nostos Version Evolution** — openspec_changes_nostos_v01_proposal, openspec_changes_nostos_v02_provisioners_proposal, openspec_changes_nostos_v03_dashboard_and_hygiene_proposal [EXTRACTED 1.00]
- **OpenSpec Framework** — claude_skills_openspec_explore_skill, claude_skills_openspec_propose_skill [EXTRACTED 1.00]
- **Provisioner Interface Pattern** — internal_provisioner_provisioner, internal_cluster_orchestrate, internal_provisioner_tpi_tpi [EXTRACTED 1.00]
- **Remote Agent System** — agent_gateway, agent_task_crd, agent_md, beads_workstrator [EXTRACTED 1.00]
- **SLO Monitoring Pipeline** — k8s_applications_pyrra, k8s_applications_victoria_metrics_k8s_stack, hack_reference_slo_class [INFERRED 0.85]
- **GitOps Provisioning Flow** — agents_md_nostos, agents_md_argocd, agents_md_onepassword [INFERRED 0.90]
- **Incident Management & Hardening** — claude_skills_postmortem_skill, claude_skills_postmortem_vocab, docs_argo_crossplane_hardening [INFERRED 0.90]

## Communities (102 total, 53 thin omitted)

### Community 0 - "CLI Argument Parsing"
Cohesion: 0.06
Nodes (70): ArgumentParser, CompletedProcess, Exception, _age(), _backup_pass(), _build_incident_prompt(), _build_parser(), _capture() (+62 more)

### Community 1 - "JSON Schema Metadata"
Cohesion: 0.06
Nodes (35): type, minLength, type, items, type, items, type, minLength (+27 more)

### Community 2 - "Kubernetes Service Schema"
Cohesion: 0.06
Nodes (30): allOf, minLength, type, $ref, definitions, portNumber, servicePort, maximum (+22 more)

### Community 3 - "Helm Chart Values Schema"
Cohesion: 0.07
Nodes (29): minLength, type, type, type, type, properties, required, name (+21 more)

### Community 4 - "GCP External Secret Schema"
Cohesion: 0.07
Nodes (28): type, minLength, type, type, type, type, properties, type (+20 more)

### Community 5 - "Network Policy Schema"
Cohesion: 0.08
Nodes (28): items, type, type, type, items, type, items, type (+20 more)

### Community 6 - "Kubernetes Service Ports Schema"
Cohesion: 0.10
Nodes (22): enum, type, enum, type, $ref, type, items, type (+14 more)

### Community 7 - "Node.js Package Dependencies"
Cohesion: 0.10
Nodes (19): google-auth-library, dependencies, @google-cloud/functions-framework, engines, node, @google-cloud/functions-framework, main, name (+11 more)

### Community 8 - "Persistence Volume Schema"
Cohesion: 0.10
Nodes (21): enum, type, type, minLength, type, properties, type, accessMode (+13 more)

### Community 9 - "Pod Affinity and Strategy"
Cohesion: 0.10
Nodes (20): properties, type, podAntiAffinity, topologyKey, type, updateStrategy, minLength, type (+12 more)

### Community 10 - "Renovate Configuration"
Cohesion: 0.12
Nodes (15): argocd, config:recommended, custom.regex, github-actions, helm-values, /k8s/applications/.+\\.yaml$/, argocd, managerFilePatterns (+7 more)

### Community 11 - "Infrastructure Postmortems"
Cohesion: 0.15
Nodes (13): Postmortem: vmsingle storage exhausted → read-only, Postmortem: kube-apiserver CRD-cache OOM on the sole control-plane (dell01), Postmortem: Longhorn DaemonSets excluded from dell01 by stale node-isolation affinity, Postmortem: Crossplane finalizer retry storm saturates kube-apiserver, Postmortem: hermes-dashboard missing PVC affinity + gh multi-account active-account race, Crossplane Provider GCP Compute, dell01, Gatus Dead-Man's-Switch (+5 more)

### Community 12 - "Secret Credentials Schema"
Cohesion: 0.15
Nodes (13): properties, required, type, credentials, secretKey, secretName, secretNamespace, type (+5 more)

### Community 13 - "Image Preflight Schema"
Cohesion: 0.29
Nodes (7): type, type, properties, type, enabled, image, preflight

### Community 14 - "SSH Endpoint Schema"
Cohesion: 0.25
Nodes (8): pattern, type, type, endpoint, insecure, sshUsername, properties, type

### Community 15 - "ConfigMap Integration Schema"
Cohesion: 0.18
Nodes (11): properties, minLength, type, type, type, configKey, existingConfigMap, overwrite (+3 more)

### Community 16 - "Workstation Seed Scripts"
Cohesion: 0.51
Nodes (10): main(), need(), note_op_token(), seed_docker(), seed_github(), seed_kubeconfig(), seed_passwords(), seed_tailscale() (+2 more)

### Community 17 - "Proxmox VM Defaults"
Cohesion: 0.20
Nodes (10): required, type, additionalProperties, required, type, defaults, isos, bridge (+2 more)

### Community 18 - "Windows VM Schema"
Cohesion: 0.20
Nodes (10): talosPc01, vms, windows, $ref, properties, required, type, $ref (+2 more)

### Community 19 - "Bootstrap Replica Schema"
Cohesion: 0.25
Nodes (8): type, properties, bootstrap, replicaCount, virtualService, minimum, type, type

### Community 20 - "Webhook Configuration Schema"
Cohesion: 0.20
Nodes (10): $ref, port, telegramWebhook, url, webhook, properties, type, type (+2 more)

### Community 21 - "Billing Killswitch Logic"
Cohesion: 0.22
Nodes (7): auth, fs, functions, { GoogleAuth }, KILL_RATIO, path, VERSION

### Community 22 - "API Secret Configuration"
Cohesion: 0.22
Nodes (9): type, type, API_SERVER_KEY, existingSecret, secrets, TELEGRAM_BOT_TOKEN, properties, type (+1 more)

### Community 23 - "CORS and Host Schema"
Cohesion: 0.18
Nodes (11): properties, type, type, minLength, type, minLength, type, apiServer (+3 more)

### Community 24 - "Ingress Service Schema"
Cohesion: 0.25
Nodes (9): type, items, properties, type, enabled, ingress, servicePortNumber, anyOf (+1 more)

### Community 25 - "Agent Gateway Design"
Cohesion: 0.29
Nodes (8): Agent Gateway, AgentTask CRD, Beads Workstrator, Remote Agents Design, Agent Gateway Spec, Agent Task Controller Spec, Beads Workstrator Spec, Pi Remote Agents Spec

### Community 26 - "Networking Postmortems"
Cohesion: 0.25
Nodes (8): Postmortem: pc01 cross-node pod networking silently broken by virtio VXLAN TX checksum offload, Postmortem: Tailscale selected Cilium overlay endpoints and broke ArgoCD repo access, Postmortem: pc01 (talos-pc01) container runtime hang → PLEG death → NotReady, ArgoCD Repo Server, Cilium, nic-offload-fix DaemonSet, pc01, Tailscale

### Community 27 - "VM Passthrough Schema"
Cohesion: 0.22
Nodes (9): minLength, type, type, name, passthrough, vmId, properties, minimum (+1 more)

### Community 28 - "VM Provisioning Schema"
Cohesion: 0.25
Nodes (8): required, credentials, defaults, isoPipeline, isos, preflight, providerConfig, vms

### Community 29 - "Proxmox VM Schema"
Cohesion: 0.29
Nodes (6): $defs, vm, $schema, title, type, type

### Community 30 - "Proxmox Node Schema"
Cohesion: 0.29
Nodes (7): type, type, properties, type, bridge, datastoreId, nodeName

### Community 31 - "Resource Deletion Policy"
Cohesion: 0.22
Nodes (9): enum, type, properties, deletionPolicy, isoPipeline, started, type, Delete (+1 more)

### Community 32 - "Windows Builder Schema"
Cohesion: 0.25
Nodes (8): required, required, builderImage, enabled, objectName, schedule, signTTLDays, win11DownloadName

### Community 33 - "Egress Network Policy"
Cohesion: 0.33
Nodes (6): items, type, properties, type, egress, networkPolicy

### Community 34 - "Core Infrastructure Stack"
Cohesion: 0.40
Nodes (5): Cilium, Nostos, 1Password, Tailscale, Talos Linux

### Community 37 - "Billing Discord Integration"
Cohesion: 0.40
Nodes (4): fs, functions, path, VERSION

### Community 38 - "Provider Config Schema"
Cohesion: 0.29
Nodes (7): name, providerConfig, required, type, required, endpoint, vmId

### Community 39 - "Pod Management Policy"
Cohesion: 0.40
Nodes (5): enum, type, podManagementPolicy, OrderedReady, Parallel

### Community 40 - "Nostos Project Roadmap"
Cohesion: 0.40
Nodes (5): Design: Nostos v0.1, Proposal: Nostos v0.1, Tasks: Nostos v0.1, Proposal: Nostos v0.2 Provisioners, Proposal: Nostos v0.3 Dashboard and Hygiene

### Community 41 - "Feature Guardrails Schema"
Cohesion: 0.40
Nodes (4): features, permissionGate, $schema, version

### Community 42 - "Storage and Metrics Postmortems"
Cohesion: 0.50
Nodes (4): Postmortem: CNPG/supabase down, cluster-wide volume provisioning dead, zero detection, CloudNativePG (CNPG), CSI Provisioner, kube-state-metrics

### Community 43 - "Nostos Operator Documentation"
Cohesion: 0.50
Nodes (4): Nostos Operator Guide, Proxmox Crossplane Implementation Report, Crossplane Proxmox Values, Nostos Config

### Community 45 - "MQTT Watcher Script"
Cohesion: 0.83
Nodes (3): main(), on_connect(), on_message()

### Community 47 - "Weekly Reporting Script"
Cohesion: 1.00
Nodes (3): main(), query(), total()

### Community 48 - "Remote Agent Specifications"
Cohesion: 0.67
Nodes (3): Agent.md, Agent Definition Spec, Remote Agent Runtime Spec

### Community 50 - "Operational Hardening Plans"
Cohesion: 0.67
Nodes (3): Postmortem Skill, Postmortem Vocabulary, Argo CD x Crossplane Hardening Plan

### Community 53 - "ArgoCD Core Applications"
Cohesion: 0.67
Nodes (3): ArgoCD Application, Crossplane Core, Postgres Application

### Community 54 - "Monitoring and CNI Stack"
Cohesion: 0.67
Nodes (3): Cilium CNI, Pyrra SLO Manager, VictoriaMetrics Stack

### Community 56 - "Nostos Dashboard Specs"
Cohesion: 0.67
Nodes (3): Nostos Dashboard, Nostos Dashboard Spec, Nostos Taskfile Migration Guide

## Knowledge Gaps
- **319 isolated node(s):** `type`, `minLength`, `type`, `type`, `type` (+314 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **53 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `enabled` connect `Ingress Service Schema` to `JSON Schema Metadata`, `Egress Network Policy`, `Helm Chart Values Schema`, `Network Policy Schema`, `Kubernetes Service Ports Schema`, `Persistence Volume Schema`, `Pod Affinity and Strategy`, `ConfigMap Integration Schema`, `Webhook Configuration Schema`, `CORS and Host Schema`?**
  _High betweenness centrality (0.064) - this node is a cross-community bridge._
- **Why does `properties` connect `Bootstrap Replica Schema` to `JSON Schema Metadata`, `Kubernetes Service Schema`, `Egress Network Policy`, `Helm Chart Values Schema`, `Network Policy Schema`, `Kubernetes Service Ports Schema`, `Pod Management Policy`, `Persistence Volume Schema`, `Pod Affinity and Strategy`, `Webhook Configuration Schema`, `API Secret Configuration`, `CORS and Host Schema`, `Ingress Service Schema`?**
  _High betweenness centrality (0.039) - this node is a cross-community bridge._
- **Why does `properties` connect `JSON Schema Metadata` to `Ingress Service Schema`, `Helm Chart Values Schema`?**
  _High betweenness centrality (0.032) - this node is a cross-community bridge._
- **Are the 11 inferred relationships involving `_build_parser()` (e.g. with `cmd_auth_login()` and `cmd_backup()`) actually correct?**
  _`_build_parser()` has 11 INFERRED edges - model-reasoned connections that need verification._
- **What connects `type`, `minLength`, `type` to the rest of the system?**
  _319 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `CLI Argument Parsing` be split into smaller, more focused modules?**
  _Cohesion score 0.05714285714285714 - nodes in this community are weakly interconnected._
- **Should `JSON Schema Metadata` be split into smaller, more focused modules?**
  _Cohesion score 0.06050420168067227 - nodes in this community are weakly interconnected._