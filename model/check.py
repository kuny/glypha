#!/usr/bin/env python3
"""Run every Alloy command, including expected counterexamples; no dependencies."""
import json
from pathlib import Path
import re
import subprocess
import sys
import tempfile


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: python3 model/check.py /path/to/org.alloytools.alloy.dist.jar")
    jar = Path(sys.argv[1]).resolve(strict=True)
    output = Path(tempfile.mkdtemp(prefix="glypha-alloy-"))
    print(f"Alloy receipts and XML traces: {output}", flush=True)
    total = 0
    for model in sorted(Path(__file__).parent.glob("*.als")):
        expected = re.findall(
            r"^(?:check|run)\s+(\w+)\s+.*\bexpect\s+([01])\s*$",
            model.read_text(), re.MULTILINE,
        )
        result_dir = output / model.stem
        subprocess.run([
            "java", "-jar", str(jar), "exec", "-c", "*", "-t", "xml",
            "-o", str(result_dir), str(model.resolve()),
        ], check=True)
        commands = json.loads((result_dir / "receipt.json").read_text())["commands"]
        if not expected or set(commands) != {name for name, _ in expected}:
            sys.exit(f"FAIL {model.name}: incomplete command receipt or missing expect")
        for name, sat in expected:
            actual = bool(commands[name].get("solution"))
            if actual != bool(int(sat)):
                sys.exit(f"FAIL {model.name}/{name}: expected SAT={sat}, got {actual}")
            total += 1
    print(f"PASS: {total} commands matched expectations. Results: {output}")


if __name__ == "__main__":
    main()
