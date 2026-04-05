#!/bin/sh
set -eu

: "${PLATFORM_INFRA_REPO_URL:?PLATFORM_INFRA_REPO_URL is required}"
: "${TARGET_TEAM:?TARGET_TEAM is required}"
: "${TARGET_ENV:?TARGET_ENV is required}"

promotion_sha="${CI_COMMIT_SHA:-${GITHUB_SHA:-}}"
if [ -z "$promotion_sha" ]; then
  echo "CI_COMMIT_SHA or GITHUB_SHA is required" >&2
  exit 1
fi

project_name="${CI_PROJECT_NAME:-}"
if [ -z "$project_name" ] && [ -n "${GITHUB_REPOSITORY:-}" ]; then
  project_name="${GITHUB_REPOSITORY##*/}"
fi
project_name="${project_name:-diasoft-gateway}"
PLATFORM_INFRA_BASE_BRANCH="${PLATFORM_INFRA_BASE_BRANCH:-main}"
PLATFORM_INFRA_GITHUB_TOKEN="${PLATFORM_INFRA_GITHUB_TOKEN:-${GITHUB_TOKEN:-}}"
PLATFORM_INFRA_GITHUB_API_URL="${PLATFORM_INFRA_GITHUB_API_URL:-${GITHUB_API_URL:-https://api.github.com}}"
PLATFORM_INFRA_GITHUB_REPO="${PLATFORM_INFRA_GITHUB_REPO:-}"
if [ -z "$PLATFORM_INFRA_GITHUB_REPO" ]; then
  case "$PLATFORM_INFRA_REPO_URL" in
    https://github.com/*/*.git)
      PLATFORM_INFRA_GITHUB_REPO=$(printf '%s' "$PLATFORM_INFRA_REPO_URL" | sed 's#https://github.com/##; s#\.git$##')
      ;;
  esac
fi
PLATFORM_INFRA_PUSH_URL="${PLATFORM_INFRA_PUSH_URL:-}"
if [ -z "$PLATFORM_INFRA_PUSH_URL" ] && [ -n "$PLATFORM_INFRA_GITHUB_TOKEN" ]; then
  case "$PLATFORM_INFRA_REPO_URL" in
    https://github.com/*)
      PLATFORM_INFRA_PUSH_URL=$(printf '%s' "$PLATFORM_INFRA_REPO_URL" | sed "s#https://#https://x-access-token:${PLATFORM_INFRA_GITHUB_TOKEN}@#")
      ;;
  esac
fi
PLATFORM_INFRA_PUSH_URL="${PLATFORM_INFRA_PUSH_URL:-$PLATFORM_INFRA_REPO_URL}"
PROMOTION_MODE="${PROMOTION_MODE:-direct}"
case "$PROMOTION_MODE" in
  mr) PROMOTION_MODE="pr" ;;
esac
BRANCH_NAME="promote/${project_name}/${TARGET_TEAM}-${TARGET_ENV}-${promotion_sha}"
OVERLAY_FILE="helm/tenants/${TARGET_TEAM}/${TARGET_ENV}/diasoft-gateway.yaml"
COMMIT_AUTHOR_NAME="${GITLAB_USER_NAME:-${GITHUB_ACTOR:-diasoft-gateway-ci}}"
COMMIT_AUTHOR_EMAIL="${GITLAB_USER_EMAIL:-${GITHUB_ACTOR:+${GITHUB_ACTOR}@users.noreply.github.com}}"
COMMIT_AUTHOR_EMAIL="${COMMIT_AUTHOR_EMAIL:-ci@example.com}"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

git clone "$PLATFORM_INFRA_REPO_URL" "$workdir/platform-infra"
cd "$workdir/platform-infra"
git checkout "$PLATFORM_INFRA_BASE_BRANCH"

if [ "$PROMOTION_MODE" = "pr" ]; then
  git checkout -b "$BRANCH_NAME"
fi

export PROMOTION_SHA
yq -i '.api.image.tag = strenv(PROMOTION_SHA)' "$OVERLAY_FILE"
yq -i '.consumer.image.tag = strenv(PROMOTION_SHA)' "$OVERLAY_FILE"
yq -i '.replayer.image.tag = strenv(PROMOTION_SHA)' "$OVERLAY_FILE"
yq -i '.deploymentMetadata.imageTag = strenv(PROMOTION_SHA)' "$OVERLAY_FILE"

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
git commit -m "chore: promote gateway ${TARGET_TEAM}-${TARGET_ENV} to ${promotion_sha}"

git remote set-url origin "$PLATFORM_INFRA_PUSH_URL"

if [ "$PROMOTION_MODE" = "pr" ]; then
  git push origin HEAD:"$BRANCH_NAME"

  if [ -n "${PLATFORM_INFRA_GITLAB_TOKEN:-}" ] && [ -n "${PLATFORM_INFRA_GITLAB_PROJECT_ID:-}" ]; then
    curl --fail-with-body --request POST \
      --header "PRIVATE-TOKEN: ${PLATFORM_INFRA_GITLAB_TOKEN}" \
      --data-urlencode "source_branch=${BRANCH_NAME}" \
      --data-urlencode "target_branch=${PLATFORM_INFRA_BASE_BRANCH}" \
      --data-urlencode "title=Promote diasoft-gateway ${TARGET_TEAM}-${TARGET_ENV} to ${promotion_sha}" \
      "${CI_API_V4_URL}/projects/${PLATFORM_INFRA_GITLAB_PROJECT_ID}/merge_requests"
  elif [ -n "$PLATFORM_INFRA_GITHUB_TOKEN" ] && [ -n "$PLATFORM_INFRA_GITHUB_REPO" ]; then
    curl --fail-with-body --request POST \
      --header "Authorization: Bearer ${PLATFORM_INFRA_GITHUB_TOKEN}" \
      --header "Accept: application/vnd.github+json" \
      --data "{\"title\":\"Promote diasoft-gateway ${TARGET_TEAM}-${TARGET_ENV} to ${promotion_sha}\",\"head\":\"${BRANCH_NAME}\",\"base\":\"${PLATFORM_INFRA_BASE_BRANCH}\",\"body\":\"Automated promotion for ${project_name} at ${promotion_sha}.\"}" \
      "${PLATFORM_INFRA_GITHUB_API_URL}/repos/${PLATFORM_INFRA_GITHUB_REPO}/pulls"
  else
    echo "Promotion branch pushed without pull request metadata"
  fi
else
  git push origin HEAD:"$PLATFORM_INFRA_BASE_BRANCH"
fi
