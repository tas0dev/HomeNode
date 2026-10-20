# HomeNode システム仕様書

バージョン：0.1.0  
リポジトリ：`tas0dev/HomeNode`  
バックエンド・Agent実装言語：Go  
対象：Raspberry Pi OS Lite 64-bit / Windows 11 x64

## 1. 製品定義

HomeNodeは、Raspberry Pi 4を常時稼働する管理ノードとして利用し、家庭内のWindowsデスクトップPCをWebブラウザから管理するシステムである。スマートフォンまたはノートPCから、電源操作、稼働状態の確認、ハードウェア監視、Sunshineの状態確認、ログ閲覧を行う。

Raspberry Piにはモニターを接続せず、Wi-Fi経由でルーターに接続する。Windows PCは常時有線LANに接続する。遠隔アクセスはTailscale公式サービスを利用し、Web UIと管理APIを公開インターネットへ直接開放しない。

初期バージョンでは管理対象をWindows PC 1台に限定する。

## 2. 動作環境

| 役割 | 機器・OS | 接続 |
|---|---|---|
| 管理ノード | Raspberry Pi 4 Model B / 8GB / Raspberry Pi OS Lite 64-bit | Wi-Fi |
| 管理対象 | Windows 11 / Ryzen 7 9700X / Radeon RX 7700 XT / 32GB | 有線LAN |
| 操作クライアント | スマートフォン、WindowsノートPCのブラウザ | Tailscale経由 |
| 配信ホスト | Windows 11のSunshine | 有線LAN |
| ゲームクライアント | WindowsノートPCのMoonlight | LAN / Wi-Fi / Tailscale |

Raspberry PiにはTailscale、SSH、Docker Engine、Docker Composeを導入する。Windows側にはHomeNode AgentとTailscaleを導入する。ブラウザの管理画面アクセスも同一Tailnet内に限定する。

Sunshine/Moonlightの映像・入力ストリームはHomeNode Serverを経由しない。HomeNodeは接続先の確認と稼働状態の把握だけを担当する。

## 3. コンポーネント

### 3.1 HomeNode Server

Raspberry PiのDockerコンテナ内で動作するGoサービスである。

担当機能：

- HTTP APIとWeb UIの提供
- WebSocketによるブラウザ向けイベント配信
- Windows Agentの接続と認証
- PCの稼働状態判定と状態遷移管理
- Wake-on-LANのマジックパケット送信
- Agentへのシャットダウン・再起動指示
- Windowsの最新監視値・短期時系列データの保持
- Raspberry Pi自身のリソース監視
- Sunshineの状態情報の集約
- 監査ログ・システムログの記録と提供

### 3.2 HomeNode Agent

Windowsサービスとしてログオン前から起動するGo製ネイティブプログラムである。

担当機能：

- WindowsのCPU、GPU、RAM、ネットワーク監視
- Sunshineのサービス・プロセス監視
- Serverへのハートビートと監視値送信
- 認証済みの電源操作要求の実行
- 接続断からの自動再接続
- Windows Event LogとServer向けイベントの記録

Agentは任意のコマンド実行、ファイル操作、シェル実行、画面キャプチャーのAPIを公開しない。受け付ける電源操作は `shutdown` と `restart` のみとする。

### 3.3 HomeNode Web

Serverが配信するレスポンシブWebアプリケーションである。`Overview`、`Metrics`、`Logs` の3タブを持ち、詳細な表示・操作は別文書 `ui-design.md` に従う。

## 4. ネットワーク設計

1. Raspberry Piは家庭内Wi-Fiへ接続する。
2. Windows PCはルーター配下の有線LANに接続する。
3. Raspberry PiとWindows PCは同一の家庭内IPネットワークで到達可能とする。
4. Raspberry Pi、Windows PC、スマートフォンは同じTailnetに参加する。
5. HomeNodeのWeb UIはPi上のTailscale ServeによるHTTPSで公開し、Tailnetからのみ到達可能とする。
6. Windows AgentはPiのTailscaleホスト名に対して外向きに接続し、認証付きWebSocket接続を維持する。
7. 電源OFFのWindows PCを起動する場合は、Piが家庭内ネットワークへ直接Wake-on-LANを送信する。

Windows Agentは外部からの着信ポートを開放する必要がない。Tailscaleが停止している間はAgentとの制御通信が利用できなくなるが、Piは継続稼働し、復帰後に再接続を受け入れる。

