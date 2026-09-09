"""Unit tests for core.presets."""
import unittest
import os
from core.presets import save_preset, load_preset, list_presets


class TestPresets(unittest.TestCase):
    def test_save_and_load_roundtrip(self):
        save_preset("unit_test", {"name": "unit_test", "action": "noop"})
        loaded = load_preset("unit_test")
        self.assertEqual(loaded["action"], "noop")
        self.assertIn("unit_test", list_presets())
        os.remove(os.path.join("presets", "unit_test.json"))


if __name__ == "__main__":
    unittest.main()
