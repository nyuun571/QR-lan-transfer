# レビュー報告書 (Review Report) — ラウンド 1

## 実施情報

| 項目 | 内容 |
|------|------|
| 実施日 | 2026-07-12 |
| 実施者 | Reviewer(Claude Code。P5 総合レビュー) |
| **分離規則の充足** | 実装・テストを行ったセッションとは別のサブエージェント(独立コンテキスト)で実施。コードは一切修正せず、指摘の記録のみを行った |
| 対象コミット | `a96e25221f152be491d02a02a60c9e205f24faa8`(main 最新。"Merge feature/qrdrop-p4-testing (P4)") |
| 種別 | 総合(P4 完了後・リリース前) |

## 検証実行結果(自分で再実行)

| 検証 | コマンド | 結果 |
|------|----------|------|
| ビルド | `go build ./...` | 成功 |
| 整形 | `gofmt -l .` | 差分なし(準拠) |
| 静的解析 | `go vet ./...` | 指摘なし |
| 静的解析 | `staticcheck ./...`(honnef.co/go/tools@latest) | 指摘なし(exit 0) |
| 自動テスト | `go test ./... -count=1` | 全パッケージ ok(単体・統合 31 + E2E 12 = 43 件全 Pass、E2E は実バイナリ起動) |
| 依存脆弱性 | `govulncheck ./...`(golang.org/x/vuln/cmd/govulncheck@latest) | 標準ライブラリに 11 件のシンボル到達脆弱性を検出(exit 3)。詳細は指摘 #1・下記セキュリティ節 |

- ビルド環境: Go 1.26.1 windows/amd64(go.mod は `go 1.22`)。

## 観点別結果

| 観点 | 結果 | 指摘数 |
|------|------|--------|
| 正しさ | 問題なし(軽微な境界・並行の改善余地のみ) | 1(Minor) |
| 要件整合 | 問題なし(Must REQ-001〜005・NFR すべて実装で充足) | 0 |
| セキュリティ(依存脆弱性: govulncheck。標準ライブラリ 11 件検出、いずれも本アプリの平文 HTTP 文脈では実質未到達/低影響) | 指摘あり | 2(Minor) |
| 保守性 | 問題なし(gofmt/vet/staticcheck 準拠、命名・構成が設計 §8 と一致) | 0 |
| 成果物整合 | 指摘あり(decision-log と設計・実装の記述差) | 1(Minor) |

### 正しさ(詳細)

- `session.Manager`: `Validate` は `subtle.ConstantTimeCompare`(定数時間比較)、TTL は `now()` フック経由で境界テスト済み。`Revoke`/`Token`/`ExpiresAt` は `sync.Mutex` で保護。`issue()` は初期化時を除き呼び出し側ロック前提で正しく運用。**問題なし**。
- `transfer.Store`: `mu` が `shared/received/subs/nextID` を保護。`notify()` はロック内で非ブロッキング送信(`default` フォールスルー)し、購読チャネルはバッファ 1。`AddShared/AddReceived` はロック解放後に `notify()` を呼び再帰ロックなし。**デッドロック・データ競合なし**。
- SSE(`Handler.Events`): `r.Context().Done()` で切断検知し `defer Unsubscribe`。別 goroutine を起こさずハンドラ goroutine 内で `select` するため **goroutine リークなし**。
- アップロードは `io.LimitReader(part, max+1)` で上限超過を検知し `.part` → `os.Rename` の原子的差し替え、失敗時は `os.Remove` で残骸除去。パストラバーサルは `SanitizeFilename`(パス区切り除去・禁止/制御文字除去・`.`/`..` 無害化)で防御。**問題なし**。
- 指摘 #2(Minor): 同名ファイルの同時アップロードで `uniquePath` の TOCTOU により 2 件目が 500 になりうる境界。

### 要件整合(詳細)

- REQ-001(QR・NIC 検出・ポートフォールバック・LAN 未接続エラー): `cmd/qrdrop`(`lanIPv4s`/`checkLan`/`listenWithFallback`)+ `web` の QR 配信で充足。TC-001/002/003 で検証。
- REQ-002(スマホ→PC・連番・テキスト): `Upload`/`TextFromPhone`/`uniquePath`。TC-004/005/006。
- REQ-003(PC→スマホ・SSE 即時): `AdminShare`/`Items`/`Download`/`Events`。TC-007。
- REQ-004(同一 NW 限定): `guard.Middleware` の 4 層(private IP → /admin ループバック限定 → トークン検査 → その他拒否)。TC-008/009/010。
- REQ-005(失効): TTL + `Revoke`。TC-011/012。
- NFR-001/002/004/005 も実装・テストで確認(NFR-001 は 100MB 0.88 秒のローカル実測)。**カバレッジ表(設計 §10)と実装の対応に欠落なし**。

### セキュリティ(詳細)

