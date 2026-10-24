#!/usr/bin/env python3
"""Recheck the report's direct declarations, selector differences and objects.

Uses the installed Xcode SDKs. Writes compiler outputs to a temporary directory;
does not invoke CoreSimulator or download anything. JSON on stdout is evidence,
and any mismatch with observations.json causes a nonzero exit.
"""
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile


def run(*args):
    return subprocess.run(args, check=True, text=True, capture_output=True).stdout


def documents(text):
    decoder = json.JSONDecoder()
    offset = 0
    while offset < len(text):
        if text[offset].isspace():
            offset += 1
            continue
        value, offset = decoder.raw_decode(text, offset)
        yield value


root = Path(__file__).resolve().parent
expected = json.loads((root / 'observations.json').read_text())
probe = root / 'probe.m'
assert hashlib.sha256(probe.read_bytes()).hexdigest() == expected['toolchain']['source_sha256']
compiler = run('xcrun', '--find', 'clang').strip()
version = expected['toolchain']['sdk_version']
output = Path(tempfile.mkdtemp(prefix='go-ios-vm-ast-audit-'))
results = {'xcode': run('xcodebuild', '-version').strip(),
           'compiler': run(compiler, '--version').strip(),
           'output_directory': str(output), 'targets': [], 'checks': []}
selectors = {}
for sdk, platform, suffix, platform_id in (
    ('iphonesimulator', 'iPhoneSimulator', '-simulator', 7),
    ('iphoneos', 'iPhoneOS', '', 2),
):
    sysroot = Path(run('xcrun', '--sdk', sdk, '--show-sdk-path').strip())
    target = f'arm64-apple-ios{version}{suffix}'
    common = [compiler, '-target', target, '-isysroot', str(sysroot),
              '-fobjc-arc', '-fblocks', '-fno-modules']
    result = {'platform': platform, 'target': target, 'sysroot': str(sysroot), 'definitions': []}
    for relative, digest in expected['input_sha256'][platform].items():
        assert hashlib.sha256((sysroot / relative).read_bytes()).hexdigest() == digest, relative
    for symbol, kind in [('UIWindow', 'ObjCInterfaceDecl'),
                         ('CAMetalLayer', 'ObjCInterfaceDecl'),
                         ('MTLDevice', 'ObjCProtocolDecl')]:
        ast = run(*common, '-fsyntax-only', str(probe), '-Xclang', '-ast-dump=json',
                  '-Xclang', f'-ast-dump-filter={symbol}')
        (output / f'{sdk}-{symbol}.json').write_text(ast)
        roots = list(documents(ast))
        definition = max((node for node in roots if node.get('name') == symbol
                          and node.get('kind') == kind), key=lambda node: len(node.get('inner', [])))
        members = definition.get('inner', [])
        properties = [node for node in members if node['kind'] == 'ObjCPropertyDecl']
        methods = [node for node in members if node['kind'] == 'ObjCMethodDecl']
        record = {'symbol': symbol, 'direct_properties': len(properties),
                  'direct_methods': len(methods), 'json_documents': len(roots)}
        prior = next(row for row in expected['textual_ast_definitions']
                     if row['platform'] == platform and row['symbol'] == symbol)
        for field in ('direct_properties', 'direct_methods'):
            assert record[field] == prior[field], (platform, symbol, field, record[field], prior[field])
        if 'json_documents' in prior:
            assert record['json_documents'] == prior['json_documents']
        result['definitions'].append(record)
        if symbol == 'MTLDevice':
            selectors[platform] = {node['name'] for node in methods}
    obj = output / f'{sdk}.o'
    run(*common, '-c', str(probe), '-o', str(obj))
    metadata = run('otool', '-l', str(obj))
    (output / f'{sdk}-load-commands.txt').write_text(metadata)
    build = re.search(r'cmd LC_BUILD_VERSION\s+cmdsize \d+\s+platform (\d+)\s+minos ([\d.]+)\s+sdk ([\d.]+)', metadata)
    assert build and int(build[1]) == platform_id and build[2] == version and build[3] == version
    result['build_version'] = {'platform': int(build[1]), 'minos': build[2], 'sdk': build[3]}
    results['targets'].append(result)
diff = {'device_only_selectors': sorted(selectors['iPhoneOS'] - selectors['iPhoneSimulator']),
        'simulator_only_selectors': sorted(selectors['iPhoneSimulator'] - selectors['iPhoneOS'])}
for key, value in diff.items():
    assert value == expected['metal_device_protocol_diff'][key], key
results['selector_diff'] = diff
results['checks'] = ['probe digest', 'SDK/header digests', 'six direct-definition counts',
                     'available JSON document counts', 'exact Metal selector differences',
                     'two object build-platform/minimum/SDK versions']
results['not_rechecked'] = ['module-enabled dumps', 'selected-shape normalization',
                           'availability payloads', 'target macros', 'runtime behavior']
print(json.dumps(results, indent=2))
