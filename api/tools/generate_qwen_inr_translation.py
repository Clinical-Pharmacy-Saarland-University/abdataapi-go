#!/usr/bin/env python3

import argparse
import csv
import json
import os
import subprocess
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import date
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
ENV_PATH = ROOT / "api" / ".env"
OUTPUT_PATH = ROOT / "api" / "data" / "inr_translation_qwen.csv"
CHECKPOINT_PATH = ROOT / "api" / "data" / "inr_translation_qwen.checkpoint.json"


def read_env(path: Path) -> dict[str, str]:
    settings = {}
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        settings[key.strip()] = value.strip().strip('"').strip("'")
    return settings


def fetch_rows(settings: dict[str, str]) -> list[dict[str, str]]:
    database = settings.get("MYSQL_DB_NAME") or settings.get("MYSQL_DATABASE")
    command = [
        "mysql", "-N", "-B", "--skip-ssl", "--default-character-set=utf8mb4",
        "-h", settings["MYSQL_HOST"], "-u", settings["MYSQL_USER"],
    ]
    if settings.get("MYSQL_PORT"):
        command.extend(["-P", settings["MYSQL_PORT"]])
    command.extend(
        [
            database,
            "-e",
            """
SELECT INR_DB.Key_IND, INR_DB.Zaehler,
       COALESCE(IND_DB.Name, ''), INR_DB.Name
FROM INR_DB
LEFT JOIN IND_DB ON IND_DB.Key_IND = INR_DB.Key_IND
WHERE INR_DB.Name IS NOT NULL AND TRIM(INR_DB.Name) <> ''
ORDER BY INR_DB.Key_IND, INR_DB.Zaehler;
""",
        ]
    )
    environment = os.environ.copy()
    environment["MYSQL_PWD"] = settings.get("MYSQL_PASSWORD", "")
    result = subprocess.run(
        command, check=True, capture_output=True, encoding="utf-8", env=environment
    )
    rows = []
    for line in result.stdout.splitlines():
        key_ind, counter, context, name_de = line.split("\t", 3)
        rows.append(
            {
                "id": f"{key_ind}:{counter}",
                "key_ind": key_ind,
                "zaehler": counter,
                "indication_class_de": context,
                "name_de": name_de,
            }
        )
    return rows


def extract_mapping(content: str) -> dict[str, str]:
    start = content.find("{")
    end = content.rfind("}")
    if start < 0 or end <= start:
        raise ValueError("Qwen did not return a JSON object")
    value = json.loads(content[start : end + 1])
    if not isinstance(value, dict) or not all(
        isinstance(key, str) and isinstance(item, str) and item.strip()
        for key, item in value.items()
    ):
        raise ValueError("Qwen returned an invalid translation mapping")
    return {key: item.strip() for key, item in value.items()}


