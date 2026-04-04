#!/bin/sh
set -eu

contracts_root="${CONTRACTS_WORKDIR:-.ci-contracts}"
package_name="${REGISTRY_CONTRACTS_PACKAGE_NAME:-registry-contracts}"

if [ -n "${REGISTRY_CONTRACTS_PROJECT_ID:-}" ] && [ -n "${REGISTRY_CONTRACTS_VERSION:-}" ]; then
  if [ -z "${CI_API_V4_URL:-}" ] || [ -z "${CI_JOB_TOKEN:-}" ]; then
    echo "CI_API_V4_URL and CI_JOB_TOKEN are required to download registry contracts" >&2
    exit 1
  fi

  mkdir -p "$contracts_root"
  archive_path="$contracts_root/$package_name.tar.gz"
  extract_dir="$contracts_root/$package_name"

  curl --fail --show-error --silent \
    --header "JOB-TOKEN: $CI_JOB_TOKEN" \
    --output "$archive_path" \
    "${CI_API_V4_URL}/projects/${REGISTRY_CONTRACTS_PROJECT_ID}/packages/generic/${package_name}/${REGISTRY_CONTRACTS_VERSION}/${package_name}.tar.gz"

  rm -rf "$extract_dir"
  mkdir -p "$extract_dir"
  tar -xzf "$archive_path" -C "$extract_dir"

  export REGISTRY_KAFKA_CONTRACTS_DIR="$extract_dir/$package_name/contracts/kafka"
else
  export REGISTRY_KAFKA_CONTRACTS_DIR="${REGISTRY_KAFKA_CONTRACTS_DIR:-contracts/upstream/registry/kafka}"
fi

echo "Using registry kafka contracts from: $REGISTRY_KAFKA_CONTRACTS_DIR"
