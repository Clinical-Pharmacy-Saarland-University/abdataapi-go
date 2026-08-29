param(
    [Parameter(Mandatory = $true)]
    [string]$CsvPath,

    [Parameter(Mandatory = $true)]
    [string]$MySqlDatabase,

    [string]$MySqlHost = "127.0.0.1",
    [string]$MySqlUser = "root",
    [string]$MySqlPassword = "",
    [string]$MySqlSslMode = "DISABLED",

    [switch]$AllowPendingReview
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

function ConvertTo-SqlString([string]$Value) {
    if ($null -eq $Value) {
        return ""
    }
    return (($Value -replace "\\", "\\\\") -replace "'", "''").Trim()
}

function ConvertTo-SqlNullString([string]$Value) {
    $escaped = ConvertTo-SqlString $Value
    return "NULLIF('$escaped', '')"
}

$requiredColumns = @(
    "key_ind", "zaehler", "name_de", "name_en", "translation_source",
    "translation_date", "confidence_level", "validation_status", "review_source",
    "review_date", "reviewed_name_en", "review_notes"
)
$rows = @(Import-Csv -LiteralPath $CsvPath)
if ($rows.Count -eq 0) {
    throw "CSV contains no rows: $CsvPath"
}
$columns = $rows[0].PSObject.Properties.Name
foreach ($column in $requiredColumns) {
    if ($columns -notcontains $column) {
        throw "CSV is missing required column: $column"
    }
}

$allowedStatuses = @("pending_review", "valid", "corrected", "rejected")
$reviewedConfidence = @("high", "medium", "low")
foreach ($row in $rows) {
    if ($allowedStatuses -notcontains $row.validation_status) {
        throw "Invalid validation_status for $($row.key_ind)/$($row.zaehler): $($row.validation_status)"
    }
    if ($row.validation_status -in @("valid", "corrected", "rejected")) {
        if (-not $row.review_source -or -not $row.review_date) {
            throw "Reviewed row lacks reviewer metadata: $($row.key_ind)/$($row.zaehler)"
        }
        if ($reviewedConfidence -notcontains $row.confidence_level) {
            throw "Reviewed row has invalid confidence_level: $($row.key_ind)/$($row.zaehler)"
        }
    }
    if ($row.validation_status -eq "corrected" -and -not $row.reviewed_name_en) {
        throw "Corrected row lacks reviewed_name_en: $($row.key_ind)/$($row.zaehler)"
    }
    if (-not $AllowPendingReview -and $row.validation_status -eq "pending_review") {
        throw "Unreviewed row cannot be imported: $($row.key_ind)/$($row.zaehler)"
    }
}

$tempSql = New-TemporaryFile
try {
    $sql = @(
        "CREATE TABLE IF NOT EXISTS TRANSLATION_INR_C (",
        "  key_ind varchar(16) NOT NULL,",
        "  zaehler int NOT NULL,",
        "  name_de varchar(500) NOT NULL,",
        "  name_en varchar(500) NOT NULL,",
        "  translation_source varchar(64) NOT NULL,",
        "  translation_date date NULL,",
        "  confidence_level varchar(32) NOT NULL,",
        "  validation_status varchar(64) NOT NULL,",
        "  review_source varchar(64) NULL,",
        "  review_date date NULL,",
        "  reviewed_name_en varchar(500) NULL,",
        "  review_notes text NULL,",
        "  PRIMARY KEY (key_ind, zaehler),",
        "  INDEX TRANSLATION_INR_C_validation_idx (validation_status)",
        ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;",
        "SET SESSION innodb_lock_wait_timeout = 10;"
    )

    foreach ($row in $rows) {
        $keyInd = ConvertTo-SqlString $row.key_ind
        $counter = [int]$row.zaehler
        $nameDe = ConvertTo-SqlString $row.name_de
        $nameEn = ConvertTo-SqlString $row.name_en
        $translationSource = ConvertTo-SqlString $row.translation_source
        $translationDate = ConvertTo-SqlNullString $row.translation_date
        $confidenceLevel = ConvertTo-SqlString $row.confidence_level
        $validationStatus = ConvertTo-SqlString $row.validation_status
        $reviewSource = ConvertTo-SqlNullString $row.review_source
        $reviewDate = ConvertTo-SqlNullString $row.review_date
        $reviewedNameEn = ConvertTo-SqlNullString $row.reviewed_name_en
        $reviewNotes = ConvertTo-SqlNullString $row.review_notes
        $sql += "INSERT INTO TRANSLATION_INR_C (key_ind, zaehler, name_de, name_en, translation_source, translation_date, confidence_level, validation_status, review_source, review_date, reviewed_name_en, review_notes) VALUES ('$keyInd', $counter, '$nameDe', '$nameEn', '$translationSource', $translationDate, '$confidenceLevel', '$validationStatus', $reviewSource, $reviewDate, $reviewedNameEn, $reviewNotes) ON DUPLICATE KEY UPDATE name_de = VALUES(name_de), name_en = VALUES(name_en), translation_source = VALUES(translation_source), translation_date = VALUES(translation_date), confidence_level = VALUES(confidence_level), validation_status = VALUES(validation_status), review_source = VALUES(review_source), review_date = VALUES(review_date), reviewed_name_en = VALUES(reviewed_name_en), review_notes = VALUES(review_notes);"
    }

    Set-Content -LiteralPath $tempSql -Value $sql -Encoding utf8NoBOM
    $arguments = @("-h", $MySqlHost, "-u", $MySqlUser)
    if ($MySqlPassword -ne "") {
        $arguments += "-p$MySqlPassword"
    }
    $arguments += @(Get-MySqlSslArguments)
    $tempSqlPath = $tempSql.FullName -replace "\\", "/"
    $arguments += @($MySqlDatabase, "-e", "source $tempSqlPath")
    & mysql @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "MySQL import failed with exit code $LASTEXITCODE"
    }

    $countArguments = @("-N", "-B", "-h", $MySqlHost, "-u", $MySqlUser)
    if ($MySqlPassword -ne "") {
        $countArguments += "-p$MySqlPassword"
    }
    $countArguments += @(Get-MySqlSslArguments)
    $countArguments += @($MySqlDatabase, "-e", "SELECT COUNT(*) FROM TRANSLATION_INR_C;")
    $importedCount = (& mysql @countArguments | Select-Object -Last 1)
    if ($LASTEXITCODE -ne 0 -or [int]$importedCount -ne $rows.Count) {
        throw "MySQL import verification failed: expected $($rows.Count) rows, found $importedCount"
    }
} finally {
    Remove-Item -LiteralPath $tempSql -Force -ErrorAction SilentlyContinue
}
