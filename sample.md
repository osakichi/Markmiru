# Markmiru レンダリング確認

これは **Markdown** と *mermaid* の表示テストです。  
テストですったらテストです。

## GFM 機能
- 箇条書き
- [x] 完了タスク
- [ ] 未完了タスク

1. あああ
1. いいい
1. ううう

[外部リンク](https://github.com/osakichi/markmiru/blob/main/frontend/src/lib/components/ExportPickerDialog.svelte)

[文書内リンク](#mermaid-図)

![画像](./build/appicon.png)

| 機能 | 状態 |
|------|:----:|
| 表 | OK |
| 打消し線 | ~~対応~~ |

> 引用ブロックの例です。

インラインコード `const x = 1` と、コードブロック:

```js
function greet(name) {
  return `Hello, ${name}!`
}
```

## mermaid 図

```mermaid
graph TD
  A[Markdown] --> B{markdown-it}
  B --> C[HTML]
  B --> D[mermaid 図]
  C --> E[閲覧モード表示]
  D --> E
```
