import importlib.util
import sys
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("audit_qwen_inr_translation.py")
SPEC = importlib.util.spec_from_file_location("audit_qwen_inr_translation", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


def row(**overrides: str) -> dict[str, str]:
    value = {
        "key_ind": "01",
        "zaehler": "1",
        "indication_class_de": "Anasthetika",
        "name_de": "Allgemeinanasthetika",
        "name_en": "General anesthetics",
        "confidence_level": "high",
        "validation_status": "valid",
        "review_source": "openai-codex-manual-review",
        "review_date": "2026-08-29",
        "reviewed_name_en": "",
    }
    value.update(overrides)
    return value


class AuditQwenINRTranslationTest(unittest.TestCase):
    def test_audit_rejects_pending_review(self) -> None:
        errors = MODULE.audit(
            [row(validation_status="pending_review", confidence_level="pending")]
        )
        self.assertIn("unreviewed row 01:1", errors)

    def test_audit_accepts_complete_correction(self) -> None:
        errors = MODULE.audit(
            [
                row(
                    validation_status="corrected",
                    reviewed_name_en="General anaesthetics",
                )
            ]
        )
        self.assertEqual([], errors)

    def test_consistency_report_finds_different_effective_translations(self) -> None:
        rows = [
            row(),
            row(
                key_ind="02",
                zaehler="2",
                name_en="General anaesthetics",
                indication_class_de="Narkotika",
            ),
        ]
        result = MODULE.consistency_rows(rows)
        self.assertEqual(1, len(result))
        self.assertIn("General anaesthetics", result[0]["translations_en"])


if __name__ == "__main__":
    unittest.main()
