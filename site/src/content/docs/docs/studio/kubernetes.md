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

Each cluster sets a mode for four reads: discover, list, describe and logs. A mode is `deny`, `prompt` or `allow`. A cluster can carry a description of up to 1000 characters. See [MCP tools](../../reference/mcp-tools/).
