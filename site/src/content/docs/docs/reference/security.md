---
title: Security and privacy
description: What Kira Space stores, how it protects it and what stays on your machine.
sources:
  - README.md
  - docs/PACKAGING.md
  - internal/secrets/keyring_darwin.go
  - apps/kira-space/internal/dbmcp/server.go
  - apps/kira-space/internal/kubernetes
sidebar:
  order: 5
---

## Credentials

Saved credentials, secret script parameters and agent secrets are encrypted at rest. The key is one macOS Keychain item. Kira Space asks to use it once per build. See [Install and update](../../start/install/).

## Local data

Kira Space keeps its data in `~/.kira-space` on your machine. Agents run your own `claude` or `opencode` CLI. Kira does not write your agent config, except when you click **Export**.

## MCP servers

The database MCP server is off by default. It listens on loopback only and has its own bearer token. Each connection sets its access per mode. A write or an expensive query waits for your approval. Column masking and a table allow list limit what a client sees. See [Database MCP server](../../studio/database-mcp/).

## Kubernetes

The Kubernetes module sends only GET requests. See [Kubernetes](../../studio/kubernetes/).

## Updates

The app checks GitHub for a newer release. Nothing installs until you click **Update**.
