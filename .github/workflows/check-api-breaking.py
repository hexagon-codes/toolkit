#!/usr/bin/env python3

from __future__ import annotations

import argparse
import difflib
import json
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


def has_compatible_changes(report: str) -> bool:
    package = ""
    compatible = False
    found = False
    for line in report.replace("\r\n", "\n").splitlines():
        if line.startswith("# "):
            package = line
            compatible = False
        elif line == "## compatible changes":
            if not package or package == "# summary":
                raise SystemExit("Compatible API section has no package header.")
            compatible = True
        elif line.startswith("## "):
            compatible = False
        elif compatible and line.strip():
            found = True
    return found


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("report", type=Path)
    parser.add_argument("baseline", type=Path)
    parser.add_argument("--allow-empty", action="store_true")
    parser.add_argument("--json-output", type=Path)
    args = parser.parse_args()

    report_path = args.report
    baseline_path = args.baseline
    report = read_text(report_path, "gorelease report")
    actual = extract_breaking_sections(report)
    compatible = has_compatible_changes(report)
    expected = read_text(baseline_path, "breaking API baseline").replace("\r\n", "\n")
    if extract_breaking_sections(expected) != expected:
        raise SystemExit("Breaking API baseline is not canonical.")
    if actual != expected:
        # patch 发布对比上一 tag 无 incompatible 变更时允许空报告；
        # 有 incompatible 变更的发布仍必须与 baseline 完全一致。
        if args.allow_empty and actual == "":
            print("No breaking API changes detected.")
        else:
            sys.stderr.writelines(
                difflib.unified_diff(
                    expected.splitlines(keepends=True),
                    actual.splitlines(keepends=True),
                    fromfile=str(baseline_path),
                    tofile=str(report_path),
                )
            )
            raise SystemExit("Unapproved breaking API changes detected.")
    else:
        print("Breaking API baseline matched.")
    if args.json_output:
        args.json_output.write_text(json.dumps({"has_breaking": bool(actual),
                                               "has_compatible": compatible}) + "\n")


if __name__ == "__main__":
    main()