def request_translations(
    rows: list[dict[str, str]], base_url: str, model: str, timeout: int
) -> dict[str, str]:
    inputs = [
        {
            "id": row["id"],
            "term_de": row["name_de"],
            "context_de": row["indication_class_de"],
        }
        for row in rows
    ]
    payload = {
        "model": model,
        "temperature": 0,
        "max_tokens": 4096,
        "messages": [
            {
                "role": "system",
                "content": (
                    "You translate German ABDA pharmaceutical classification terms into concise "
                    "standard clinical English. Use current medical and pharmacological nomenclature. "
                    "The context disambiguates but must not add meaning. Preserve all qualifiers. "
                    "Do not return codes, notes, or explanations."
                ),
            },
            {
                "role": "user",
                "content": (
                    "/no_think Return one JSON object that maps every exact id to one English "
                    f"translation. Inputs: {json.dumps(inputs, ensure_ascii=False)}"
                ),
            },
        ],
    }
    request = urllib.request.Request(
        f"{base_url.rstrip('/')}/chat/completions",
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={"Authorization": "Bearer local", "Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=timeout) as response:
        result = json.loads(response.read().decode("utf-8"))
    mapping = extract_mapping(result["choices"][0]["message"]["content"])
    expected = {row["id"] for row in rows}
    if set(mapping) != expected:
        raise ValueError(
            f"Qwen key mismatch: missing={sorted(expected - set(mapping))}, "
            f"extra={sorted(set(mapping) - expected)}"
        )
    return mapping


def translate_batch(
    rows: list[dict[str, str]], base_url: str, model: str, timeout: int
) -> dict[str, str]:
    try:
        return request_translations(rows, base_url, model, timeout)
    except Exception:
        if len(rows) == 1:
            raise
        midpoint = len(rows) // 2
        result = translate_batch(rows[:midpoint], base_url, model, timeout)
        result.update(translate_batch(rows[midpoint:], base_url, model, timeout))
        return result


def load_checkpoint(path: Path, model: str) -> dict[str, str]:
    if not path.exists():
        return {}
    value = json.loads(path.read_text(encoding="utf-8"))
    if value.get("model") != model:
        return {}
    translations = value.get("translations", {})
    if not isinstance(translations, dict):
        raise ValueError(f"Invalid checkpoint: {path}")
    return {str(key): str(item) for key, item in translations.items()}


def write_checkpoint(path: Path, model: str, translations: dict[str, str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(
        json.dumps(
            {"model": model, "translations": translations},
            ensure_ascii=False,
            indent=2,
        )
        + "\n",
        encoding="utf-8",
        newline="\n",
    )
    temporary.replace(path)


def load_reviews(path: Path, model: str) -> dict[str, dict[str, str]]:
    reviews = {}
    if not path.exists():
        return reviews
    with path.open(encoding="utf-8", newline="") as existing:
        for row in csv.DictReader(existing):
            if row.get("translation_source") != model:
                continue
            reviews[f"{row['key_ind']}:{row['zaehler']}"] = row
    return reviews


def write_output(
    path: Path,
    rows: list[dict[str, str]],
    translations: dict[str, str],
    model: str,
    previous: dict[str, dict[str, str]],
) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fields = [
        "key_ind", "zaehler", "indication_class_de", "name_de", "name_en",
        "translation_source", "translation_date", "confidence_level",
        "validation_status", "review_source", "review_date", "reviewed_name_en",
        "review_notes",
    ]
    with path.open("w", encoding="utf-8", newline="") as output:
        writer = csv.DictWriter(output, fieldnames=fields, lineterminator="\n")
        writer.writeheader()
        for row in rows:
            prior = previous.get(row["id"], {})
            translation = translations[row["id"]]
            unchanged = prior.get("name_de") == row["name_de"] and prior.get("name_en") == translation
            writer.writerow(
                {
                    **{key: row[key] for key in fields[:4]},
                    "name_en": translation,
                    "translation_source": model,
                    "translation_date": prior.get("translation_date") or date.today().isoformat(),
                    "confidence_level": prior.get("confidence_level", "pending") if unchanged else "pending",
                    "validation_status": prior.get("validation_status", "pending_review") if unchanged else "pending_review",
                    "review_source": prior.get("review_source", "") if unchanged else "",
                    "review_date": prior.get("review_date", "") if unchanged else "",
                    "reviewed_name_en": prior.get("reviewed_name_en", "") if unchanged else "",
                    "review_notes": prior.get("review_notes", "") if unchanged else "",
                }
            )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Generate Qwen translations for INR_DB")
    parser.add_argument("--env", type=Path, default=ENV_PATH)
    parser.add_argument("--output", type=Path, default=OUTPUT_PATH)
    parser.add_argument("--checkpoint", type=Path, default=CHECKPOINT_PATH)
    parser.add_argument("--base-url", default=os.environ.get("OPENAI_BASE_URL"))
    parser.add_argument("--model", default="qwen3.8-flash-next")
    parser.add_argument("--batch-size", type=int, default=20)
    parser.add_argument("--workers", type=int, default=4)
    parser.add_argument("--timeout", type=int, default=120)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if not args.base_url:
        raise SystemExit("Set OPENAI_BASE_URL or pass --base-url")
    if args.batch_size < 1:
        raise SystemExit("--batch-size must be positive")
    if args.workers < 1:
        raise SystemExit("--workers must be positive")

    rows = fetch_rows(read_env(args.env))
    translations = load_checkpoint(args.checkpoint, args.model)
    batches = []
    for offset in range(0, len(rows), args.batch_size):
        batch = [
            row
            for row in rows[offset : offset + args.batch_size]
            if row["id"] not in translations
        ]
        if batch:
            batches.append(batch)

    with ThreadPoolExecutor(max_workers=args.workers) as executor:
        futures = {
            executor.submit(
                translate_batch, batch, args.base_url, args.model, args.timeout
            ): batch
            for batch in batches
        }
        for future in as_completed(futures):
            translations.update(future.result())
            write_checkpoint(args.checkpoint, args.model, translations)
            print(f"Translated {len(translations)}/{len(rows)}", flush=True)

    previous = load_reviews(args.output, args.model)
    write_output(args.output, rows, translations, args.model, previous)
    print(f"Wrote {len(rows)} pending-review rows to {args.output}")


if __name__ == "__main__":
    main()