Wake-on-LANは同じLAN内のブロードキャストまたは適切に設定された送信先に依存する。Wi-Fiの端末間分離・ブロードキャスト制限が有効な場合は動作しないことがある。対象PCはBIOSとNICでWoLを有効化し、Windowsの高速スタートアップなど起動阻害要因を調整する。

## 5. Windows PCの状態管理

| 状態 | 意味 |
|---|---|
| `OFFLINE` | Agent接続がなく、稼働を確認できない。電源OFFとは断定しない |
| `STARTING` | WoL要求が受理され、Agent接続を待っている |
| `ONLINE` | 有効なAgentハートビートを継続受信している |
| `SHUTTING_DOWN` | Windowsに正常終了を要求し、切断を待っている |
| `RESTARTING` | Windowsに再起動を要求し、切断・再接続を待っている |
| `UNREACHABLE` | ONLINEから予期せずAgentの到達性を失った |

初期値：ハートビート間隔5秒、到達性タイムアウト15秒、起動タイムアウト120秒、再起動タイムアウト180秒。値は設定から変更可能とする。

`ONLINE` の判定には有効なAgentハートビートを必須とする。ICMP Pingの応答だけでオンラインと判定しない。起動直後やシャットダウン時には、それぞれ遷移状態を維持する。電源OFFをネットワークだけで確実に検出できないため、`OFFLINE` はあくまで到達性がない状態として表示する。

## 6. 電源管理

### 6.1 Power On

Web UIから認証・認可済みの起動要求を受け取ったServerは、設定された有線LAN MACアドレス宛てのWake-on-LANマジックパケットを送信する。送信後、状態を `STARTING` に変更し、Agentの接続を待つ。

マジックパケットの送信成功はOSの起動成功を意味しない。Agentの接続が確認されて初めて起動成功とする。タイムアウト時は失敗イベントを記録し、監視状態へ戻す。

### 6.2 Shutdown / Restart

Serverは既存の認証済みAgent WebSocket接続を通じて操作命令を送る。Agentは命令種別、要求ID、認証状態を検証してからWindowsの正常なシャットダウンまたは再起動APIを呼び出す。強制終了フラグは指定しない。

Agentが要求受付を通知したことと、Windowsの終了・再起動が実際に完了したことは区別する。Serverは状態遷移とAgentの切断・再接続を監視する。

同じ要求IDの命令は重複実行しない。電源遷移中は別の電源要求を拒否する。電源操作の実行者、対象、操作種別、要求ID、結果を監査ログに記録する。

## 7. ハードウェア監視

### 7.1 監視項目

| 対象 | 項目 | 単位 |
|---|---|---|
| Windows CPU | 使用率、温度、動作クロック、モデル名 | %、℃、MHz、文字列 |
| Windows GPU | 使用率、温度、動作クロック、VRAM使用量・総量、消費電力、モデル名 | %、℃、MHz、byte、W、文字列 |
| Windows Memory | 総容量、使用容量、空き容量、使用率 | byte、% |
| Windows Network | インターフェース名、IPv4、リンク速度、送受信速度、累計送受信量 | 文字列、bit/s、byte/s、byte |
| Raspberry Pi | CPU使用率・温度、メモリ使用量・総量、稼働時間、Wi-Fi状態 | %、℃、byte、秒、文字列 |

監視は1秒間隔を基本とする。Windows Agentが時刻付きサンプルをServerへ送信する。Serverは最新サンプルと直近120秒分をメモリ上のリングバッファに保持し、Web UIには60秒間のグラフを提供する。監視値の長期保存は行わない。

### 7.2 データ取得の実装

- CPU使用率・クロック、RAM、ネットワーク統計はWindowsの正式なシステムAPI・パフォーマンスカウンターを優先する。
- GPU利用率・VRAMはWindowsのGPU関連APIとAMD側の計測手段を利用する。
- CPU温度、GPU温度・クロック・消費電力などは対応するハードウェア監視プロバイダーから取得する。
- AMD Radeon RX 7700 XTとRyzen 7 9700Xで、採用プロバイダーがどのセンサーを取得できるか検証する。
- カーネルドライバーを必要とする方式は明示的な導入手順とし、署名されていないドライバーを自動導入しない。
- 対応していない項目、センサー取得失敗、値の無効状態は `null` として送信する。`0` で代用しない。

