One file per repository. The `path` key names the repository; the file name is free. A repository without a file uses the defaults.

- Changing `path` moves the settings to another repository. Deleting the file restores the defaults. Removing a repository from Kira keeps its file, and adding it again applies it. Renaming a file changes nothing.
- Kira reads repository settings only from this folder, never from inside a repository (`.git/config`, tracked files).
- The prepare script runs in each new task worktree, with its timeout. Environment scripts run in the repository root on board refresh.
- `prepare_run` and `deployed_sha_run` run a normal script of the Run module by name instead of inline text, like a workflow step. Smart scripts are refused. A script with "Run in ADE worktree" on runs in the new worktree (the repository root for an environment), else in its own folder.
- `integration_branches` lists branches besides the main branch. The main branch is ignored if listed.
- The Git window's Repository settings dialog edits the graph, stash, review, pull, host and checkout keys in this file. It cannot set `worktree` keys.
- Repository name, color, order and hidden state are set in Kira, not in files. Branch heads, fetch state and task data stay in Kira's database.
