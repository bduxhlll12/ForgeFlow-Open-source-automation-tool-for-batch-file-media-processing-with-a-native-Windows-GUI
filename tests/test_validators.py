"""Unit tests for core.validators."""
import unittest
from core.validators import is_video, is_image


class TestValidators(unittest.TestCase):
    def test_video_extensions(self):
        self.assertTrue(is_video("clip.mp4"))
        self.assertFalse(is_video("photo.png"))

    def test_image_extensions(self):
        self.assertTrue(is_image("photo.PNG"))
        self.assertFalse(is_image("clip.mkv"))


if __name__ == "__main__":
    unittest.main()
