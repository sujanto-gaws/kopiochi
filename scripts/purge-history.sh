#!/usr/bin/env bash
#
# Phase 5.1 — purge committed binaries and secrets from git history.
#
# THIS SCRIPT IS NOT RUN BY CI, AND MUST NOT BE. It rewrites every commit hash
# in the repository and force-pushes the result. `git pull` cannot recover from
# that: everyone re-clones, and anyone who does not will silently push the old
# history back on their next merge.
#
# It is committed so the operation is reviewable in advance rather than typed
# from memory at the console, and so that the binaries and the secret are
# purged in ONE rewrite rather than two — a second rewrite doubles the
# coordination cost for no benefit.
#
# ---------------------------------------------------------------------------
# Before running
# ---------------------------------------------------------------------------
#
#   1. Announce a freeze. Everyone pushes outstanding work and stops.
#   2. Merge or close every open pull request. PR refs point at commits that
#      will cease to exist, and GitHub does not rewrite them.
#   3. Take the backup mirror this script makes, and keep it until the team has
#      confirmed the new history is good. It is the only way back.
#   4. Read the DRY RUN output. Do not skip this.
#
# ---------------------------------------------------------------------------
# After running
# ---------------------------------------------------------------------------
#
#   1. Everyone deletes their clone and re-clones. Not `git pull`.
#   2. Re-protect the default branch — a force-push to a protected branch
#      requires temporarily lifting protection, and it must go back on.
#   3. ROTATE THE EXPOSED CREDENTIALS ANYWAY. This is the part people skip.
#      Removing a secret from history does not un-expose it: it was cloned,
#      cached, forked and indexed while it was there. The rewrite limits future
#      exposure; only rotation addresses the past. See
#      docs/architectures/03-configuration/secret-management.md.
#
set -euo pipefail

REMOTE="${1:-}"
if [[ -z "$REMOTE" ]]; then
	cat >&2 <<'USAGE'
usage: scripts/purge-history.sh <remote-url> [--execute]

  <remote-url>  the repository to rewrite, e.g. git@github.com:org/kopiochi.git
  --execute     actually force-push. Without it this is a dry run.

Dry run by default. A script that rewrites history and force-pushes on its
first invocation, with no confirmation, is a footgun regardless of how good
the comments are.
USAGE
	exit 64
fi

EXECUTE=false
[[ "${2:-}" == "--execute" ]] && EXECUTE=true

if ! command -v git-filter-repo >/dev/null 2>&1; then
	echo "error: git-filter-repo is not installed." >&2
	echo "       pip install git-filter-repo   (do NOT substitute filter-branch:" >&2
	echo "       it is orders of magnitude slower and mangles tags)" >&2
	exit 69
fi

WORKDIR="$(mktemp -d)"
MIRROR="$WORKDIR/kopiochi-mirror.git"
BACKUP="$PWD/kopiochi-history-backup-$(date +%Y%m%d-%H%M%S).bundle"

echo "==> Mirroring $REMOTE"
git clone --mirror "$REMOTE" "$MIRROR"

echo "==> Writing a backup bundle to $BACKUP"
# A bundle is a single file containing every ref and object. If the rewrite is
# wrong, this is what restores the repository.
git -C "$MIRROR" bundle create "$BACKUP" --all

echo "==> Size before"
du -sh "$MIRROR"

# What gets removed, and why each entry is here:
#
#   bin/            ~120 MB of committed Go binaries (three of them, ~38 MB
#                   each). Git cannot delta-compress stripped Go binaries, so
#                   every rebuild that was committed added a full copy.
#   kopiochi.exe    a 20 MB Windows build committed at the repository root.
#   keys/, *.pem    the RSA signing key. See the rotation note above.
#   .env            environment files carrying credentials.
#
# --invert-paths means "remove these", not "keep only these". Getting that
# backwards deletes the entire codebase, which is the single most likely way
# to misuse this tool.
echo "==> Rewriting history (removing binaries and secrets)"
git -C "$MIRROR" filter-repo --force \
	--invert-paths \
	--path bin/ \
	--path kopiochi.exe \
	--path keys/ \
	--path .env \
	--path-glob '*.pem' \
	--path-glob '*.key'

echo "==> Size after"
du -sh "$MIRROR"

echo
echo "==> Verifying nothing obvious survived"
if git -C "$MIRROR" rev-list --objects --all | grep -Ei '(^|/)(bin/|kopiochi\.exe|keys/)|\.pem$|\.key$'; then
	echo "error: matching paths are still present in the rewritten history." >&2
	echo "       Do NOT push. Investigate before continuing." >&2
	exit 1
fi
echo "    clean"

echo
echo "==> Ten largest remaining objects"
git -C "$MIRROR" rev-list --objects --all |
	git -C "$MIRROR" cat-file --batch-check='%(objecttype) %(objectname) %(objectsize) %(rest)' |
	awk '$1 == "blob" {print $3, $4}' | sort -rn | head -10 |
	awk '{printf "    %8.1f MB  %s\n", $1/1048576, $2}'

if [[ "$EXECUTE" != true ]]; then
	echo
	echo "DRY RUN. Nothing was pushed."
	echo "Backup bundle: $BACKUP"
	echo "Rewritten mirror: $MIRROR"
	echo
	echo "Inspect it, then re-run with --execute to force-push."
	exit 0
fi

echo
read -r -p "Force-push the rewritten history to $REMOTE? Type 'rewrite' to confirm: " reply
if [[ "$reply" != "rewrite" ]]; then
	echo "Aborted. Nothing was pushed."
	exit 1
fi

echo "==> Force-pushing"
# filter-repo removes the origin remote deliberately, to stop exactly this
# command being run by accident. Adding it back is the explicit act.
git -C "$MIRROR" remote add origin "$REMOTE"
git -C "$MIRROR" push --force --all origin
git -C "$MIRROR" push --force --tags origin

cat <<EOF

Done.

  Backup bundle: $BACKUP
  Keep it until the team confirms the new history is good.

Now, in order:
  1. Tell everyone to DELETE their clone and re-clone. Not 'git pull'.
  2. Re-enable branch protection on the default branch.
  3. Rotate the exposed signing key and database credentials. The rewrite
     limits future exposure; it does nothing about the period they were
     public.
EOF
