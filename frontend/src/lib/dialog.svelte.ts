// アプリ内モーダルダイアログの状態。Promise ベースで選択結果を返す。
// Windows のネイティブ MessageDialog はカスタム3ボタンに非対応のため、
// 確認系はアプリ内モーダルで実装する（クロスプラットフォームでも一貫）。

export type SaveChoice = 'save' | 'discard' | 'cancel'
export type MissingChoice = 'retry' | 'skip'
export type RemoteImageChoice = 'allow' | 'block'

interface DialogButton {
  label: string
  value: string
  primary?: boolean
}

class DialogStore {
  open = $state(false)
  title = $state('')
  message = $state('')
  buttons = $state<DialogButton[]>([])

  #resolve: ((value: string) => void) | null = null
  #enterValue = ''
  #escValue = ''

  get enterValue(): string {
    return this.#enterValue
  }
  get escValue(): string {
    return this.#escValue
  }

  #show(opts: {
    title: string
    message: string
    buttons: DialogButton[]
    enterValue: string
    escValue: string
  }): Promise<string> {
    this.title = opts.title
    this.message = opts.message
    this.buttons = opts.buttons
    this.#enterValue = opts.enterValue
    this.#escValue = opts.escValue
    this.open = true
    return new Promise<string>((resolve) => {
      this.#resolve = resolve
    })
  }

  /** 未保存タブ1件の保存確認（保存/保存しない/キャンセル）。 */
  confirmSave(fileName: string): Promise<SaveChoice> {
    return this.#show({
      title: '未保存の変更',
      message: `「${fileName}」には保存していない変更があります。保存しますか？`,
      buttons: [
        { label: '保存', value: 'save', primary: true },
        { label: '保存しない', value: 'discard' },
        { label: 'キャンセル', value: 'cancel' }
      ],
      enterValue: 'save',
      escValue: 'cancel'
    }) as Promise<SaveChoice>
  }

  /** セッション復元時、ファイルが見つからない場合の確認（再試行/スキップ）。 */
  confirmMissing(path: string): Promise<MissingChoice> {
    return this.#show({
      title: 'ファイルが見つかりません',
      message: `次のファイルを開けませんでした:\n${path}`,
      buttons: [
        { label: '再試行', value: 'retry', primary: true },
        { label: 'スキップ', value: 'skip' }
      ],
      enterValue: 'retry',
      escValue: 'skip'
    }) as Promise<MissingChoice>
  }

  /** リモート画像を含むファイルを開いた際の表示確認（表示する/表示しない）。 */
  confirmRemoteImages(fileName: string): Promise<RemoteImageChoice> {
    return this.#show({
      title: '外部画像の読み込み確認',
      message:
        `「${fileName}」には外部サーバーから読み込む画像が含まれています。\n` +
        '表示すると、画像の配信元サーバーにアクセス（IP アドレス等）が伝わる場合があります。\n' +
        '表示しますか？',
      buttons: [
        { label: '表示しない', value: 'block', primary: true },
        { label: '表示する', value: 'allow' }
      ],
      enterValue: 'block',
      escValue: 'block'
    }) as Promise<RemoteImageChoice>
  }

  /** ボタン選択時に呼ぶ。モーダルを閉じて Promise を解決する。 */
  choose(value: string): void {
    this.open = false
    const resolve = this.#resolve
    this.#resolve = null
    resolve?.(value)
  }
}

export const dialogStore = new DialogStore()
