#!/bin/sh
set -eu

[ "$#" -eq 2 ] || { echo "usage: release-gate.sh <version> <absolute-output-dir>" >&2; exit 2; }
version="$1"
output="$2"
case "$output" in /*) ;; *) echo "output must be absolute" >&2; exit 2 ;; esac
[ ! -e "$output" ] || { echo "refusing to overwrite $output" >&2; exit 1; }

for command in go docker syft trivy cosign; do
    command -v "$command" >/dev/null 2>&1 || { echo "missing release tool: $command" >&2; exit 1; }
done

mkdir -p "$output"
go test -mod=vendor ./...
go test -mod=vendor -race ./...
go vet -mod=vendor ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -trimpath -ldflags="-s -w -buildid=" -o "$output/vlesshappy-linux-amd64" ./cmd/vlesshappy
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -trimpath -ldflags="-s -w -buildid=" -o "$output/vlesshappy-linux-arm64" ./cmd/vlesshappy
docker buildx build --platform linux/amd64 --tag "vlesshappy:$version" \
    --metadata-file "$output/image-metadata.json" \
    --output "type=oci,dest=$output/vlesshappy-$version-linux-amd64.oci.tar" .
syft "oci-archive:$output/vlesshappy-$version-linux-amd64.oci.tar" -o "spdx-json=$output/vlesshappy-$version.spdx.json"
trivy image --input "$output/vlesshappy-$version-linux-amd64.oci.tar" --exit-code 1 --severity HIGH,CRITICAL --format json --output "$output/trivy.json"
if command -v sha256sum >/dev/null 2>&1; then
    (cd "$output" && sha256sum vlesshappy-linux-amd64 vlesshappy-linux-arm64 "vlesshappy-$version-linux-amd64.oci.tar" "vlesshappy-$version.spdx.json" image-metadata.json trivy.json > SHA256SUMS)
else
    (cd "$output" && shasum -a 256 vlesshappy-linux-amd64 vlesshappy-linux-arm64 "vlesshappy-$version-linux-amd64.oci.tar" "vlesshappy-$version.spdx.json" image-metadata.json trivy.json > SHA256SUMS)
fi
cosign sign-blob --yes --bundle "$output/SHA256SUMS.sigstore.json" "$output/SHA256SUMS"
