param(
    [Parameter(Mandatory = $true)]
    [string]$MySqlDatabase,

    [Parameter(Mandatory = $false)]
    [string]$MySqlHost = "127.0.0.1",

    [Parameter(Mandatory = $false)]
    [string]$MySqlUser = "root",

    [Parameter(Mandatory = $false)]
    [string]$MySqlPassword = "",

    [Parameter(Mandatory = $false)]
    [int]$SourceYear = 2026,

    [Parameter(Mandatory = $false)]
    [string]$SourceUrl = "https://atcddd.fhi.no/atc_ddd_index/",

    [Parameter(Mandatory = $false)]
    [string]$MySqlSslMode = "DISABLED"
)

$ErrorActionPreference = "Stop"

function Get-MySqlSslArguments {
    $clientVersion = (& mysql --version 2>&1 | Out-String)
    if ($clientVersion -notmatch "MariaDB") {
        return @("--ssl-mode=$MySqlSslMode")
    }

    switch ($MySqlSslMode.ToUpperInvariant()) {
        "DISABLED" { return @("--skip-ssl") }
        "PREFERRED" { return @() }
        "REQUIRED" { return @("--ssl") }
        "VERIFY_CA" { return @("--ssl", "--ssl-verify-server-cert") }
        "VERIFY_IDENTITY" { return @("--ssl", "--ssl-verify-server-cert") }
        default { throw "Unsupported MySqlSslMode: $MySqlSslMode" }
    }
}

$mySqlSslArguments = @(Get-MySqlSslArguments)

function Invoke-MySql {
    param([string]$Sql)

    $arguments = @("-N", "-B", "-h", $MySqlHost, "-u", $MySqlUser)
    if ($MySqlPassword -ne "") {
        $arguments += "-p$MySqlPassword"
    }
    $arguments += $mySqlSslArguments
    $arguments += @($MySqlDatabase, "-e", $Sql)
    $output = & mysql @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "MariaDB/MySQL command failed with exit code $LASTEXITCODE"
    }
    return $output
}

function Get-AtcLevel {
    param([string]$Code)

    switch ($Code.Length) {
        1 { return 1 }
        3 { return 2 }
        4 { return 3 }
        5 { return 4 }
        7 { return 5 }
        default { return 0 }
    }
}

function Get-AtcHierarchyCodes {
    param([string]$Code)

    $normalizedCode = $Code.Trim().ToUpperInvariant()
    foreach ($length in @(1, 3, 4, 5, 7)) {
        if ($length -gt $normalizedCode.Length) {
            break
        }
        $normalizedCode.Substring(0, $length)
    }
}

function Get-WhoAtcLabel {
    param([string]$Code)

    $url = "$SourceUrl`?code=$Code"
    $response = Invoke-WebRequest -Uri $url -UseBasicParsing
    $content = $response.Content
    $content = $content -replace "(?i)<br\s*/?>", "`n"
    $content = $content -replace "(?i)</(tr|p|div|h[1-6])>", "`n"
    $content = $content -replace "(?i)</t[dh]>", " | "
    $content = $content -replace "<[^>]+>", " "
    $content = [System.Net.WebUtility]::HtmlDecode($content)

    $escapedCode = [Regex]::Escape($Code)
    foreach ($line in ($content -split "`r?`n")) {
        $normalizedLine = ($line -replace "\s+", " ").Trim()
        $tableMatch = [Regex]::Match($normalizedLine, "(?i)^$escapedCode\s*\|\s*([^|]+?)\s*\|")
        if ($tableMatch.Success) {
            return $tableMatch.Groups[1].Value.Trim()
        }

        $headingMatch = [Regex]::Match($normalizedLine, "(?i)^$escapedCode\s+(.+?)$")
        if ($headingMatch.Success) {
            return $headingMatch.Groups[1].Value.Trim()
        }
    }

    return ""
}

Invoke-MySql "CREATE TABLE IF NOT EXISTS who_atc_mapping (atc_code varchar(16) NOT NULL, level int NOT NULL, label_en varchar(255) NULL, source_year int NULL, source_url varchar(512) NULL, PRIMARY KEY (atc_code)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;"

$sourceCodes = Invoke-MySql "SELECT DISTINCT Key_ATC FROM FAM_DB WHERE Key_ATC IS NOT NULL AND Key_ATC <> '' UNION SELECT DISTINCT Key_ATCA FROM FAM_DB WHERE Key_ATCA IS NOT NULL AND Key_ATCA <> '' ORDER BY 1;"
$codes = @(
    $sourceCodes |
        ForEach-Object { Get-AtcHierarchyCodes -Code $_ } |
        Sort-Object -Unique |
        Sort-Object @{ Expression = { Get-AtcLevel -Code $_ } }, @{ Expression = { $_ } }
)
foreach ($code in $codes) {
    $code = $code.Trim()
    if ($code -eq "") {
        continue
    }

    $label = Get-WhoAtcLabel -Code $code
    if ($label -eq "") {
        Write-Warning "No WHO ATC label found for $code"
        continue
    }

    $safeCode = $code -replace "'", "''"
    $safeLabel = $label -replace "'", "''"
    $safeUrl = $SourceUrl -replace "'", "''"
    $level = Get-AtcLevel -Code $code

    Invoke-MySql "INSERT INTO who_atc_mapping (atc_code, level, label_en, source_year, source_url) VALUES ('$safeCode', $level, '$safeLabel', $SourceYear, '$safeUrl') ON DUPLICATE KEY UPDATE level = VALUES(level), label_en = VALUES(label_en), source_year = VALUES(source_year), source_url = VALUES(source_url);"
    Write-Output "Imported $code $label"
    Start-Sleep -Milliseconds 500
}
