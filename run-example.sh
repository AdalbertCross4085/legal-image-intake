#!/bin/sh
set -eu

curl --fail-with-body -X POST http://localhost:8080/matters/intake \
  -F matter_id=MAT-2026-1042 \
  -F deadline=2026-10-02 \
  -F format=webp \
  -F document=@"${1:?pass an image path}"
