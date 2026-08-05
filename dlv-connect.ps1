# 查找 ai-disk-cleaner 进程
$process = Get-Process -Name "ai-disk-cleanner-dev" -ErrorAction SilentlyContinue

if ($null -eq $process) {
    Write-Host "ai-disk-cleaner.exe not running"
    exit 1
}

$processId = $process.Id

Write-Host "Found ai-disk-cleaner.exe PID: $processId"

# 启动 dlv attach
dlv `
    --listen=:2345 `
    --headless=true `
    --api-version=2 `
    --check-go-version=false `
    --only-same-user=false `
    attach $processId