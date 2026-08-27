#!/usr/bin/env bash
# 同梱フォント（Noto Sans JP / Serif JP / Sans Mono, OFL）を web/assets/ へ vendoring する。
#
# @fontsource のサブセット別 @font-face を使い、日本語＋欧文をカバーする名前付きサブセット
# （latin / latin-ext / cyrillic / vietnamese / japanese）の woff2 のみを取り込む。
#
# 取得元は @fontsource の npm パッケージだが、**npm は使わず** jsDelivr（npm CDN）から直接
# ダウンロードする（本プロジェクトは node / npm に依存しない方針）。CDN 上の内容は npm パッケージと
# 同一で、サブセット別 CSS（`japanese-400.css` 等）と `files/*.woff2` が同じ構成で並ぶ。
#
# 生成物（web/assets/fonts/ と web/assets/fonts.css）はリポジトリにコミットするため、通常のビルドで
# このスクリプトを実行する必要は無い。フォント更新時のみ VERSION を上げて再実行する。
#
# 前提: curl（Windows は Git Bash 同梱の curl.exe。PowerShell の curl は Invoke-WebRequest の
#       別名で挙動が異なるため、実体の curl.exe を優先する）。
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=5.3.0 # @fontsource のバージョン（3 パッケージ共通で pin し、再現性を確保する）
CDN=https://cdn.jsdelivr.net/npm/@fontsource
OUT_DIR=web/assets/fonts
OUT_CSS=web/assets/fonts.css

# curl.exe（Windows の実体）を優先し、無い環境（macOS / Linux）は curl にフォールバックする。
CURL=curl.exe
command -v "$CURL" >/dev/null 2>&1 || CURL=curl
command -v "$CURL" >/dev/null 2>&1 || {
  echo "error: curl が見つかりません（Windows は Git Bash 同梱の curl.exe を使用）" >&2
  exit 1
}

# 取得できなければ即失敗させる（-f）。生成物が中途半端に混ざるのを防ぐ。
fetch() { "$CURL" -fsSL --retry 3 --retry-delay 2 -o "$2" "$1"; }

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"
{
  echo "/* 同梱フォント（Noto Sans JP / Serif JP / Sans Mono, OFL）。@fontsource $VERSION のサブセット別 @font-face を"
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
      css_tmp="$OUT_DIR/.$fam-$sub-$w.css"
      woff2="$fam-$sub-$w-normal.woff2"
      # そのサブセットを持たないパッケージもあるため、CSS が無ければ静かに飛ばす。
      if ! fetch "$CDN/$fam@$VERSION/$sub-$w.css" "$css_tmp" 2>/dev/null; then
        rm -f "$css_tmp"
        continue
      fi
      fetch "$CDN/$fam@$VERSION/files/$woff2" "$OUT_DIR/$woff2"
      copied=$((copied + 1))
      # url を /assets/fonts/ へ書換え、woff（非 woff2）フォールバックを除去する。
      sed -e 's#\./files/#/assets/fonts/#g' \
          -e "s#, url([^)]*\.woff) format('woff')##g" \
          "$css_tmp" >> "$OUT_CSS"
      rm -f "$css_tmp"
    done
  done
done

echo "vendored $copied woff2 files into $OUT_DIR (@fontsource $VERSION via jsDelivr)"
echo "generated $OUT_CSS ($(grep -c '@font-face' "$OUT_CSS") @font-face rules)"
