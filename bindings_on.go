//go:build bindings

package main

// isBindingsBuild は `wails build` / `wails dev` の bindings 生成段が作る一時バイナリで
// のみ true になる（Wails CLI が `bindings` タグを付けてこのパッケージをビルドし、
// 生成した exe をプロジェクト直下で実行する）。
//
// 一時バイナリは GUI を起動せず bindings を書き出して終了するが、main() の前段は
// 通常どおり実行されてしまう。副作用を伴う処理（単一インスタンス IPC・デスクトップ
// 統合）をその実行で走らせないよう、main() 側でこの定数により分岐する。
// 経緯と塞いでいる事象は docs/アーキテクチャ・画面設計.md §10 を参照。
//
// このファイルは通常の `go build ./...` / `go vet ./...` ではコンパイルされないため、
// 変更時は `go build -tags bindings .` で確認すること。
const isBindingsBuild = true
