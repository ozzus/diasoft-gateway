#!/bin/sh
set -eu

tarball_path="${1:?tarball path is required}"
version="${2:?version is required}"
package_name="${CONTRACTS_PACKAGE_NAME:-gateway-contracts}"
file_name="${package_name}.tar.gz"

if [ ! -f "$tarball_path" ]; then
  echo "contracts tarball not found: $tarball_path" >&2
  exit 1
fi

if [ -z "${CI_API_V4_URL:-}" ] || [ -z "${CI_PROJECT_ID:-}" ] || [ -z "${CI_JOB_TOKEN:-}" ]; then
  echo "GitLab package registry variables are required" >&2
  exit 1
fi

curl --fail --show-error --silent \
  --header "JOB-TOKEN: $CI_JOB_TOKEN" \
  --upload-file "$tarball_path" \
  "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/packages/generic/${package_name}/${version}/${file_name}"
