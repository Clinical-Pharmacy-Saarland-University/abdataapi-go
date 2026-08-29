import importlib.util
import sys
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("review_qwen_inr_translation.py")
SPEC = importlib.util.spec_from_file_location("review_qwen_inr_translation", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


def row(key_ind: str, counter: str) -> dict[str, str]:
    return {
        "key_ind": key_ind,
        "zaehler": counter,
        "name_en": "Candidate",
        "confidence_level": "pending",
        "validation_status": "pending_review",
        "review_source": "",
        "review_date": "",
        "reviewed_name_en": "",
        "review_notes": "",
    }


class ReviewQwenINRTranslationTest(unittest.TestCase):
    def test_review_records_valid_corrected_and_rejected_rows(self) -> None:
        rows = [row("01", "1"), row("01A", "2"), row("02", "1")]
        count = MODULE.review_rows(
            rows,
            ["01"],
            set(),
            set(),
            {"01:1": "Correction"},
            {"01A:2"},
            {"01:1"},
            "reviewer",
            "2026-08-29",
            "Reviewed.",
        )
        self.assertEqual(2, count)
        self.assertEqual("corrected", rows[0]["validation_status"])
        self.assertEqual("Correction", rows[0]["reviewed_name_en"])
        self.assertEqual("medium", rows[0]["confidence_level"])
        self.assertEqual("rejected", rows[1]["validation_status"])
        self.assertEqual("pending_review", rows[2]["validation_status"])

    def test_review_rejects_unknown_correction_id(self) -> None:
        with self.assertRaisesRegex(ValueError, "Unknown row ids"):
            MODULE.review_rows(
                [row("01", "1")],
                ["01"],
                set(),
                set(),
                {"99:1": "Correction"},
                set(),
                set(),
                "reviewer",
                "2026-08-29",
                "Reviewed.",
            )


if __name__ == "__main__":
    unittest.main()
