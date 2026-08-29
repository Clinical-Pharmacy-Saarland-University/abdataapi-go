#!/usr/bin/env python3

import argparse
import csv
from datetime import date
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_INPUT = ROOT / "api" / "data" / "inr_translation_qwen.csv"
DEFAULT_REVIEW_SOURCE = "openai-codex-manual-review"


def parse_corrections(values: list[str]) -> dict[str, str]:
    result = {}
    for value in values:
        identifier, separator, translation = value.partition("=")
        if not separator or not identifier.strip() or not translation.strip():
            raise ValueError(f"Invalid correction: {value}")
        result[identifier.strip()] = translation.strip()
    return result


def review_rows(
    rows: list[dict[str, str]],
    prefixes: list[str],
    exact_keys: set[str],
    exact_row_ids: set[str],
    corrections: dict[str, str],
    rejections: set[str],
    medium_confidence: set[str],
    review_source: str,
    review_date: str,
    notes: str,
) -> int:
    identifiers = {f"{row['key_ind']}:{row['zaehler']}" for row in rows}
    requested = set(corrections) | rejections | medium_confidence
    missing = requested - identifiers
    if missing:
        raise ValueError(f"Unknown row ids: {', '.join(sorted(missing))}")

    reviewed = 0
    for row in rows:
        identifier = f"{row['key_ind']}:{row['zaehler']}"
        selected = (
            identifier in exact_row_ids
            or row["key_ind"] in exact_keys
            or any(row["key_ind"].startswith(prefix) for prefix in prefixes)
        )
        if not selected:
            continue
        if row["validation_status"] != "pending_review":
            raise ValueError(f"Row was already reviewed: {identifier}")

        row["review_source"] = review_source
        row["review_date"] = review_date
        row["review_notes"] = notes
        if identifier in rejections:
            row["confidence_level"] = "low"
            row["validation_status"] = "rejected"
        elif identifier in corrections:
            row["confidence_level"] = (
                "medium" if identifier in medium_confidence else "high"
            )
            row["validation_status"] = "corrected"
            row["reviewed_name_en"] = corrections[identifier]
        else:
            row["confidence_level"] = (
                "medium" if identifier in medium_confidence else "high"
            )
            row["validation_status"] = "valid"
        reviewed += 1
    return reviewed


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Record one manual INR review batch")
    parser.add_argument("--input", type=Path, default=DEFAULT_INPUT)
    parser.add_argument("--prefix", action="append", default=[])
    parser.add_argument("--key", action="append", default=[])
    parser.add_argument("--row-id", action="append", default=[])
    parser.add_argument("--correction", action="append", default=[])
    parser.add_argument("--reject", action="append", default=[])
    parser.add_argument("--medium", action="append", default=[])
    parser.add_argument("--review-source", default=DEFAULT_REVIEW_SOURCE)
    parser.add_argument("--review-date", default=date.today().isoformat())
    parser.add_argument(
        "--notes", default="Manual review against current clinical nomenclature."
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if not args.prefix and not args.key and not args.row_id:
        raise SystemExit("Pass at least one --prefix, --key, or --row-id")
    with args.input.open(encoding="utf-8", newline="") as input_file:
        reader = csv.DictReader(input_file)
        fields = reader.fieldnames
        rows = list(reader)
    if not fields:
        raise SystemExit("Input CSV has no header")

    corrections = parse_corrections(args.correction)
    reviewed = review_rows(
        rows,
        args.prefix,
        set(args.key),
        set(args.row_id),
        corrections,
        set(args.reject),
        set(args.medium),
        args.review_source,
        args.review_date,
        args.notes,
    )
    if reviewed == 0:
        raise SystemExit("No pending rows matched the requested prefixes")

    temporary = args.input.with_suffix(args.input.suffix + ".tmp")
    with temporary.open("w", encoding="utf-8", newline="") as output:
        writer = csv.DictWriter(output, fieldnames=fields, lineterminator="\n")
        writer.writeheader()
        writer.writerows(rows)
    temporary.replace(args.input)
    print(f"Recorded review for {reviewed} rows")


if __name__ == "__main__":
    main()