ネットワーク速度は連続サンプル間の累積バイト差分と経過時間から算出する。カウンターがリセットされた場合、負の速度を算出しない。時刻はUTCのRFC 3339形式を使用する。

### 7.3 データ欠損

15秒以上ハートビートが途絶えた場合、監視値は古いものとして扱い、UIに更新停止を示す。Agent再接続後は新たなサンプルから再開する。欠損期間を補間して実測値に見せない。

## 8. Sunshine連携

AgentはSunshineのサービスまたはプロセス状態を監視し、可能な範囲でサービスの待受状態も確認する。Sunshineのプロセス稼働と、Moonlightクライアントからの接続可否は別項目として扱う。

Sunshineの状態は次の基準で分類する。`STOPPED` はプロセスまたはサービスの停止、`RUNNING` は起動を確認したが待受確認ができていない状態、`READY` は起動と指定ポートの待受を確認した状態、`UNKNOWN` はAgentから確認できない状態を示す。`READY` でもMoonlight側から接続できるとは限らない。

ServerはSunshineの状態、Windows PCのLAN IPアドレス、Tailscale経由の接続先をWeb UIへ提供する。ユーザーはMoonlightに登録する接続先を確認・コピーできる。HomeNodeはストリーミング処理、ゲーム起動、Moonlightの代替機能を提供しない。

## 9. 通信プロトコル

### 9.1 Web API

すべてのエンドポイントは `/api/v1` を接頭辞とする。

| メソッド | パス | 機能 |
|---|---|---|
| `GET` | `/system/status` | Windowsの状態・最終接続時刻 |
| `GET` | `/system/metrics` | 最新監視値 |
| `GET` | `/system/metrics/history` | 直近の短期監視値 |
| `POST` | `/system/power/on` | Wake-on-LAN送信 |
| `POST` | `/system/power/shutdown` | 正常シャットダウン要求 |
| `POST` | `/system/power/restart` | 正常再起動要求 |
| `GET` | `/sunshine/status` | Sunshineの状態と接続先 |
| `GET` | `/node/metrics` | Raspberry Piの監視値 |
| `GET` | `/logs` | ログ一覧、重要度・発生元の絞り込み |
| `GET` | `/health` | プロセスの生存確認 |
| `GET` | `/events` | ブラウザ向けWebSocket |

電源操作APIは成功時に即座に操作完了とせず、要求IDと `accepted` 状態を返す。実際の進捗は状態APIとWebSocketイベントから取得する。通信エラーの形式は `code`、`message`、`request_id` を持つJSONとする。

### 9.2 Agentとの接続

AgentはPiのTailnet上のHTTPSホスト名へ外向き接続し、`/agent/v1/connect` で認証付きWebSocketセッションを確立する。接続上で次のメッセージ種別を扱う。

- `hello`：Agent ID、プロトコルバージョン、システム情報
- `heartbeat`：生存通知
- `telemetry`：監視サンプル
- `sunshine_status`：Sunshineの状態
- `command`：Serverからの電源操作要求
- `command_ack`：Agentによる受付・拒否結果
- `event`：Agent側イベント

すべてのメッセージにプロトコルバージョン、送信時刻、必要に応じて要求IDを付与する。未知のメッセージ種別はログに残し、安全に拒否する。受信したJSONのサイズとフィールド値を検証する。

### 9.3 メッセージとメトリクスのデータ形式

Agentメッセージは次の共通フィールドを持つJSONオブジェクトとする。

| フィールド | 型 | 内容 |
|---|---|---|
| `version` | 整数 | プロトコルバージョン（初期値1） |
| `type` | 文字列 | メッセージ種別 |
| `agent_id` | 文字列 | Agent識別子 |
| `timestamp` | RFC 3339 UTC文字列 | 送信時刻 |
| `request_id` | 文字列・省略可能 | 操作要求との対応付け |
| `payload` | オブジェクト | メッセージ本体 |

監視値のJSONキーと単位を次のように統一する。

- `cpu.usage_percent`、`cpu.temperature_c`、`cpu.clock_mhz`
- `gpu.usage_percent`、`gpu.temperature_c`、`gpu.clock_mhz`、`gpu.vram_used_bytes`、`gpu.vram_total_bytes`、`gpu.power_w`
- `memory.used_bytes`、`memory.total_bytes`、`memory.available_bytes`
- `network.rx_bytes_per_sec`、`network.tx_bytes_per_sec`、`network.rx_total_bytes`、`network.tx_total_bytes`、`network.link_speed_bps`

