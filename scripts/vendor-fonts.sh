#!/usr/bin/env bash
# 同梱フォント（Noto Sans JP / Serif JP / Sans Mono, OFL）を web/assets/ へ vendoring する。
#
# @fontsource のサブセット別 @font-face（unicode-range 付き）を使い、日本語＋欧文をカバーする
# 名前付きサブセット（latin / latin-ext / cyrillic / vietnamese / japanese）の woff2 のみを取り込む。
# 生成物（web/assets/fonts/ と web/assets/fonts.css）はリポジトリにコミットするため、通常のビルドに
# node_modules は不要。フォント更新時のみ、@fontsource を用意してこのスクリプトを再実行する。
#
# 前提: frontend/node_modules/@fontsource/{noto-sans-jp,noto-serif-jp,noto-sans-mono} が存在すること。
set -euo pipefail
cd "$(dirname "$0")/.."

FS=frontend/node_modules/@fontsource
OUT_DIR=web/assets/fonts
OUT_CSS=web/assets/fonts.css

if [ ! -d "$FS" ]; then
  echo "error: $FS が見つかりません（cd frontend && npm install で @fontsource を用意）" >&2
  exit 1
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"
{
  echo "/* 同梱フォント（Noto Sans JP / Serif JP / Sans Mono, OFL）。@fontsource のサブセット別 @font-face を"
  echo "   web/assets/fonts/ へ vendoring（woff2 のみ・url を /assets/fonts/ へ書換）。再生成: scripts/vendor-fonts.sh */"
} > "$OUT_CSS"

copied=0
for fam in noto-sans-jp noto-serif-jp noto-sans-mono; do
  # 等幅（コード用）は欧文のみ。日本語の等幅は Noto Sans JP へフォールバックする。
  if [ "$fam" = "noto-sans-mono" ]; then
    subsets="latin latin-ext"
  else
    subsets="latin latin-ext cyrillic vietnamese japanese"
  fi
  for w in 400 700; do
    for sub in $subsets; do
      css="$FS/$fam/$sub-$w.css"
      woff2="$FS/$fam/files/$fam-$sub-$w-normal.woff2"
      if [ -f "$css" ] && [ -f "$woff2" ]; then
        cp "$woff2" "$OUT_DIR/"
        copied=$((copied + 1))
        # url を /assets/fonts/ へ書換え、woff（非 woff2）フォールバックを除去する。
        sed -e 's#\./files/#/assets/fonts/#g' \
            -e "s#, url([^)]*\.woff) format('woff')##g" \
            "$css" >> "$OUT_CSS"
      fi
    done
  done
done

echo "vendored $copied woff2 files into $OUT_DIR"
echo "generated $OUT_CSS ($(grep -c '@font-face' "$OUT_CSS") @font-face rules)"
