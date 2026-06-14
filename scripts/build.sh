#!/usr/bin/env bash
# Markmiru ビルドスクリプト（macOS / Linux）。
#
# !!! 未検証 !!! 現在 Windows（scripts/build.ps1）でのみ動作確認済み。
#   macOS / Linux 対応に着手する際に、このスクリプトの動作を必ず検証すること。
#
# フロントエンド・Go バックエンド・実行バイナリのすべてが現在のソース状態を一括反映する:
#   - 旧 frontend/dist を先に削除し、埋め込みアセット（//go:embed all:frontend/dist）に
#     古いファイルが残らないようにする。
#   - wails build が "npm run build" を再実行して dist をソースから再生成し、
#     -clean が build/bin の旧成果物を一掃する。
#   - Go は wails build により再コンパイルされる（内容ハッシュのキャッシュで常にソース追従）。
#
# git のショート SHA をバージョンとして埋め込む（Windows 版 build.ps1 と同等）:
#   - 実行時（メニュー「Markmiru について...」の末尾に表示）: Go の ldflags (-X main.version=<sha>)
#   - OS のプロパティ（macOS: 情報を見る → バージョン）: wails.json の info.productVersion に一時注入。
# 注入した wails.json はビルド後に必ず元へ戻す（リポジトリに SHA の差分を残さない）。
#
# 使い方: リポジトリ直下で  ./scripts/build.sh
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
wails_json="$root/wails.json"
wails="$HOME/go/bin/wails"

# 版＝git ショート SHA。作業ツリーが汚れていれば -dirty を付与。
sha="$(git -C "$root" rev-parse --short HEAD)"
if [ -n "$(git -C "$root" status --porcelain)" ]; then
  sha="$sha-dirty"
fi
echo "Markmiru version: $sha"

# wails.json を退避し、終了時（成功・失敗問わず）に必ず復元する。
# 復元時は productVersion を常に "dev" へ戻す（前回クラッシュで SHA が残っていても自己修復。
# 他の編集は退避内容から保持する）。
backup="$(mktemp)"
cp "$wails_json" "$backup"
trap 'sed -E "s/(\"productVersion\"[[:space:]]*:[[:space:]]*)\"[^\"]*\"/\1\"dev\"/" "$backup" > "$wails_json"; rm -f "$backup"' EXIT

# info.productVersion の値だけを SHA に差し替える（sed -i 非依存のため一時ファイル経由）。
sed -E 's/("productVersion"[[:space:]]*:[[:space:]]*)"[^"]*"/\1"'"$sha"'"/' "$wails_json" > "$wails_json.tmp"
mv "$wails_json.tmp" "$wails_json"

# 旧 frontend/dist を削除して埋め込みアセットを必ずソースから再生成させる
# （wails build が "npm run build" を再実行して dist を作り直す）。
rm -rf "$root/frontend/dist"

# -clean で build/bin を先に一掃し、古い実行バイナリ/成果物を残さない。
"$wails" build -clean -ldflags "-X main.version=$sha"
