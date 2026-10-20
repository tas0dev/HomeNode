#Requires -RunAsAdministrator
$ErrorActionPreference = 'Stop'
Stop-Service HomeNodeAgent -ErrorAction SilentlyContinue
& sc.exe delete HomeNodeAgent
if ($LASTEXITCODE -ne 0) { throw 'サービスの削除に失敗しました。' }
Write-Host 'サービスを削除しました。設定と保護済みトークンはProgramData\HomeNodeに保持されています。'