各指標は数値または `null` とし、取得元が返す非数値、負の累計値、不正な範囲の値をそのまま配信しない。値が欠けた場合の原因は必要に応じて `availability` フィールドに含める。時刻・システム情報は取得元が持つ時刻と区別し、サーバー受信時刻も記録する。

`command` には `operation`（`shutdown` または `restart`）と `request_id` を必須とする。`command_ack` は同じ要求ID、`accepted` の真偽、失敗時の理由コードを返す。電源操作の最終結果は状態遷移イベントから判定する。

### 9.4 Web UIへの更新

Web UIはWebSocketで状態遷移、最新メトリクス、ログイベントを受信する。切断時は指数バックオフで再接続し、再接続後にREST APIからスナップショットを取得する。接続復旧まで古い値を現在値として表示しない。

## 10. 認証・セキュリティ

1. Web UIの公開範囲はTailnet内に限定する。ルーターでHomeNodeの管理ポートを転送しない。
2. Tailscale接続に加え、HomeNode Webへのログイン認証を実装する。管理者パスワードはArgon2idでハッシュ化し、平文で保存しない。
3. Webログイン後は `Secure`、`HttpOnly`、`SameSite` 属性を持つセッションCookieを使用する。電源操作にはCSRF対策と再認証可能なアクセス制御を適用する。
4. Agentは個別の認証トークンを用いて接続する。トークンはユーザー用セッションと区別し、Windows側ではOSが提供する秘密情報保護機構を利用する。
5. Agent接続にはTLSを使用する。Serverはトークンの照合と接続元の認可を行う。
6. シャットダウン・再起動などの操作は、許可済みの種類だけを受け入れる。任意のコマンド実行を禁止する。
7. APIの入力値検証、要求頻度制限、重複実行防止を行う。
8. ログにトークン、パスワード、Cookie、秘密鍵を出力しない。
9. デフォルト設定では公開インターネットからHomeNodeに直接接続できないようにする。

## 11. ログ仕様

ログレベルは `DEBUG`、`INFO`、`WARN`、`ERROR` とする。イベントは `timestamp`、`level`、`component`、`event_id`、`message`、`fields` を含む構造化JSONとして記録する。

記録対象は次のとおりとする。

- ServerおよびAgentの起動・終了・異常
- Agentの接続、切断、再接続
- PCの状態遷移
- Wake-on-LAN送信結果
- シャットダウン・再起動の要求と受付結果
- Sunshine状態変化
- 認証失敗
- ネットワーク障害、監視データ取得失敗

Serverのログを永続ボリュームに保持する。ログを最大サイズと保持日数でローテーションし、容量の無制限増加を防ぐ。初期値は合計20MiB、最大7日間とする。ブラウザにはページ単位で提供する。監視メトリクスの長期履歴は保存しない。

## 12. 配置・自動起動・復旧

### 12.1 Raspberry Pi

PiではTailscale・SSH・Docker Engineをホストのsystemdサービスとして有効化する。HomeNode ServerはDocker Composeで実行し、`restart: unless-stopped` を設定する。

Dockerコンテナはホスト側でループバックアドレスにのみWeb APIを待ち受け、Tailscale ServeからHTTPS経由で公開する。Wake-on-LANのブロードキャスト送信を単純化するため、PiではDockerのhostネットワークを使用する。プロセスは可能な限り非rootで実行し、不要なLinux Capabilityを付与しない。

設定ファイル・秘密情報・ログは永続ボリュームで保持する。起動時にネットワークが未確立でもプロセスは終了せず、接続可能になるまで再試行する。

### 12.2 Windows

AgentはWindowsサービスとして自動起動する。システム再起動後もユーザーログインを必要としない。Windows Service Control Managerに失敗時の再起動を設定し、Agent自身もServerとの通信断から再接続する。

Agentは終了要求を受け取った際に新規サンプル送信を停止し、可能な範囲でログ・イベントを確定してから終了する。Windowsサービスに付与する権限は、ハードウェア監視および電源操作に必要な範囲とする。

## 13. 設定

Serverの設定項目：

- 管理対象PCの表示名
- 有線LAN MACアドレス
- Wake-on-LANの送信先ブロードキャストアドレス・ポート
- Agentの識別子と認可情報
- ハートビート間隔、状態タイムアウト
- テレメトリ取得間隔、保持サンプル数
- ログレベル、ログ容量、保持日数
- HTTP待受アドレス

