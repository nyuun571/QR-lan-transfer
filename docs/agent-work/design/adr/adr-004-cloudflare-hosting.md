# ADR-004: v2 のホスティングは Cloudflare 無料枠(Pages + Workers/Durable Objects)を採用する

- **日付**: 2026-07-13
- **状態**: 提案(G2 承認待ち)
- **決定者**: Architect(Claude Code)

## 文脈 (Context)

project-brief.md §6 の「費用ゼロ」制約を維持したまま、(1) 静的 Web アプリの配信、
(2) シグナリング(WebSocket によるルーム管理・SDP/ICE 中継、状態保持)を
どこでホストするかを決める(DEC-010、ユーザー決定によりホスティングは
Cloudflare 無料枠と既に確定しているため、本 ADR は具体的なサービス構成と
無料枠の上限値を確認・記録するもの)。

## 検討した選択肢

### 案A: Cloudflare Pages(静的配信)+ Workers + Durable Objects(シグナリング) — 採用

- 利点:
  - Pages は無料枠で帯域制限なし(ビルドは月 500 回/日あたり目安 16 回 [出典: Cloudflare Pages Limits])
  - Workers Free は 1 日 10 万リクエストまで無料(UTC 深夜リセット)[出典: Cloudflare Workers Limits]
  - Durable Objects は Workers Free プランに含まれる(SQLite ストレージバックエンドのみ、無料枠 5GB)、WebSocket Hibernation API で1 Durable Object あたり最大 32,768 接続まで扱え、休止中は課金対象時間を抑えられる [出典: Cloudflare Durable Objects Limits/Pricing]
  - `wrangler` CLI + GitHub 連携でデプロイが完結し、TLS(`https://`)・WSS(`wss://`)が標準対応
- 欠点: Durable Objects の実装は TypeScript/JavaScript が前提(v1 の Go 資産は流用不可)。Workers Free は 1 リクエストあたり CPU 時間 10ms・50 外部サブリクエストの制限があり、シグナリング処理をこの範囲に収める設計が必要

### 案B: Render / Fly.io の無料枠で Go 製シグナリングサーバをホスト — 却下

- 利点: v1 と実装言語(Go)を統一できる
- 欠点: 無料枠インスタンスはアイドル時にスリープし、再開時にコールドスタート遅延が発生する(「QR を読んですぐ使える」体験を損なう)。両サービスとも無料枠の縮小・終了が過去に発生しており、継続性のリスクがある
- 却下理由: 中核体験(即時性)を損なうリスクと、無料枠継続性への懸念

### 案C: 自前ドメイン + 小型 VPS — 却下

- 利点: 構成の自由度が最大、言語も自由
- 欠点: 月額費用が発生する
- 却下理由: project-brief.md §6 の費用ゼロ制約に反する

## 決定 (Decision)

案A を採用する。Pages・Workers・Durable Objects の無料枠は本サービスの想定利用規模
(個人利用・同時セッション少数)に対して十分に余裕があり、TURN 中継を使わない設計
(adr-003)とあわせて費用ゼロを維持できる。

## 影響 (Consequences)

- シグナリングサーバの実装言語は TypeScript(Cloudflare Workers ランタイム)。v1 の Go コードとは別実装になる
- 開発環境に Node.js・`wrangler` CLI・Cloudflare アカウントが必要(README のセットアップ手順に追記する)
- Workers Free の 1 日 10 万リクエスト上限、Durable Objects の SQLite 5GB 上限に達した場合、新規ルーム作成やシグナリングが失敗しうる(NFR-103)。上限到達時は明示的なエラーメッセージを表示するのみとし、自動的な有料プランへの切替は行わない(費用ゼロ制約の維持。P2 の design-doc で挙動を明記)
- 費用: なし。想定利用規模では無料枠内に収まる見込み(監視方法は P7 の runbook v2 で扱う)
