# 一键运行：确保前端已构建，构建后端并启动
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

# 前端若未构建，则先构建
if (-not (Test-Path 'frontend\dist\index.html')) {
    Write-Host "frontend/dist 未构建，开始构建前端..."
    Set-Location 'frontend'
    if (-not (Test-Path 'node_modules')) { npm install }
    npm run build
    Set-Location ..
}

Set-Location 'backend'
go build -o anonfile.exe .
Write-Host "服务启动中，访问 http://127.0.0.1:8080 （管理后台 /admin）"
.\anonfile.exe