Agentの設定項目：

- ServerのTailnet HTTPS URL
- Agentの識別子
- 接続・再接続の設定
- 監視対象ネットワークインターフェース
- センサープロバイダーの選択

秘密情報は設定ファイルに平文で固定せず、別の保護された保存領域に置く。設定の初期値は保守的にし、無認証の管理APIは有効にしない。

## 14. リポジトリ構成

```text
HomeNode/
├── go.mod
├── go.sum
├── cmd/
│   ├── server/
│   │   └── main.go
│   └── agent/
│       └── main.go
├── internal/
│   ├── server/
│   │   ├── api/
│   │   ├── auth/
│   │   ├── state/
│   │   ├── power/
│   │   ├── telemetry/
│   │   └── logging/
│   ├── agent/
│   │   ├── collector/
│   │   ├── sunshine/
│   │   ├── power/
│   │   └── service/
│   ├── protocol/
│   └── config/
├── web/
│   ├── src/
│   └── package.json
├── deploy/
│   ├── docker/
│   ├── systemd/
│   └── windows/
├── docs/
│   ├── specification.md
│   └── ui-design.md
├── compose.yaml
├── config.example.yaml
└── README.md
```

Go標準ライブラリのHTTP機能と`log/slog`を基本とし、WebSocketには最小限の実績あるライブラリを利用する。Web UIはTypeScriptで実装し、ビルド済み静的ファイルをGo Serverから配信する。Pi上でのGoビルドが可能であることを前提とし、依存関係を過度に増やさない。

## 15. 実装フェーズ

| フェーズ | 実装内容 | 完了条件 |
|---|---|---|
| 1. 基盤 | Go共通プロトコル、Agent/Server接続、認証、Docker・Windowsサービス、自動起動 | PiとWindows間で認証済みの接続が確立する |
| 2. 電源管理 | 状態機械、WoL、安全な終了・再起動、二重要求防止 | 3つの電源操作と状態遷移を検証できる |
| 3. 監視 | Windows CPU/GPU/RAM/Network、Pi監視、Sunshine状態、欠損処理 | 取得可能な各メトリクスが正しい単位と時刻で届く |
| 4. Web UI | Overview、Metrics詳細、Logs、操作確認、リアルタイム更新 | スマホから主要機能を使える |
| 5. 接続支援 | Sunshine接続先表示、LAN/Tailscale利用案内 | Moonlight登録に必要な情報を確認できる |
| 6. 安定化 | 切断復旧、サービス異常復旧、負荷・長時間稼働試験 | 再起動・通信断から手動介入なしで復旧する |

Windows Agentを先に実装し、監視値と電源操作の実機検証を進める。ServerはAgentと並行して接続・状態管理の最小機能から構築する。

## 16. 対象外

v0.1.0では以下を実装しない。

- Raspberry Piに接続した常設モニターへの表示・HDMI制御
- 強制電源断、GPIOリレーによる電源スイッチ制御
- 任意のリモートシェル、ファイル管理
- ゲーム映像の中継、独自ストリーミングプロトコル
- ゲームの起動指示・プロセス操作
- 複数Windows PCの同時管理
- 監視メトリクスの長期履歴・分析DB
- 独自VPNサーバー、Headscaleの運用

## 17. 受け入れ条件

1. PiがWi-Fi接続のまま、再起動後にTailscale・HomeNode Serverを自動起動できる。
2. スマホからTailscale経由でログインし、管理画面へ接続できる。
3. PCの状態を `ONLINE`、`OFFLINE`、移行状態、`UNREACHABLE` として扱える。
4. 電源ON、シャットダウン、再起動の要求と結果を区別して確認できる。
5. CPU・GPU・RAM・Network・Piの監視値を1秒単位で更新でき、利用できないセンサーを誤表示しない。
6. Sunshineの起動状態とMoonlight接続先を確認できる。
7. ログをWeb UIで閲覧し、重要度とコンポーネントで絞り込める。
8. Agent停止、Wi-Fi切断、Tailscale切断、Pi再起動の後に自動復旧できる。
9. 認証のない電源操作や、Tailnet外からのWeb APIアクセスを拒否できる。
10. Raspberry Pi・Windows PCのいずれにも管理用の常設モニターを必要としない。
