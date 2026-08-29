param(
    [Parameter(Mandatory = $true)]
    [string]$CsvPath,

    [Parameter(Mandatory = $true)]
    [string]$MySqlDatabase,

    [Parameter(Mandatory = $false)]
    [string]$MySqlHost = "127.0.0.1",

    [Parameter(Mandatory = $false)]
    [string]$MySqlUser = "root",

    [Parameter(Mandatory = $false)]
    [string]$MySqlPassword = "",

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

$requiredColumns = @("atc_code", "level", "label_en", "source_year", "source_url")
$rows = Import-Csv -LiteralPath $CsvPath
if ($rows.Count -eq 0) {
    throw "CSV contains no rows: $CsvPath"
}

$columns = $rows[0].PSObject.Properties.Name
foreach ($column in $requiredColumns) {
    if ($columns -notcontains $column) {
        throw "CSV is missing required column: $column"
    }
}

$tempSql = New-TemporaryFile
try {
    $sql = @()
    $sql += "CREATE TABLE IF NOT EXISTS who_atc_mapping ("
    $sql += "  atc_code varchar(16) NOT NULL,"
    $sql += "  level int NOT NULL,"
    $sql += "  label_en varchar(255) NULL,"
    $sql += "  source_year int NULL,"
    $sql += "  source_url varchar(512) NULL,"
    $sql += "  PRIMARY KEY (atc_code)"
    $sql += ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;"

    foreach ($row in $rows) {
        $code = ($row.atc_code -replace "'", "''").Trim()
        $level = [int]$row.level
        $label = ($row.label_en -replace "'", "''").Trim()
        $year = if ($row.source_year) { [int]$row.source_year } else { "NULL" }
        $urlValue = if ($row.source_url) { $row.source_url } else { $SourceUrl }
        $url = ($urlValue -replace "'", "''").Trim()
        $sql += "INSERT INTO who_atc_mapping (atc_code, level, label_en, source_year, source_url) VALUES ('$code', $level, '$label', $year, '$url') ON DUPLICATE KEY UPDATE level = VALUES(level), label_en = VALUES(label_en), source_year = VALUES(source_year), source_url = VALUES(source_url);"
    }

    Set-Content -LiteralPath $tempSql -Value $sql -Encoding utf8NoBOM

    $arguments = @("-h", $MySqlHost, "-u", $MySqlUser)
    if ($MySqlPassword -ne "") {
        $arguments += "-p$MySqlPassword"
    }
    $arguments += $mySqlSslArguments
    $arguments += @($MySqlDatabase, "-e", "source $tempSql")
    & mysql @arguments
} finally {
    Remove-Item -LiteralPath $tempSql -Force -ErrorAction SilentlyContinue
}
