# PowerShell script to initialize Kopiochi boilerplate for a new project
# Usage: .\scripts\init.ps1 -ProjectName "myapi" -Author "John Doe" -ModulePath "github.com/john/myapi"

param(
    [Parameter(Mandatory=$true)]
    [string]$ProjectName,
    
    [Parameter(Mandatory=$false)]
    [string]$Author = (whoami),
    
    [Parameter(Mandatory=$false)]
    [string]$ModulePath = "github.com/$Author/$ProjectName",
    
    [Parameter(Mandatory=$false)]
    [string]$DBName = $ProjectName.ToLower(),
    
    [Parameter(Mandatory=$false)]
    [switch]$RemoveExample = $true,
    
    [Parameter(Mandatory=$false)]
    [switch]$ResetGit = $true
)

$ErrorActionPreference = "Stop"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Kopiochi Project Initializer" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Project Name : $ProjectName" -ForegroundColor Yellow
Write-Host "Author       : $Author" -ForegroundColor Yellow
Write-Host "Module Path  : $ModulePath" -ForegroundColor Yellow
Write-Host "DB Name      : $DBName" -ForegroundColor Yellow
Write-Host "Remove Example: $RemoveExample" -ForegroundColor Yellow
Write-Host "Reset Git    : $ResetGit" -ForegroundColor Yellow
Write-Host ""

# Confirm
$confirm = Read-Host "Continue? (y/n)"
if ($confirm -ne 'y') {
    Write-Host "Aborted." -ForegroundColor Red
    exit
}

$OldModule = "github.com/sujanto-gaws/kopiochi"
$OldName = "kopiochi"
$RootPath = Split-Path -Parent $MyInvocation.MyCommand.Path | Split-Path -Parent

function Replace-InFile {
    param([string]$FilePath, [string]$Old, [string]$New)
    
    if (Test-Path $FilePath) {
        $content = Get-Content $FilePath -Raw -Encoding UTF8
        $content = $content -replace [regex]::Escape($Old), $New
        $content | Set-Content $FilePath -Encoding UTF8 -NoNewline
        Write-Host "  Updated: $FilePath" -ForegroundColor Gray
    }
}

Write-Host ""
Write-Host "[1/5] Updating module paths..." -ForegroundColor Green

# Update go.mod
Replace-InFile "$RootPath\go.mod" $OldModule $ModulePath

# Update all Go source files
Get-ChildItem -Path "$RootPath" -Recurse -Filter "*.go" | ForEach-Object {
    Replace-InFile $_.FullName $OldModule $ModulePath
}

Write-Host ""
Write-Host "[2/5] Updating configuration..." -ForegroundColor Green

# Update config files
Replace-InFile "$RootPath\config\default.yaml" $OldName $ProjectName.ToLower()
Replace-InFile "$RootPath\config\default.yaml" $OldDBName $DBName
Replace-InFile "$RootPath\.env.example" $OldName $ProjectName.ToLower()

# Update README
Replace-InFile "$RootPath\README.md" "Kopiochi" $ProjectName
Replace-InFile "$RootPath\README.md" $OldName $ProjectName.ToLower()
Replace-InFile "$RootPath\README.md" "sujanto-gaws/kopiochi" "$Author/$ProjectName"

# Update PLUGIN_GUIDE.md
Replace-InFile "$RootPath\PLUGIN_GUIDE.md" "Kopiochi" $ProjectName

# Update cmd/api/main.go (CLI name)
Replace-InFile "$RootPath\cmd\api\main.go" $OldName $ProjectName.ToLower()

# Update generator defaults
Replace-InFile "$RootPath\cmd\generator\main.go" $OldModule $ModulePath
Replace-InFile "$RootPath\cmd\generator\README.md" $OldModule $ModulePath

Write-Host ""
Write-Host "[3/5] Updating copyright information..." -ForegroundColor Green

$CurrentYear = (Get-Date).Year
Replace-InFile "$RootPath\LICENSE" "Sujanto" $Author
Replace-InFile "$RootPath\LICENSE" "2026" $CurrentYear.ToString()

Write-Host ""
Write-Host "[4/5] tidying Go module..." -ForegroundColor Green

Push-Location $RootPath
go mod tidy
Pop-Location

if ($RemoveExample) {
    Write-Host ""
    Write-Host "[5/5] Removing example domain (user CRUD)..." -ForegroundColor Green
    
    # Remove example domain files
    $examplePaths = @(
        "$RootPath\internal\domain\user",
        "$RootPath\internal\application\user",
        "$RootPath\internal\infrastructure\http\handlers\user.go",
        "$RootPath\internal\infrastructure\persistence\repository\user.go"
    )
    
    foreach ($path in $examplePaths) {
        if (Test-Path $path) {
            Remove-Item -Path $path -Recurse -Force
            Write-Host "  Removed: $path" -ForegroundColor Gray
        }
    }
    
    # Remove health check handler only if it's mixed with user handlers
    # (Keep it as it's useful)
}

if ($ResetGit) {
    Write-Host ""
    Write-Host "Resetting Git history..." -ForegroundColor Green
    
    Push-Location $RootPath
    
    # Remove .git directory
    if (Test-Path ".git") {
        Remove-Item -Path ".git" -Recurse -Force
        Write-Host "  Removed .git directory" -ForegroundColor Gray
    }
    
    # Reinitialize
    git init | Out-Null
    Write-Host "  Initialized new Git repository" -ForegroundColor Gray
    
    Pop-Location
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Initialization Complete! 🎉" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host "  1. Review the changes:" -ForegroundColor White
Write-Host "     git status" -ForegroundColor Gray
Write-Host ""
Write-Host "  2. Make initial commit:" -ForegroundColor White
Write-Host "     git add ." -ForegroundColor Gray
Write-Host "     git commit -m 'Initial commit: $ProjectName boilerplate'" -ForegroundColor Gray
Write-Host ""
Write-Host "  3. Start developing:" -ForegroundColor White
Write-Host "     go run ./cmd/api serve" -ForegroundColor Gray
Write-Host ""
Write-Host "  4. Generate new domains:" -ForegroundColor White
Write-Host "     go run ./cmd/generator -domain Product -fields `'name:string,price:float64`'" -ForegroundColor Gray
Write-Host ""
