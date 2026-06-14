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

# frontend/package.json の volta フィールドに固定した node/npm と、実行環境のバージョンを照合。
# Volta 導入済みなら自動で固定版が選ばれる。未導入で版が違う場合は、frontend install 中の
# 分かりにくい engine-strict エラーになる前に、ここで明示メッセージを出して停止する。
# frontend/ 内で node/npm を実行して判定する。Volta 等 cwd 連動のマネージャは、volta/engines の
# ピンを持つ最寄りの package.json（＝frontend/。リポジトリ直下ではない）から版を選ぶため。
want_node="$(cd "$root/frontend" && node -p "require('./package.json').volta.node.split('.')[0]")"
want_npm="$(cd "$root/frontend" && node -p "require('./package.json').volta.npm.split('.')[0]")"
have_node="$(cd "$root/frontend" && node -v | sed 's/^v//' | cut -d. -f1)"
have_npm="$(cd "$root/frontend" && npm -v | cut -d. -f1)"
if [ "$have_node" != "$want_node" ] || [ "$have_npm" != "$want_npm" ]; then
  echo "Error: Node ${want_node}.x / npm ${want_npm}.x required (found node ${have_node}.x / npm ${have_npm}.x)." >&2
  echo "Install Volta (https://volta.sh) so the pinned versions are used automatically, or install matching versions manually." >&2
  exit 1
fi

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
