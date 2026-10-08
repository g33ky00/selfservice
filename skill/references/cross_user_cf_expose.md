# Cross-User CF-Expose Pattern

## Problem
Install/update a bash executable under `~/.local/bin/` from another account while:
- leaving `$HOME` untouched
- avoiding inline heredocs through `su -c`
- avoiding shell quoting failures
- handling cases where `write_file` or `cp /tmp/...` fails because of read/write boundaries

## Lessons learned
- Do not use one-line inline heredocs inside `su -c '...';` they split on the first delimiter.
- Do not mix `sudo`, `su`, and quoted heredocs in the same command.
- `write_file` to a destination will fail if the existing destination parent is not writable by the active account; check directory ownership before retrying the same write.
- Do not `cp` from `/tmp/<file>` if the source was created by another account and remains `600` to another user; that path requires that account's cooperation or a copy done under the same account.
- Safe fallback: provide the wrapper logic via an existing readable path or via a command the user can run directly, instead of looping on wrapper installation.

## Execution options
- Option A: use a shared readable path for the script, then copy under the target user in a `su` session that owns both read and write.
- Option B: make the wrapper generation idempotent and atomically readable before chmod.
- Option C: if wrapper creation remains blocked, run the wrapper logic inline from an already-readable script path, preserving JSON output and caller-visible behavior.
