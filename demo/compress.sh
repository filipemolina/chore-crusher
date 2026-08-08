#!/usr/bin/env bash
# Compress the raw VHS-recorded demo/demo.gif for the README.
#
# VHS writes every frame at 25fps in full at 1200x660, and the raw GIF comes
# out around 900K. Three things shrink it while keeping text legible:
#   1. mpdecimate — drop duplicate frames (most of a recording's bytes)
#   2. fps=10      — halve the rate (VHS text doesn't need 25fps)
#   3. scale=900   — GitHub renders a README image at ~900px wide anyway
#
# The palette is sampled from ALL frames at the target resolution (pre-mpdecimate)
# so no colour is lost when duplicate frames are dropped. max_colors=48 matches
# stack-stitcher's compression recipe.
set -euo pipefail

cd "$(dirname "$0")/.."

FILT="mpdecimate,fps=10,scale=900:-1:flags=lanczos"

# Step 1: generate a 48-color palette from every (upscaled) frame.
# -update 1 writes a single PNG (palettegen emits one frame, but ffmpeg's
# image2 muxer otherwise wants a sequence pattern).
ffmpeg -y -i demo/demo.gif \
  -vf "fps=25,scale=900:-1:flags=lanczos,palettegen=max_colors=48" \
  -update 1 /tmp/cc-pal.png

# Step 2: apply the palette + the compression filter chain.
ffmpeg -y -i demo/demo.gif -i /tmp/cc-pal.png \
  -lavfi "$FILT[x];[x][1:v]paletteuse=dither=none" \
  -fps_mode vfr /tmp/cc-demo.gif

cp /tmp/cc-demo.gif demo/demo.gif
rm -f /tmp/cc-pal.png /tmp/cc-demo.gif

echo "Compressed to demo/demo.gif ($(ls -lh demo/demo.gif | awk '{print $5}'))"
