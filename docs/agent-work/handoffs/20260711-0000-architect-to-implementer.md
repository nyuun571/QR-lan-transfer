# 引き継ぎ票 (Handoff) — 20260711-0000-architect-to-implementer

| 項目 | 内容 |
|------|------|
| 日時 | 2026-07-11 00:00 |
| 引き継ぎ元 | Architect(Claude Code) |
| 引き継ぎ先 | Implementer(Claude Code) |
| 工程 | P2 → P3 |
| ブランチ | main(docs のみ。コード未着手) |

## 完了したこと(検証済みの事実のみ)

- G1/G2 がユーザーの一括指示により承認された(state.md 承認欄・DEC-006 に記録)
- design-doc.md に DES-001〜007、API 一覧、セキュリティ設計(§7)、ディレクトリ構成(§8)が確定済み
- 依存ライブラリ `github.com/skip2/go-qrcode` が本環境で `go get` 取得可能なことを確認した(実行して確認)
- Go 1.26.1 が本環境にインストール済み(`go version` で確認)

## 完了していないこと

- 実装コード一式(未着手)
- implementation-plan.md(P3 冒頭で作成する)

## 検証していない推測

- スマホ実機での QR 読取・転送は本環境(実機なし)では検証できない見込み。
  P4 では HTTP レベルの自動テストで受け入れ基準を検証し、実機確認は既知の制約として記録する想定。

## 罠・注意点

- コード置き場は先行プロジェクトの慣例に従い `04-qr-lan-transfer/qrdrop/` とする(リポジトリルートは `design-doc/`)
- `agentOS/` は .gitignore 済み(読み取り専用・追跡しない)
- main 直コミット禁止(コード)。`feature/TASK-xxx-<slug>` ブランチ + マージの慣例(コミットメッセージに TASK ID)
- 2GB 上限・streaming 書込(メモリに全量を載せない)は NFR-001/002 由来の必須事項

## 次の作業者が最初にやるべきこと(1〜3個)

1. `phases/03-implementation.md` を読み、implementation-plan.md を作成する(DES 単位のタスク分解)
2. TASK-001(雛形)から実装サイクルを開始する
3. タスクごとに build / vet / test をローカル実行してからコミットする

## ビルド・テストの状態

- ビルド: N/A(コード未着手)
- テスト: N/A(コード未着手)
