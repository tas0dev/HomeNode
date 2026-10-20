# HomeNode

Raspberry Piを常時稼働する管理ノードにして、1台のWindows PCの電源・監視・Sunshineの接続先をブラウザから確認するサービスです。ServerとAgentはGo、Web UIはTypeScriptで実装しています。

管理画面はTailscale ServeのHTTPSで公開します。Windows Agentは外向きWebSocket接続を維持するため、Windowsへの管理用ポート開放は必要ありません。Sunshine/Moonlightのストリームは中継しません。

## 必要な環境

- Raspberry Pi OS Lite 64-bit、Docker Engine、Docker Compose、Tailscale
- Windows 11 x64、Tailscale
- ビルド用のGo 1.24以降、Node.js 24以降（DockerでServerをビルドする場合はホストへの導入不要）
- BIOS・NICのWake-on-LAN設定、同一LANに届くブロードキャスト経路

## ビルド

```sh
make all
make test
```

`build/homenode-server`と`build/homenode-agent.exe`を生成します。Pi用のServerを別のPCでビルドする場合は、Webをビルドした後に次を実行します。

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=mod -o build/homenode-server-arm64 ./cmd/server
```

Go依存関係を初めて解決する際は`go mod tidy`を実行してください。Webの実行時に外部のJavaScriptライブラリは必要ありません。`cd web && npm run check`でTypeScriptの型を検査できます。

## Raspberry Piへの導入

1. リポジトリをPiへ配置します。`deploy/systemd/homenode-compose.service`を使う場合の配置先は`/opt/HomeNode`です。
2. `config.example.yaml`を`config.yaml`へコピーし、PC名、MAC、LANのブロードキャスト、PiのTailscale HTTPS URLを設定します。`public_url`には末尾のパスを含めません。
3. Dockerイメージと認証情報を用意します。

```sh
sudo systemctl enable --now docker tailscaled ssh
cp config.example.yaml config.yaml
# config.yamlを編集してから続けてください。
docker compose build
mkdir -p secrets data
chmod 700 secrets data
HOMENODE_IMAGE=$(docker compose config --images)
read -rs -p '管理者パスワード（12文字以上）: ' HOMENODE_PASSWORD
printf '\n'
printf '%s\n' "$HOMENODE_PASSWORD" | docker run --rm -i --user "$(id -u):$(id -g)" -v "$PWD/secrets:/secrets" "$HOMENODE_IMAGE" -init-secrets /secrets
unset HOMENODE_PASSWORD
sudo chown -R 10001:10001 secrets data
chmod 600 config.yaml
sudo chown 10001:10001 config.yaml
docker compose up -d
sudo tailscale serve --bg http://127.0.0.1:8080
```

Tailscaleの案内に従ってHTTPSを有効化してください。表示されたURLを`public_url`とWindows側の`server_url`にそろえます。Funnelやルーターのポート転送は使用しません。Tailnetのアクセス制御も管理者の端末だけに制限してください。

Dockerの再起動ポリシーで自動復旧します。Compose自体をsystemdから起動する場合は次を実行します。

```sh
sudo cp deploy/systemd/homenode-compose.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now homenode-compose
```

## Windowsへの導入

ビルド済みの`build/homenode-agent.exe`と`deploy/windows/install.ps1`を含むリポジトリをWindowsへコピーします。同じTailnetに参加してから、管理者PowerShellで実行します。

```powershell
.\deploy\windows\install.ps1 -ServerURL 'https://homenode.example-tailnet.ts.net' -Interface 'Ethernet' -TailnetAddress 'desktop.example-tailnet.ts.net'
```

入力を求められたらPiの`secrets/agent.token`を入力します。トークンをコマンド引数や通常の設定ファイルには保存しません。WindowsではマシンDPAPIで暗号化し、SYSTEMとAdministratorsだけが読めるディレクトリに保管します。

Agentはログオン前からLocalSystemのWindowsサービスとして動作します。電源操作と監視に使用する権限を持ちますが、プロトコルは`shutdown`・`restart`だけを受け付けます。正常終了APIを使用し、アプリケーションの強制終了は指定しません。操作受付から15秒の猶予があります。

設定は`C:\ProgramData\HomeNode\agent.yaml`です。`network_interface`にはWindowsのインターフェース名を指定します。サービス停止後に設定を変更して再起動します。更新時もサービスを停止し、`C:\Program Files\HomeNode\homenode-agent.exe`を置き換えて再起動します。保護済みトークンと要求IDの記録は維持してください。

## ハードウェア監視

CPU・メモリ・ネットワークはOSの計測APIを利用します。GPU使用率・専用VRAM使用量はWindowsのPDHパフォーマンスカウンターを利用します。GPUが複数存在する場合は混同を避けるため自動値を無効化し、明示的なセンサーの割り当てを使用します。

CPU温度、GPU温度・クロック・消費電力・VRAM総量は、任意導入のLibreHardwareMonitorのローカルJSONエンドポイントから取得できます。HomeNodeはハードウェアドライバーを導入しません。プロバイダーが必要とするドライバーの署名と配布元を確認し、HTTPの待受をループバックに限定してください。

```yaml
sensor_url: http://127.0.0.1:8085/data.json
sensors:
  cpu.temperature_c: /amdcpu/0/temperature/0
  gpu.temperature_c: /gpu-amd/0/temperature/0
  gpu.clock_mhz: /gpu-amd/0/clock/0
  gpu.power_w: /gpu-amd/0/power/0
