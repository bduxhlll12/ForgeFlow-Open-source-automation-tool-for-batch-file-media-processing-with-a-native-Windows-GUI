"""Headless CLI entry point (no GUI) for automation/CI use."""
import argparse

from core.presets import load_preset
from processing.video_ops import transcode


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="flowforge-cli")
    parser.add_argument("input", help="input file path")
    parser.add_argument("output", help="output file path")
    parser.add_argument("--preset", default="default")
    return parser


def run(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    preset = load_preset(args.preset)
    transcode(args.input, args.output, codec=preset["options"].get("codec", "libx264"))
    return 0
