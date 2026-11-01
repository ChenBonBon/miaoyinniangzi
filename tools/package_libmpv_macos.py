#!/usr/bin/env python3
"""Wails post-build: bundle prepared libmpv and re-sign the local test app."""
import argparse
import json
from pathlib import Path
import shutil
import subprocess


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    if binary.parent.name != 'MacOS' or binary.parent.parent.name != 'Contents':
        raise ValueError('expected a packaged macOS application executable')
    root = Path(__file__).resolve().parent.parent
    source = root / 'build/deps/libmpv/Frameworks'
    manifest = json.loads((source / 'libmpv-build.json').read_text())
    archs = subprocess.check_output(['lipo', '-archs', str(binary)], text=True).split()
    if archs != [manifest['architecture']]:
        raise ValueError('libmpv architecture does not match application')
    destination = binary.parent.parent / 'Frameworks'
    destination.mkdir(exist_ok=True)
    for name in manifest['libraries']:
        if Path(name).name != name or not name.endswith('.dylib'):
            raise ValueError('invalid library manifest')
        shutil.copy2(source / name, destination / name)
    notices = binary.parent.parent / 'Resources/third-party'
    notices.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source / 'libmpv-build.json', notices / 'libmpv-build.json')
    shutil.copy2(root / 'docs/third-party/libmpv.md', notices / 'libmpv.md')
    subprocess.run(['codesign', '--force', '--sign', '-', str(binary.parents[2])], check=True)
    subprocess.run(['codesign', '--verify', '--deep', '--strict', str(binary.parents[2])], check=True)
    print('Bundled libmpv; local ad-hoc signature verified. Release license/source audit remains pending.')


if __name__ == '__main__':
    main()