```

識別子は例です。実機のJSONに含まれる`SensorId`または`id`を確認し、該当センサーを割り当ててください。値のない項目は`null`で配信し、画面には`—`／`取得不可`を表示します。Ryzen 7 9700XとRX 7700 XTでのセンサー対応は実機で確認する必要があります。

## 操作と状態

ブラウザでTailscaleのHTTPS URLを開き、管理者パスワードでログインします。OverviewのPowerから操作を選び、確認画面でパスワードを再入力すると要求します。

- `OFFLINE`：Agent接続なし。物理的な電源OFFとは断定しません。
- `STARTING`：WoLを送信し、ハートビートを待っています。
- `ONLINE`：有効なハートビートを受信しています。
- `SHUTTING_DOWN`：正常終了要求後、Agentの切断を待っています。
- `RESTARTING`：切断後、異なる起動時刻を持つAgentのハートビートを待っています。
- `UNREACHABLE`：予期せずAgentの到達性を失いました。電源操作を無効にします。

Server受付、Agent受付、切断・復帰の確認を別々に表示します。シャットダウン後の切断は物理電源断の証明ではありません。タイムアウト、拒否、通信障害は操作失敗として記録します。電源操作中は別要求を拒否し、Agentは要求IDを永続化して再実行を防ぎます。

メトリクスは直近120秒をメモリに保持し、画面には60秒を表示します。詳細グラフは最大2種類の単位を左右の独立軸に表示します。ログはページごとに50件、最大20MiB・7日間で保持します。トークン、パスワード、Cookieはログに出力しません。

## API

API一覧とデータ形式は[システム仕様書](docs/specification.md)、画面の定義は[UI設計書](docs/ui-design.md)を参照してください。

認証用の追加エンドポイントは`POST /api/v1/auth/login`、`GET /api/v1/auth/session`、`POST /api/v1/auth/logout`です。ログイン・電源操作のJSONには`password`を渡します。電源操作とログアウトではセッションAPIが返す`csrf_token`を`X-CSRF-Token`ヘッダーに設定します。変更要求は`Origin`とJSON Content-Typeも検証します。

CookieはSecure・HttpOnly・SameSite=Strict、管理APIの待受はループバックのみです。TLSはTailscale Serveが終端します。公開URLとHostが異なる要求、認証されていない要求を拒否します。ヘルスチェックのみログイン不要です。

## 検証

Webの表示値テストは`cd web && npm test`で実行します。

Goのテストは`make test`で実行します。認証・CSRF・Agent接続・状態遷移・WoLパケット・監視値の検証を含みます。

ブラウザテストはWebディレクトリで次を実行します。

```sh
cd web
npm install --no-save --package-lock=false playwright@1.62.1
npx playwright install chromium
npm run build
npm run test:ui
```

ブラウザテストの入力データはテスト専用です。本番では模擬データを生成しません。Pi・Windows間の接続、WoL、実際の終了・再起動、センサープロバイダーの対応は対象機での確認が必要です。

Agentの`hello`には再起動の確認に使う`boot_time`（UTC Unix秒）を含めます。Serverは有効なハートビートに`heartbeat_ack`で応答します。どちらも共通エンベロープとプロトコルバージョン1を使用します。

## ライセンス

既存の[GNU GPL v3](license)を適用します。
