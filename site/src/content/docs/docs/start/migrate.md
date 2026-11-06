---
title: Migrate from Kira Studio
description: Move your data and config from Kira Studio into Kira Space.
sources:
  - README.md
  - scripts/install.sh
  - docs/PACKAGING.md
sidebar:
  order: 2
---

Kira Studio is now part of Kira Space. Its database, Api and Docker modules live inside Kira Space.

## Move your data

1. Quit Kira Studio. Kira Space refuses to start while the old app runs.
2. Run the [install command](../install/). It moves an installed Kira Studio to the Trash and leaves `~/.kira-studio` alone.
3. Open Kira Space.

On first launch Kira Space moves the data in `~/.kira-studio` into `~/.kira-space`: saved connections, scripts, runs and Api collections. The old folder is kept whole as a backup in `~/.kira-space/backups/`. Delete it when you no longer need it.

## Config folders

Config folders then move once into `~/.kira-space/config/`: connections, API files, runs, workflows, steps, repository settings and agent. A second backup lands in `~/.kira-space/backups/`.
