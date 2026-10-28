# Сборка don't sleep в один exe (Windows, через Docker).
#
#   .\build.ps1
#
# Результат: dist\dontsleep.exe

$ErrorActionPreference = 'Stop'

$root = $PSScriptRoot
$work = $root -replace '\\', '/'

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Error 'Docker не найден. Установите Docker Desktop.'
}

& docker info *> $null
if ($LASTEXITCODE -ne 0) {
    Write-Error 'Docker не запущен. Запустите Docker Desktop и повторите.'
}

New-Item -ItemType Directory -Force -Path (Join-Path $root 'dist') | Out-Null

Write-Host 'Сборка dist\dontsleep.exe ...' -ForegroundColor Cyan

$dockerArgs = @(
    'run', '--rm'
    '-v', "${work}:/work"
    '-v', 'go-mod-cache:/go/pkg/mod'
    '-w', '/work'
    '-e', 'GOOS=windows'
    '-e', 'GOARCH=amd64'
    '-e', 'CGO_ENABLED=0'
    '-e', 'GOPROXY=https://goproxy.cn,direct'
    'golang:1.26'
    'go', 'build', '-ldflags=-s -w', '-o', 'dist/dontsleep.exe', '.'
)

& docker @dockerArgs
if ($LASTEXITCODE -ne 0) {
    Write-Error 'Сборка завершилась с ошибкой.'
}

$exe = Join-Path $root 'dist\dontsleep.exe'
if (-not (Test-Path $exe)) {
    Write-Error 'Бинарник не появился: dist\dontsleep.exe'
}

$size = [math]::Round((Get-Item $exe).Length / 1MB, 1)
Write-Host "Готово: dist\dontsleep.exe ($size МБ)" -ForegroundColor Green
