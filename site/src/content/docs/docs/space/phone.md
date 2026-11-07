---
title: Phone view
description: Pair a phone to watch and steer the agents board over your home network.
sources:
  - apps/kira-space/README.md
  - docs/ARCHITECTURE.md
  - apps/kira-space/internal/mobileweb
  - apps/kira-space/internal/lannet
  - packages/theme/src/i18n/en.json
sidebar:
  order: 7
---

The phone view shows the agents board on a phone: Backlog, Need You and Plan. It is off by default.

:::danger
The phone view uses plain HTTP with no encryption. Anyone on the same network can read the board, the backlog and terminal output. They can also copy a phone's cookie and act as that phone. Enable it on a trusted home network only.
:::

## Turn it on

1. Open `Settings > Mobile access`.
2. Choose **Trust this network**.
3. Enable **View the agents board from a phone on my trusted network**.

The server listens on one address: this computer's address on the trusted network. The default port is 7790; change it in the same pane. The server stops when this computer leaves the trusted network and starts again when it returns. **Forget** clears the trusted network.

## Pair a phone

1. Scan the QR code in the pane with the phone.
2. The phone shows a four-digit code.
3. Approve the code in the dialog on the desktop.

Device tokens expire after 30 days. Pair again after that. **Revoke** removes a phone at any time.

## What a phone can do

Each paired phone has two switches in the device list:

- **Changes**: reorder the backlog, move tasks between steps, start runs.
- **Agent input**: reply to agents and control their terminals.

Agent input also needs the global switch **Let phones reply to agents and control their terminals**. It is off by default. A new pairing gets Changes on and Agent input off.

When a phone controls a terminal, the desktop window shows **Reconnect here** to take control back.

Tap a link in a terminal to open it on the phone.
