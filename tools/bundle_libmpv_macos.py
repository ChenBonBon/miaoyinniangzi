#!/usr/bin/env python3
"""Prepare relocatable macOS libmpv dylibs from an explicitly trusted app.

Development integration tool, NOT a license-complete release pipeline. Verify
upstream artifacts before use. Public distribution also requires corresponding
source and notices for the actual library builds and their dependencies.
"""
import argparse
import json
from pathlib import Path
import re
import shutil
import subprocess


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def dependencies(path, arch):
    lines = run('otool', '-arch', arch, '-L', str(path)).splitlines()[1:]
    return [line.strip().split(' (compatibility')[0] for line in lines]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-app', type=Path, required=True)
    parser.add_argument('--destination', type=Path, required=True)
    parser.add_argument('--arch', choices=['arm64', 'x86_64'], required=True)
    args = parser.parse_args()
    source = (args.source_app / 'Contents/Frameworks').resolve(strict=True)
    destination = args.destination.resolve()
    destination.mkdir(parents=True, exist_ok=True)
    pending, selected = ['libmpv.2.dylib'], {}
    while pending:
        name = pending.pop()
        if name in selected:
            continue
        if not re.fullmatch(r'[A-Za-z0-9_.+-]+\.dylib', name):
            raise ValueError('unexpected dependency name')
        path = (source / name).resolve(strict=True)
        if not path.is_relative_to(source):
            raise ValueError('dependency escapes source directory')
        deps = dependencies(path, args.arch)
        selected[name] = (path, deps)
        for dependency in deps:
            if dependency.startswith('@rpath/'):
                child = dependency.removeprefix('@rpath/')
                if child != name:
                    pending.append(child)
            elif not dependency.startswith(('/usr/lib/', '/System/Library/')):
                raise ValueError(f'non-system dependency requires explicit handling: {dependency}')
    for name, (path, deps) in selected.items():
        target = destination / name
        if target.exists():
            target.unlink()
        archs = run('lipo', '-archs', str(path)).split()
        if len(archs) > 1:
            run('lipo', str(path), '-thin', args.arch, '-output', str(target))
        elif archs == [args.arch]:
            shutil.copyfile(path, target)
        else:
            raise ValueError('requested architecture unavailable')
        target.chmod(0o755)
        command = ['install_name_tool', '-id', '@rpath/' + name]
        for dependency in deps:
            if dependency.startswith('@rpath/') and dependency != '@rpath/' + name:
                command += ['-change', dependency, '@loader_path/' + dependency.removeprefix('@rpath/')]
        run(*command, str(target))
        run('codesign', '--force', '--sign', '-', str(target))
    manifest = {'source_app': args.source_app.name, 'architecture': args.arch,
                'libraries': sorted(selected), 'release_license_audit': 'pending'}
    (destination / 'libmpv-build.json').write_text(json.dumps(manifest, indent=2) + '\n')
    print(f'Prepared {len(selected)} relocatable libraries in {destination}')


if __name__ == '__main__':
    main()