- 入力検証: ファイル名サニタイズ(13 パターンの単体 + E2E トラバーサル)、アップロード上限 2GB(413)、テキスト 1MB 上限、`Content-Disposition` の `filename*` は全非英数字をパーセントエンコードしヘッダインジェクション不可。**問題なし**。
- 認可: `/admin*` はループバック限定(0.0.0.0 バインドでも LanGuard で遮断=多層防御)、`/s/<token>/*` はトークン有効性検査。トークンバイパス経路(空トークン `/s/`、`/` 直叩き、無効/失効トークン)はいずれも 403 を確認。**バイパス経路なし**。
- 秘密情報混入: ソース・ログにトークン以外の秘密なし。ログは接続元 IP・操作・ファイル名・結果のみで**本文・ファイル内容を記録しない**(TC-017 で確認)。`Item.Path` は JSON 応答から除外(`json:"-"`)。CORS ヘッダを付与しないため、悪意あるサイトからのクロスオリジン応答読み取りは不可。**問題なし**。
- 指摘 #1(Minor): govulncheck が標準ライブラリ 11 件を検出。
- 指摘 #3(Minor): `/admin` の状態変更 POST に CSRF/Origin 対策がない。

## 指摘一覧

> Severity: Critical(リリース不可)/ Major(修正必須)/ Minor(任意)

| # | Severity | 場所 | 内容 | 対応 | 再検査 |
|---|----------|------|------|------|--------|
| 1 | Minor | qrdrop(ビルド toolchain Go 1.26.1) | `govulncheck` が標準ライブラリの既知脆弱性 11 件(GO-2026-4865/4866/4870/4946/4947/4971/4980/4982/5037/5039/5856)をシンボル到達として検出。いずれも go1.26.2〜1.26.5 で修正済み。**ただし本アプリは平文 HTTP 提供のため crypto/tls・crypto/x509 系(#1,#3,#7,#8,#9,#10 ほか)は実際には未到達(http.Serve を非 TLS リスナで使用)、html/template 系(#4,#5,#11)は静的文言のみ描画で XSS 不成立、net.Listen の Windows NUL panic(#6)はポートが攻撃者非制御、net/textproto(#2)はローカルログにのみ影響**。実害到達は限定的。 | 推奨: リリースビナリを go1.26.5+ のパッチ済み toolchain でビルド(コード変更不要)。判定は G3 に委ねる | — |
| 2 | Minor | internal/transfer/handler.go:44-57, 64-67 | `uniquePath` の存在確認と `os.OpenFile(O_EXCL)` の間に TOCTOU があり、同一セッションで同名ファイルを**同時**アップロードすると 2 件目が `.part` 名衝突で 500 になりうる。単発・逐次では問題なし。 | 任意(衝突時リトライ or 一意 tmp 名)。個人利用の同時同名は稀 | — |
| 3 | Minor | internal/guard/guard.go:74-81, internal/web/server.go:79-85 | `/admin/share`・`/admin/session/revoke` 等の状態変更 POST に CSRF/Origin 検査がない。ループバック判定のみのため、PC のブラウザで開いた悪意あるサイトがポートを推測できればセッション失効(DoS)や共有項目の混入を誘発しうる。応答は CORS 不許可で読めず、`/s/` はトークン必須のため影響は限定的。 | 任意(Origin/Sec-Fetch-Site 検査 or CSRF トークン) | — |
| 4 | Minor | docs/agent-work/decision-log.md(DEC-004) vs design-doc §7 / internal/guard/guard.go | DEC-004 は多層防御の第2層として「接続元 IP が RFC1918 かつ**サーバと同一サブネット**であること」を掲げるが、設計書 §7-2 も実装(`IsPrivate`)もサブネット一致までは検査せず private 範囲判定のみ。REQ-004 の受け入れ基準(LAN 外遮断)は充足しているが、append-only の decision-log と設計/実装の記述に差がある。 | 任意(設計 §7 に整合注記 or DEC で追記して整合を明示) | — |

## 判定

- [x] Critical / Major がゼロ(該当なし。全指摘 Minor)
- **判定**: **リリース可**

> 5 観点すべてを検査し、Critical・Major はゼロ。ビルド/整形/vet/staticcheck/全 43 テストを Reviewer 自身が再実行して再現・合格を確認した。test-report.md(合格・Must 全 Pass)との矛盾はない(test-report の対象コミット 5817f44 は P4 マージ前だが、本レビューは main a96e252 で全テスト再現・合格を確認済み)。

## 申し送り(Minor 残項目)

- **指摘 #1(依存脆弱性)**: 実害到達は限定的でコード起因ではないが、リリースビナリは go1.26.5+ のパッチ済み toolchain でビルドすることを G3 で推奨。go.mod の宣言(`go 1.22`)は下限であり、ビルド環境の toolchain 選択の問題。
- **指摘 #2・#3・#4**: いずれも個人利用・同一 LAN の想定脅威モデル(ADR-002/DEC-005 で受容済み)では低リスク。対応可否は G3 で確認。
- **DEC-007 の設計補完**: 追加 API(`/admin/info`・`/admin/interface`・`/admin/items`)は design-doc §5 の API 一覧に未記載だが DEC-007(FYI)で記録済み・LanGuard でループバック限定。セキュリティ境界の変更はなく整合上の問題なし(将来 design-doc §5 へ追記すると可読性向上)。
- **トレーサビリティ表**: requirements-spec.md §5 の TC 列が未記入(`—`)のまま。実際の対応は test-plan.md §4 にあり実害なし。次回ドキュメント更新時に埋めると整合性が上がる。
