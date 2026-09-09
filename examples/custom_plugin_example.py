"""Example third-party plugin for FlowForge."""
from core.plugin_api import FlowForgePlugin, register


class GrayscalePlugin(FlowForgePlugin):
    name = "grayscale"

    def run(self, input_path: str, output_path: str, **options) -> None:
        from PIL import Image
        Image.open(input_path).convert("L").save(output_path)


register(GrayscalePlugin())
