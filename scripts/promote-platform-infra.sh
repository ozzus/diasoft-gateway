#!/bin/sh
set -eu

: "${PLATFORM_INFRA_REPO_URL:?PLATFORM_INFRA_REPO_URL is required}"
: "${TARGET_TEAM:?TARGET_TEAM is required}"
: "${TARGET_ENV:?TARGET_ENV is required}"
: "${CI_COMMIT_SHA:?CI_COMMIT_SHA is required}"

PLATFORM_INFRA_BASE_BRANCH="${PLATFORM_INFRA_BASE_BRANCH:-main}"
PLATFORM_INFRA_PUSH_URL="${PLATFORM_INFRA_PUSH_URL:-$PLATFORM_INFRA_REPO_URL}"
PROMOTION_MODE="${PROMOTION_MODE:-direct}"
BRANCH_NAME="promote/${CI_PROJECT_NAME:-diasoft-gateway}/${TARGET_TEAM}-${TARGET_ENV}-${CI_COMMIT_SHA}"
OVERLAY_FILE="helm/tenants/${TARGET_TEAM}/${TARGET_ENV}/diasoft-gateway.yaml"
COMMIT_AUTHOR_NAME="${GITLAB_USER_NAME:-diasoft-gateway-ci}"
COMMIT_AUTHOR_EMAIL="${GITLAB_USER_EMAIL:-ci@example.com}"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

git clone "$PLATFORM_INFRA_REPO_URL" "$workdir/platform-infra"
cd "$workdir/platform-infra"
git checkout "$PLATFORM_INFRA_BASE_BRANCH"

if [ "$PROMOTION_MODE" = "mr" ]; then
  git checkout -b "$BRANCH_NAME"
fi

export CI_COMMIT_SHA
yq -i '.api.image.tag = strenv(CI_COMMIT_SHA)' "$OVERLAY_FILE"
yq -i '.consumer.image.tag = strenv(CI_COMMIT_SHA)' "$OVERLAY_FILE"
yq -i '.replayer.image.tag = strenv(CI_COMMIT_SHA)' "$OVERLAY_FILE"
yq -i '.deploymentMetadata.imageTag = strenv(CI_COMMIT_SHA)' "$OVERLAY_FILE"

if [ -n "${REGISTRY_CONTRACT_VERSION:-}" ]; then
  export REGISTRY_CONTRACT_VERSION
  yq -i '.deploymentMetadata.registryContractVersion = strenv(REGISTRY_CONTRACT_VERSION)' "$OVERLAY_FILE"
fi

if git diff --quiet -- "$OVERLAY_FILE"; then
  echo "No platform-infra changes for $OVERLAY_FILE"
  exit 0
fi

git config user.name "$COMMIT_AUTHOR_NAME"
git config user.email "$COMMIT_AUTHOR_EMAIL"
git add "$OVERLAY_FILE"
git commit -m "chore: promote gateway ${TARGET_TEAM}-${TARGET_ENV} to ${CI_COMMIT_SHA}"

git remote set-url origin "$PLATFORM_INFRA_PUSH_URL"

if [ "$PROMOTION_MODE" = "mr" ]; then
  git push origin HEAD:"$BRANCH_NAME"

  if [ -n "${PLATFORM_INFRA_GITLAB_TOKEN:-}" ] && [ -n "${PLATFORM_INFRA_GITLAB_PROJECT_ID:-}" ]; then
    curl --fail-with-body --request POST \
      --header "PRIVATE-TOKEN: ${PLATFORM_INFRA_GITLAB_TOKEN}" \
      --data-urlencode "source_branch=${BRANCH_NAME}" \
      --data-urlencode "target_branch=${PLATFORM_INFRA_BASE_BRANCH}" \
      --data-urlencode "title=Promote diasoft-gateway ${TARGET_TEAM}-${TARGET_ENV} to ${CI_COMMIT_SHA}" \
      "${CI_API_V4_URL}/projects/${PLATFORM_INFRA_GITLAB_PROJECT_ID}/merge_requests"
  else
    echo "Promotion branch pushed without merge request metadata"
  fi
else
  git push origin HEAD:"$PLATFORM_INFRA_BASE_BRANCH"
fi
