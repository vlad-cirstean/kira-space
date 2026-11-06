---
title: Install and update
description: Install Kira Space with one command, and update from inside the app.
sources:
  - README.md
  - apps/kira-space/README.md
  - scripts/install.sh
  - docs/PACKAGING.md
  - internal/appupdate/checker.go
  - internal/secrets/keyring_darwin.go
sidebar:
  order: 1
---

## Install

Run this in Terminal to install Kira Space:

```sh
curl -fsSL https://raw.githubusercontent.com/vlad-cirstean/kira-studio/main/scripts/install.sh | sh
```

The same command replaces an older copy.

The script:

1. Downloads the latest release's disk image.
2. Moves the app into `/Applications`.
3. Opens the app, with no Gatekeeper prompt.
4. Moves an installed Kira Studio to the Trash.

A disk image downloaded through a browser does trigger Gatekeeper. The script avoids that.

Coming from Kira Studio? See [Migrate from Kira Studio](../migrate/).

Failures land in `~/Library/Logs/<app name>/install.log`.

## What macOS asks

:::caution
Builds are ad-hoc signed, not notarized. Each new build counts as a different app to macOS.
:::

- **Keychain.** The first launch after an update may ask to use "confidential information stored in…". Kira Space asks to use its own key. It is the one item the app stores, and it encrypts your saved credentials. Choose **Always Allow** once for that build. Kira Space never reads items of other apps.
- **Folders.** macOS may ask for access to folders that hold your repositories, such as Documents or Desktop. Allow it. Without access, Kira Space cannot read those repositories.

## Update

The app checks GitHub for a newer release and offers to update. Nothing installs until you click **Update**. The app never updates itself silently.

## Build from source

Building is the developer path. See [Building from source](https://github.com/vlad-cirstean/kira-studio#building-from-source) in the README.
