#!/usr/bin/env python3
"""Opt-in Cocoa/Wails smoke in an isolated throwaway .app, no player profile."""
import json
import os
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile
import uuid


def main():
    if sys.platform != "darwin":
        raise SystemExit("macOS is required")
    repo = Path(__file__).resolve().parents[1]
    env = os.environ.copy()
    env.setdefault("CGO_CFLAGS", "-mmacosx-version-min=13.0")
    env.setdefault("CGO_LDFLAGS", "-mmacosx-version-min=13.0")
    with tempfile.TemporaryDirectory(prefix="miaoyin-window-smoke-") as directory:
        app = Path(directory) / "Window Lifecycle Smoke.app"
        binary = app / "Contents/MacOS/lifecycle-smoke"
        binary.parent.mkdir(parents=True)
        subprocess.run(["go", "build", "-tags", "production", "-o", str(binary),
                        "./tools/window_lifecycle_smoke"], cwd=repo, env=env, check=True)
        (app / "Contents/Info.plist").write_bytes(plistlib.dumps({
            "CFBundleIdentifier": "com.miaoyinniangzi.window-smoke." + uuid.uuid4().hex,
            "CFBundleExecutable": binary.name,
            "CFBundleName": "Window Lifecycle Smoke",
            "CFBundlePackageType": "APPL",
            "NSHighResolutionCapable": True,
            "LSMinimumSystemVersion": "13.0",
        }))
        subprocess.run(["codesign", "--force", "--deep", "--sign", "-", str(app)],
                       check=True, capture_output=True)
        for mode in ("menu", "terminate"):
            result = subprocess.run([str(binary), mode], env=env, cwd=repo,
                                    capture_output=True, text=True, timeout=42)
            if result.returncode:
                print(result.stdout[-3000:])
                print(result.stderr[-1500:])
                raise SystemExit(result.returncode)
            reports = [line for line in result.stdout.splitlines() if line.startswith("{")]
            if not reports or not all(json.loads(reports[-1]).values()):
                raise SystemExit("native lifecycle did not finish all checks")
            print(mode + ": " + reports[-1])


if __name__ == "__main__":
    main()
