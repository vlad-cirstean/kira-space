---
title: Kubernetes
description: Browse clusters, resources, events and pod logs, read only.
sources:
  - README.md
  - apps/kira-space/README.md
  - apps/kira-space/internal/kubernetes
sidebar:
  order: 7
---

The Kubernetes module reads your clusters. It never changes them.

## Read only

Kira Space sends only GET requests: discovery, list, get, events and pod logs. The module has no create, update, delete, exec, attach or port-forward path.

## Clusters

Clusters come from your kubeconfig: the paths in `KUBECONFIG`, else `~/.kube/config`. The module shows whether the file is missing or invalid. Each cluster shows a status, such as unreachable, unauthorized or forbidden. You can reorder clusters.

## MCP access

A cluster is off until you turn it on, from its menu ("MCP settings…") or with its switch in Settings, Database MCP. Agents never see an off cluster. When it is on, choose "Ask me first" (each read asks you) or "Read straight away". The choice covers all five Kubernetes tools; the approval card still names the operation (discover, list, describe or logs), so you can refuse one read. A cluster can carry a description of up to 1000 characters. Clusters that were exposed before this change stay on. One that allowed every read without asking stays "Read straight away"; any other asks first. See [MCP tools](../../reference/mcp-tools/).
