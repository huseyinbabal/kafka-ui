#!/usr/bin/env bash
# Turns the newest Playwright demo video into an optimized GIF.
set -euo pipefail
video=$(ls -t demo/out/*/video.webm | head -1)
out=${1:-../docs/demo.gif}
mkdir -p "$(dirname "$out")"
filters="fps=10,scale=1100:-1:flags=lanczos"
ffmpeg -y -loglevel error -i "$video" -vf "$filters,palettegen=max_colors=128:stats_mode=diff" /tmp/kafka-ui-palette.png
ffmpeg -y -loglevel error -i "$video" -i /tmp/kafka-ui-palette.png \
  -lavfi "$filters [x]; [x][1:v] paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" "$out"
ls -lh "$out"
