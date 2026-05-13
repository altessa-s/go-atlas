#!/usr/bin/env bash
#
# sync-proto.sh — copy freshly generated proto bindings into the target
# language repository checkout, commit, push, and on tag triggers mirror
# the tag. Invoked by .github/workflows/proto-publish.yml.
#
# Required env:
#   LANGUAGE        one of: go, java
#   TARGET_DIR      relative path to the target repo checkout (default: target)
#   SOURCE_SHA      go-atlas commit SHA the bindings were generated from
#   REF_TYPE        "branch" or "tag" (github.ref_type)
#   REF_NAME        branch name or tag name (github.ref_name)
#   BOT_NAME        git user.name for the commit
#   BOT_EMAIL       git user.email for the commit
#
# Behavior:
#   - For each language, wipe the regenerable subtree in TARGET_DIR
#     and replace it with the freshly generated output from proto/gen/<lang>/.
#   - Commit only if the working tree changed (no-op skip otherwise).
#   - Push to main; on tag triggers also create and push the matching tag.

set -euo pipefail

: "${LANGUAGE:?LANGUAGE must be set (go|java)}"
: "${SOURCE_SHA:?SOURCE_SHA must be set}"
: "${REF_TYPE:?REF_TYPE must be set}"
: "${REF_NAME:?REF_NAME must be set}"
: "${BOT_NAME:?BOT_NAME must be set}"
: "${BOT_EMAIL:?BOT_EMAIL must be set}"

TARGET_DIR="${TARGET_DIR:-target}"

if [[ ! -d "${TARGET_DIR}" ]]; then
  echo "error: target repo not checked out at ${TARGET_DIR}" >&2
  exit 1
fi

case "${LANGUAGE}" in
  go)
    SRC="proto/gen/go"
    # Wipe the regenerable subtree, preserving go.mod / go.sum / README / LICENSE / .github.
    rm -rf "${TARGET_DIR}/scheduler" "${TARGET_DIR}/protovalidate"
    cp -R "${SRC}/scheduler" "${TARGET_DIR}/scheduler"
    cp -R "${SRC}/protovalidate" "${TARGET_DIR}/protovalidate"
    ;;
  java)
    SRC="proto/gen/java"
    rm -rf "${TARGET_DIR}/src/main/java/io"
    mkdir -p "${TARGET_DIR}/src/main/java"
    cp -R "${SRC}/." "${TARGET_DIR}/src/main/java/"
    ;;
  *)
    echo "error: unknown LANGUAGE=${LANGUAGE} (expected go|java)" >&2
    exit 1
    ;;
esac

cd "${TARGET_DIR}"

git config user.name "${BOT_NAME}"
git config user.email "${BOT_EMAIL}"
git add -A

if git diff --cached --quiet; then
  echo "no changes to publish for ${LANGUAGE} at ${SOURCE_SHA}"
else
  commit_msg="chore: sync from go-atlas @ ${SOURCE_SHA}"
  if [[ "${REF_TYPE}" == "tag" ]]; then
    commit_msg="chore: sync from go-atlas ${REF_NAME} (${SOURCE_SHA})"
  fi
  git commit -m "${commit_msg}"
  git push origin HEAD:main
fi

if [[ "${REF_TYPE}" == "tag" ]]; then
  git tag --force "${REF_NAME}"
  git push --force origin "refs/tags/${REF_NAME}"
fi
