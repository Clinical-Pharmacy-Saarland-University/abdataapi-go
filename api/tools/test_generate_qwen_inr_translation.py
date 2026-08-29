import csv
import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("generate_qwen_inr_translation.py")
SPEC = importlib.util.spec_from_file_location("generate_qwen_inr_translation", MODULE_PATH)
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class QwenINRTranslationTest(unittest.TestCase):
    def test_extract_mapping_requires_nonempty_string_values(self) -> None:
        result = MODULE.extract_mapping('prefix {"01:1":"Anesthetics"} suffix')
        self.assertEqual({"01:1": "Anesthetics"}, result)

        with self.assertRaisesRegex(ValueError, "invalid translation mapping"):
            MODULE.extract_mapping('{"01:1":""}')

    def test_write_output_preserves_only_unchanged_review(self) -> None:
        rows = [
            {
                "id": "01:1",
                "key_ind": "01",
                "zaehler": "1",
                "indication_class_de": "Anästhetika",
                "name_de": "Allgemeinanästhetika",
            }
        ]
        prior = {
            "01:1": {
                "name_de": "Allgemeinanästhetika",
                "name_en": "General anesthetics",
                "translation_date": "2026-08-29",
                "confidence_level": "high",
                "validation_status": "valid",
                "review_source": "openai-codex-manual-review",
                "review_date": "2026-08-29",
                "reviewed_name_en": "",
                "review_notes": "Preferred ATC wording.",
            }
        }
        with tempfile.TemporaryDirectory() as temporary_directory:
            output = Path(temporary_directory) / "translations.csv"
            MODULE.write_output(
                output,
                rows,
                {"01:1": "General anesthetics"},
                "qwen3.8-flash-next",
                prior,
            )
            with output.open(encoding="utf-8", newline="") as input_file:
                unchanged = next(csv.DictReader(input_file))
            MODULE.write_output(
                output,
                rows,
                {"01:1": "General anaesthetics"},
                "qwen3.8-flash-next",
                prior,
            )
            with output.open(encoding="utf-8", newline="") as input_file:
                changed = next(csv.DictReader(input_file))

        self.assertEqual("valid", unchanged["validation_status"])
        self.assertEqual("pending_review", changed["validation_status"])
        self.assertEqual("", changed["review_source"])


if __name__ == "__main__":
    unittest.main()
