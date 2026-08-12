#!/usr/bin/env python3

from __future__ import annotations

import difflib
from pathlib import Path
import sys


def extract_breaking_sections(report: str) -> str:
    sections: list[list[str]] = []
    current_package = ""
    current_section: list[str] | None = None
    seen_packages: set[str] = set()

    for line in report.replace("\r\n", "\n").splitlines():
        if line.startswith("# "):
            current_package = line
            current_section = None
            continue
        if line == "## incompatible changes":
            if not current_package or current_package == "# summary":
                raise SystemExit("Incompatible API section has no package header.")
            if current_package in seen_packages:
                raise SystemExit(f"Duplicate incompatible API package: {current_package}")
            seen_packages.add(current_package)
            current_section = [current_package, line]
            sections.append(current_section)
            continue
        if line.startswith("## "):
            current_section = None
            continue
        if current_section is not None and line:
            current_section.append(line)

    if not sections:
        return ""
    for section in sections:
        if len(section) < 3:
            raise SystemExit(f"Incompatible API section is empty: {section[0]}")
    return "\n\n".join("\n".join(section) for section in sections) + "\n"


def read_text(path: Path, label: str) -> str:
    try:
        return path.read_text(encoding="utf-8")
    except OSError as error:
        raise SystemExit(f"Failed to read {label} {path}: {error}") from error


def main() -> None:
    allow_empty = len(sys.argv) == 4 and sys.argv[3] == "--allow-empty"
    if len(sys.argv) not in (3, 4):
        raise SystemExit("Usage: check-api-breaking.py REPORT BASELINE [--allow-empty]")

    report_path = Path(sys.argv[1])
    baseline_path = Path(sys.argv[2])
    actual = extract_breaking_sections(read_text(report_path, "gorelease report"))
    expected = read_text(baseline_path, "breaking API baseline").replace("\r\n", "\n")
    if extract_breaking_sections(expected) != expected:
        raise SystemExit("Breaking API baseline is not canonical.")
    if actual != expected:
        # patch 发布（如 v0.3.1）对比上一 tag 无 incompatible 变更时允许空报告；
        # 有 incompatible 变更的发布仍必须与 baseline 完全一致。
        if allow_empty and actual == "":
            print("No breaking API changes; approved for a patch release.")
            return
        sys.stderr.writelines(
            difflib.unified_diff(
                expected.splitlines(keepends=True),
                actual.splitlines(keepends=True),
                fromfile=str(baseline_path),
                tofile=str(report_path),
            )
        )
        raise SystemExit("Unapproved breaking API changes detected.")
    print("Breaking API baseline matched.")


if __name__ == "__main__":
    main()
