# プロジェクト状態 (State) — QR コード LAN データ転送

> このファイルは進捗の唯一の真実である。作業の開始・完了・中断のたびに更新すること。

## 概要

| 項目 | 内容 |
|------|------|
| プロジェクト名 | QR コード LAN データ転送(QRDrop) |
| 開始日 | 2026-07-09 |
| 現在工程 | P2 |
| 現在状態 | WAITING_APPROVAL(G1・G2 の承認待ち) |
| 現在の担当ロール/エージェント | Architect(Claude Code) |
| 最新ハンドオフ | [20260709-1305-analyst-to-architect.md](handoffs/20260709-1305-analyst-to-architect.md) |
| ブロッカー | なし(CLARIFY 3件は [q-20260709-1](questions/q-20260709-1.md) 参照・前提置き済み) |

## 工程状況

| 工程 | 状態 | 主成果物 | 完了日 |
|------|------|----------|--------|
| P0 企画・受付 | DONE | [project-brief.md](project-brief.md) | 2026-07-09 |
| P1 要件定義 | WAITING_APPROVAL | [requirements-spec.md](requirements-spec.md) | —(G1 待ち) |
| P2 設計 | WAITING_APPROVAL | [design/design-doc.md](design/design-doc.md) | —(G2 待ち) |
| P3 実装 | NOT_STARTED | implementation-plan.md | — |
| P4 テスト | NOT_STARTED | test/test-report.md | — |
| P5 レビュー | NOT_STARTED | review/ | — |
| P6 リリース | NOT_STARTED | release/ | — |
| P7 運用 | NOT_STARTED | ops/runbook.md | — |

※ DEC-001: ユーザーの一括指示により P1/P2 成果物をドラフト先行作成。G1 で要件修正が入った場合、P2 は REWORK となる。

## ゲート承認記録

| ゲート | 対象 | 承認者 | 日時 | コメント |
|--------|------|--------|------|----------|
| G1 要件承認 | requirements-spec.md | — | — | 承認待ち |
| G2 設計承認 | design-doc.md + ADR ×2 | — | — | 承認待ち |
| G3 リリース承認 | release-notes.md | — | — | — |

## 未回答の質問

| ID | 分類 | 概要 | 提示予定 |
|----|------|------|----------|
| [q-20260709-1](questions/q-20260709-1.md) | CLARIFY | QR表示の解釈 / LAN内通信の暗号化 / 受信ファイル保存先 | G1/G2(最終報告で提示済み) |

## 作業ログ(新しいものを上に追記)

| 日時 | 担当 | 内容 |
|------|------|------|
| 2026-07-09 13:30 | Architect | P2 完了チェックリスト消化(カバレッジ表で全REQ/NFR対応確認、ADR 2件に代替案・却下理由、費用ゼロ確認、多層防御のセキュリティ設計、セットアップ手順記載)。G2 承認待ちへ |
| 2026-07-09 13:05 | Architect | P2 開始。design-doc.md / adr-001(基本方式)/ adr-002(通信保護)作成。DEC-005 記録 |
| 2026-07-09 13:05 | Analyst | P1 完了チェックリスト消化(全REQにID/優先度/受入基準、NFR全観点、スコープ外確定、矛盾なし確認)。G1 承認待ちへ |
| 2026-07-09 12:45 | Analyst | P1 開始。requirements-spec.md 作成、DEC-004(多層防御)記録、CLARIFY 3件を質問票化 |
| 2026-07-09 12:45 | Orchestrator | P0 完了チェックリスト消化(Why/Who/What/スコープ/成功基準、やらないこと4件、仮説明示、前提記録)。P1 へハンドオフ |
| 2026-07-09 12:40 | Orchestrator | プロジェクト初期化(state.md, decision-log.md, project-brief.md) |
