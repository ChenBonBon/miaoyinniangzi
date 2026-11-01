#!/usr/bin/env python3
"""Fetch the pinned upstream development artifact and prepare local libmpv.

This does not install IINA or system packages. Public release compliance remains
separate; see docs/third-party/libmpv.md before distributing any build.
"""
import argparse
import hashlib
import os
from pathlib import Path
import platform
import subprocess
import sys
import tempfile
import urllib.request

URL = 'https://github.com/iina/iina/releases/download/v1.5.0/IINA.v1.5.0.dmg'
SHA256 = '8fad50479b10e09645053457cec46bd53d054ced2242cfe68f74586735a20dc7'


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--arch', choices=['arm64', 'x86_64'], default=platform.machine())
    args = parser.parse_args()
    if sys.platform != 'darwin':
        raise RuntimeError('macOS preparation tool only')
    root = Path(__file__).resolve().parent.parent
    cache = root / 'build/deps'
    cache.mkdir(parents=True, exist_ok=True)
    artifact = cache / 'IINA.v1.5.0.dmg'
    if not artifact.exists():
        with tempfile.NamedTemporaryFile(dir=cache, delete=False) as output:
            temporary = Path(output.name)
            try:
                with urllib.request.urlopen(URL, timeout=60) as response:
                    size = 0
                    while chunk := response.read(1024 * 1024):
                        size += len(chunk)
                        if size > 128 * 1024 * 1024:
                            raise ValueError('upstream artifact exceeds limit')
                        output.write(chunk)
                output.flush()
                os.fsync(output.fileno())
                if digest(temporary) != SHA256:
                    raise ValueError('upstream artifact checksum mismatch')
                output.close()
                os.replace(temporary, artifact)
            finally:
                temporary.unlink(missing_ok=True)
    if digest(artifact) != SHA256:
        raise ValueError('cached artifact checksum mismatch; refuse to load')
    with tempfile.TemporaryDirectory(prefix='miaoyin-mpv-') as directory:
        mount = Path(directory) / 'volume'
        subprocess.run(['hdiutil', 'attach', '-readonly', '-nobrowse', '-mountpoint', str(mount), str(artifact)], check=True)
        try:
            subprocess.run([sys.executable, str(root / 'tools/bundle_libmpv_macos.py'),
                            '--source-app', str(mount / 'IINA.app'), '--arch', args.arch,
                            '--destination', str(cache / 'libmpv/Frameworks')], check=True)
        finally:
            subprocess.run(['hdiutil', 'detach', str(mount)], check=True)


if __name__ == '__main__':
    main()
