# riegel fork of Gitea

This fork carries a small, rebase-friendly patch set on top of **upstream release tags**.
Nothing here is sent upstream.

## Branch and tag model

| Ref | Meaning |
|---|---|
| `main` | untouched mirror of `go-gitea/gitea` `main` (sync only, never commit) |
| `riegel/v<major>` | upstream tag from `riegel/UPSTREAM` + our patch commits, linear history |
| `v<upstream>-riegel.<n>` | what we build and deploy, e.g. `v28.1.0-riegel.1` |

Every patch is one commit whose subject starts with `[riegel]`. The patch set is
`$(cat riegel/UPSTREAM)..riegel/v<major>`.

## Rules that keep rebases cheap

1. Prefer extension points over patches: `custom/` templates and assets, `app.ini`, webhooks,
   Actions workflows, external services (agent scheduler, runners). Board columns and boards for new
   repos come from the flow installer, card moves reach the scheduler by polling: no patch needed.
2. When code must change, put new logic in **new files** (`setting_agents.go` style) and keep
   hunks in upstream files small and additive. Never reformat upstream code.
3. Every patch ships with tests, so a rebase is verified by running them.
4. One concern per commit; squash fixups into their patch before tagging.

## Updating to a new upstream release

```sh
riegel/sync.sh v28.2.0          # rebase patches onto the tag, run patch tests, tag v28.2.0-riegel.1
git push origin riegel/v28 v28.2.0-riegel.1
```

On a conflict the script stops in the rebase; resolve, `git rebase --continue`, rerun the
script with `--resume`. A new major (v29) gets a new branch `riegel/v29`, created by the
same script.

Deployment (on the Gitea VM): `/opt/gitea/upgrade.sh v28.2.0-riegel.1` builds the image from
the tag, backs up the database and data, and swaps the container.

## Patches

| Patch | Why |
|---|---|
