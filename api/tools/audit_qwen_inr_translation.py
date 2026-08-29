#!/usr/bin/env python3

import argparse
import csv
from collections import Counter, defaultdict
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_INPUT = ROOT / "api" / "data" / "inr_translation_qwen.csv"
REVIEWED_STATUSES = {"valid", "corrected", "rejected"}
CONFIDENCE_LEVELS = {"high", "medium", "low"}


def effective_translation(row: dict[str, str]) -> str:
    if row["validation_status"] == "corrected":
        return row["reviewed_name_en"].strip()
    return row["name_en"].strip()


def audit(rows: list[dict[str, str]]) -> list[str]:
    errors = []
    identifiers = Counter((row["key_ind"], row["zaehler"]) for row in rows)
    for identifier, count in identifiers.items():
        if count != 1:
            errors.append(f"duplicate row {identifier[0]}:{identifier[1]}")

    for row in rows:
        identifier = f"{row['key_ind']}:{row['zaehler']}"
        status = row["validation_status"]
        if status not in REVIEWED_STATUSES:
            errors.append(f"unreviewed row {identifier}")
            continue
        if row["confidence_level"] not in CONFIDENCE_LEVELS:
            errors.append(f"invalid confidence for {identifier}")
        if not row["review_source"].strip() or not row["review_date"].strip():
            errors.append(f"missing review metadata for {identifier}")
        if status == "corrected" and not row["reviewed_name_en"].strip():
            errors.append(f"missing correction for {identifier}")
        if status != "rejected" and not effective_translation(row):
            errors.append(f"missing effective translation for {identifier}")
    return errors


def consistency_rows(rows: list[dict[str, str]]) -> list[dict[str, str]]:
    grouped = defaultdict(list)
    for row in rows:
        grouped[row["name_de"].strip()].append(row)

    result = []
    for name_de, items in grouped.items():
        translations = sorted(
            {effective_translation(row) for row in items if effective_translation(row)}
        )
        if len(translations) < 2:
            continue
        result.append(
            {
                "name_de": name_de,
                "row_count": str(len(items)),
                "translations_en": " | ".join(translations),
                "row_ids": " | ".join(
                    f"{row['key_ind']}:{row['zaehler']}" for row in items
                ),
                "contexts_de": " | ".join(
                    sorted({row["indication_class_de"] for row in items})
                ),
            }
        )
    return sorted(result, key=lambda row: row["name_de"].casefold())


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Verify complete review of Qwen INR translations"
    )
    parser.add_argument("--input", type=Path, default=DEFAULT_INPUT)
    parser.add_argument("--consistency-report", type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    with args.input.open(encoding="utf-8", newline="") as input_file:
        rows = list(csv.DictReader(input_file))

    errors = audit(rows)
    inconsistencies = consistency_rows(rows)
    if args.consistency_report:
        args.consistency_report.parent.mkdir(parents=True, exist_ok=True)
        with args.consistency_report.open("w", encoding="utf-8", newline="") as output:
            fields = [
                "name_de", "row_count", "translations_en", "row_ids", "contexts_de"
            ]
            writer = csv.DictWriter(output, fieldnames=fields, lineterminator="\n")
            writer.writeheader()
            writer.writerows(inconsistencies)

    print(f"Rows: {len(rows)}")
    print(f"Review errors: {len(errors)}")
    print(f"Context-dependent translation groups: {len(inconsistencies)}")
    if errors:
        for error in errors[:20]:
            print(error)
        if len(errors) > 20:
            print(f"... {len(errors) - 20} more errors")
        raise SystemExit(1)


if __name__ == "__main__":
    main()
