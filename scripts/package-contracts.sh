#!/bin/sh
set -eu

output_dir="${1:-build/contracts}"
package_name="${CONTRACTS_PACKAGE_NAME:-gateway-contracts}"

mkdir -p "$output_dir/$package_name/api/openapi"
cp api/openapi/openapi.yaml "$output_dir/$package_name/api/openapi/openapi.yaml"

tarball_path="$output_dir/$package_name.tar.gz"
tar -C "$output_dir" -czf "$tarball_path" "$package_name"

echo "Packaged $package_name at $tarball_path"
