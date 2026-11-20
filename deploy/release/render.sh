#!/usr/bin/env bash
# Renders the release manifest for a tag: deploy/release/render.sh v1.2.3 > release.yaml
set -euo pipefail
tag=${1:?usage: render.sh <tag>}
sed -e "s/__VERSION__/${tag}/g" -e "s/__IMAGE_TAG__/${tag#v}/g" "$(dirname "$0")/kafka-ui.yaml"
