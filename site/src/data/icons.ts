// Only the Codicons used on the page (CC BY 4.0, Microsoft). Typos fail `astro check`.

import arrowRight from '@vscode/codicons/src/icons/arrow-right.svg?raw';
import book from '@vscode/codicons/src/icons/book.svg?raw';
import check from '@vscode/codicons/src/icons/check.svg?raw';
import cloudDownload from '@vscode/codicons/src/icons/cloud-download.svg?raw';
import code from '@vscode/codicons/src/icons/code.svg?raw';
import database from '@vscode/codicons/src/icons/database.svg?raw';
import deviceMobile from '@vscode/codicons/src/icons/device-mobile.svg?raw';
import files from '@vscode/codicons/src/icons/files.svg?raw';
import gitBranch from '@vscode/codicons/src/icons/git-branch.svg?raw';
import gitCommit from '@vscode/codicons/src/icons/git-commit.svg?raw';
import github from '@vscode/codicons/src/icons/github.svg?raw';
import globe from '@vscode/codicons/src/icons/globe.svg?raw';
import lightbulb from '@vscode/codicons/src/icons/lightbulb.svg?raw';
import lock from '@vscode/codicons/src/icons/lock.svg?raw';
import playCircle from '@vscode/codicons/src/icons/play-circle.svg?raw';
import plug from '@vscode/codicons/src/icons/plug.svg?raw';
import robot from '@vscode/codicons/src/icons/robot.svg?raw';
import sparkle from '@vscode/codicons/src/icons/sparkle.svg?raw';
import table from '@vscode/codicons/src/icons/table.svg?raw';
import terminal from '@vscode/codicons/src/icons/terminal.svg?raw';
import vm from '@vscode/codicons/src/icons/vm.svg?raw';
import warning from '@vscode/codicons/src/icons/warning.svg?raw';

export const ICONS = {
  database: database,
  table: table,
  code: code,
  globe: globe,
  'play-circle': playCircle,
  sparkle: sparkle,
  vm: vm,
  plug: plug,
  lock: lock,
  'git-branch': gitBranch,
  'git-commit': gitCommit,
  files: files,
  robot: robot,
  lightbulb: lightbulb,
  terminal: terminal,
  'device-mobile': deviceMobile,
  check: check,
  'cloud-download': cloudDownload,
  github: github,
  book: book,
  'arrow-right': arrowRight,
  warning: warning,
} as const;

export type IconName = keyof typeof ICONS;
