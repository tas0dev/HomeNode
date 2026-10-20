#Requires -RunAsAdministrator
param(
 [Parameter(Mandatory=$true)][string]$ServerURL,
 [string]$AgentID = 'desktop',
 [string]$Interface = 'Ethernet',
 [string]$TailnetAddress = '',
 [string]$Binary = "$PSScriptRoot\..\..\build\homenode-agent.exe"
)
$ErrorActionPreference = 'Stop'
$parsed = [uri]$ServerURL
if ($parsed.Scheme -ne 'https' -or $parsed.AbsolutePath -ne '/') { throw 'ServerURLにはHTTPSのオリジンを指定してください。' }
$program = Join-Path $env:ProgramFiles 'HomeNode'
$data = Join-Path $env:ProgramData 'HomeNode'
$secrets = Join-Path $data 'secrets'
if (Get-Service HomeNodeAgent -ErrorAction SilentlyContinue) { throw '既存サービスがあります。更新手順に従って停止・置換してください。' }
New-Item -ItemType Directory -Force $program,$data,$secrets | Out-Null
# Machine DPAPI requires an ACL: only SYSTEM and Administrators may read files.
& icacls.exe $data /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'ACLの設定に失敗しました。' }
$exe = Join-Path $program 'homenode-agent.exe'
Copy-Item $Binary $exe
$configPath = Join-Path $data 'agent.yaml'
$config = @{
 server_url = $ServerURL.TrimEnd('/'); agent_id = $AgentID;
 secrets_dir = $secrets.Replace('\','/'); network_interface = $Interface;
 heartbeat_interval = '5s'; reachability_timeout = '15s'; telemetry_interval = '1s';
 reconnect_max = '30s'; sunshine_port = 47989; tailnet_address = $TailnetAddress;
 gpu_model = 'AMD Radeon RX 7700 XT'; sensor_url = ''; sensors = @{}
}
# JSON is also valid YAML; avoid quoting mistakes in paths and interface names.
$config | ConvertTo-Json -Depth 4 | Set-Content -Encoding UTF8 $configPath
$secure = Read-Host 'Piのsecrets/agent.tokenの値を入力してください' -AsSecureString
$ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
try {
 [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr) | & $exe -config $configPath -enroll-token
 if ($LASTEXITCODE -ne 0) { throw 'トークンの保護・保存に失敗しました。' }
} finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr); $secure.Dispose() }
if (-not [System.Diagnostics.EventLog]::SourceExists('HomeNodeAgent')) { New-EventLog -LogName Application -Source HomeNodeAgent }
New-Service -Name HomeNodeAgent -DisplayName 'HomeNode Agent' -BinaryPathName "`"$exe`" -config `"$configPath`"" -StartupType Automatic -Description 'HomeNode telemetry and approved power operations' | Out-Null
& sc.exe failure HomeNodeAgent reset= 86400 actions= restart/5000/restart/15000/restart/30000
if ($LASTEXITCODE -ne 0) { throw 'サービス復旧設定に失敗しました。' }
& sc.exe failureflag HomeNodeAgent 1
if ($LASTEXITCODE -ne 0) { throw 'サービス復旧設定に失敗しました。' }
Start-Service HomeNodeAgent
Write-Host 'HomeNode Agentを開始しました。'
