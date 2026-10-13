#!/usr/bin/env bash
# Rebases the riegel patch set onto a new upstream release tag and verifies it.
# Usage: riegel/sync.sh <upstream-tag> [--resume]
set -euo pipefail
new=${1:?usage: riegel/sync.sh <upstream-tag> [--resume]}
resume=${2:-}
root=$(git rev-parse --show-toplevel); cd "$root"

git remote get-url upstream >/dev/null 2>&1 || git remote add upstream https://github.com/go-gitea/gitea.git
git fetch -q upstream "refs/tags/$new:refs/tags/$new"

old=$(cat riegel/UPSTREAM)
major=${new%%.*}
branch="riegel/$major"
current=$(git branch --show-current)

if [[ $resume != --resume ]]; then
  [[ -z "$(git status --porcelain)" ]] || { echo "working tree not clean"; exit 1; }
  src=${current:-riegel/${old%%.*}}
  echo "patches on $src since $old:"
  git --no-pager log --oneline "$old..$src"
  git branch -f "$branch" "$src"
  git switch -q "$branch"
  if ! git rebase --onto "$new" "$old" "$branch"; then
    echo "conflict: resolve, 'git rebase --continue', then rerun with --resume"; exit 1
  fi
  echo "$new" > riegel/UPSTREAM
  git commit -qm "[riegel] track upstream $new" riegel/UPSTREAM
fi

echo "building and running patch tests..."
go build ./...
pkgs=$(git diff --name-only "$new..HEAD" -- '*.go' | xargs -n1 dirname | sort -u | grep -v '^tests/' | sed 's|^|./|')
go test $pkgs
n=1; while git rev-parse -q --verify "refs/tags/$new-riegel.$n" >/dev/null; do n=$((n + 1)); done
git tag -a "$new-riegel.$n" -m "upstream $new + riegel patches"
echo "ok: tagged $new-riegel.$n on $branch — push with: git push origin $branch $new-riegel.$n"
