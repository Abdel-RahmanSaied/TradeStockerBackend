#!/usr/bin/env python3
"""Install this planning pack locally, refusing conflicting files. Dry-run by default."""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile
import uuid

DEFAULT_DOCS = '/Users/asaied/WorkDir/TradeStocker/TradeStockerServices/TradeStockerBackend/docs'
START = '<!-- TRADESTOCKER_PLAN_START -->'
END = '<!-- TRADESTOCKER_PLAN_END -->'

class InstallError(Exception):
    pass

def digest(content: bytes) -> str:
    return hashlib.sha256(content).hexdigest()

def reject_symlinks(path: Path):
    current = Path(path.anchor)
    for part in path.parts[1:]:
        current = current / part
        if current.is_symlink():
            raise InstallError(f'Refusing symlink in destination path: {current}')

def read_manifest(source: Path):
    try:
        data = json.loads((source/'manifest.json').read_text(encoding='utf-8'))
    except (OSError,ValueError) as exc:
        raise InstallError(f'Cannot read package manifest: {exc}') from exc
    for row in data['files']:
        rel=Path(row['path'])
        if rel.is_absolute() or '..' in rel.parts:
            raise InstallError('Unsafe package manifest path')
        p=source/rel
        if p.is_symlink() or not p.is_file():
            raise InstallError(f'Invalid source file: {rel}')
        raw=p.read_bytes()
        if digest(raw)!=row['sha256'] or len(raw)!=row['size']:
            raise InstallError(f'Package integrity failure: {rel}')
    required={'AGENTS.root.fragment.md','docs/SOURCE_OF_TRUTH.md','docs/state/project_state.json'}
    if not required.issubset({row['path'] for row in data['files']}):
        raise InstallError('Package manifest missing required files')
    return data

def plan(source: Path, docs: Path):
    manifest=read_manifest(source)
    if docs.name != 'docs':
        raise InstallError('Destination must be a docs directory so root AGENTS paths remain correct')
    repo=docs.parent
    reject_symlinks(docs)
    if not repo.is_dir():
        raise InstallError(f'Repository parent does not exist: {repo}. Refusing to create an assumed repository.')
    if docs.exists() and not docs.is_dir():
        raise InstallError(f'Destination is not a directory: {docs}')
    entries=[]; conflicts=[]
    for row in manifest['files']:
        if not row['path'].startswith('docs/'): continue
        rel=Path(row['path']).relative_to('docs'); dst=docs/rel
        reject_symlinks(dst)
        src=source/'docs'/rel
        if dst.exists():
            if not dst.is_file() or dst.read_bytes()!=src.read_bytes(): conflicts.append(str(dst))
        else:
            entries.append((src,dst))
    agent=repo/'AGENTS.md'; reject_symlinks(agent)
    if agent.exists() and not agent.is_file(): conflicts.append(str(agent))
    original=agent.read_bytes() if agent.is_file() else None
    fragment=(source/'AGENTS.root.fragment.md').read_bytes()
    action='create'
    new=fragment
    if original is not None:
        text=original.decode('utf-8')
        frag=fragment.decode('utf-8')
        if START in text or END in text:
            if text.count(START)!=1 or text.count(END)!=1 or frag.strip() not in text:
                conflicts.append(f'{agent} (different or malformed managed block)')
            action='skip'; new=original
        else:
            action='append-preserving-existing'
            new=original.rstrip()+b'\n\n'+fragment
    if conflicts:
        raise InstallError('Conflicts found; nothing will be overwritten:\n'+'\n'.join(conflicts)+'\nReconcile manually with an agent. Do not overwrite project_state.json after work has begun.')
    return entries,agent,original,new,action

def install(source: Path, docs: Path, apply: bool):
    entries,agent,original,new,action=plan(source,docs)
    print(f'Destination: {docs}')
    print(f'New docs files: {len(entries)}; existing identical files: left unchanged')
    print(f'Repository AGENTS.md: {action}')
    if not apply:
        print('DRY RUN. No files were written. Use --apply after reviewing the destination.')
        return
    # Exclusive writes prevent overwriting a file created after the preflight.
    for src,dst in entries:
        reject_symlinks(dst)
        dst.parent.mkdir(parents=True,exist_ok=True)
        with dst.open('xb') as f:
            f.write(src.read_bytes())
    if action=='create':
        with agent.open('xb') as f: f.write(new)
    elif action=='append-preserving-existing':
        reject_symlinks(agent)
        if agent.read_bytes()!=original:
            raise InstallError('AGENTS.md changed after preflight; leave it unchanged and rerun/reconcile')
        suffix=dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+uuid.uuid4().hex[:6]
        backup=agent.with_name('AGENTS.md.tradestocker-backup-'+suffix)
        with backup.open('xb') as f: f.write(original)
        temporary=None
        try:
            with tempfile.NamedTemporaryFile('wb',dir=agent.parent,prefix='.agents-',delete=False) as f:
                temporary=Path(f.name); f.write(new); f.flush(); os.fsync(f.fileno())
            os.chmod(temporary,agent.stat().st_mode & 0o777)
            if agent.read_bytes()!=original:
                raise InstallError('AGENTS.md changed during append; original is preserved')
            os.replace(temporary,agent)
        finally:
            if temporary and temporary.exists(): temporary.unlink()
        print(f'Preserved original AGENTS backup: {backup}')
    print('Installed planning docs only. No app code, Git commit/push, cloud resource, secret or live lane changed.')
    print(f'Next: cd "{docs.parent}" && python3 docs/tools/planctl.py validate')
    print('Then read AGENTS.md and claim S00-T01 only if its actual current state is still TODO.')

def main(argv=None):
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--docs-dir',default=DEFAULT_DOCS,type=Path)
    parser.add_argument('--apply',action='store_true')
    args=parser.parse_args(argv)
    source=Path(__file__).resolve().parent
    docs=Path(os.path.abspath(os.path.expanduser(str(args.docs_dir))))
    try:
        install(source,docs,args.apply)
        return 0
    except (InstallError,OSError,UnicodeError,ValueError,KeyError) as exc:
        print(f'ERROR: {exc}',file=sys.stderr)
        print('On an operating-system failure a partial set of new docs may exist. Rerun safely after resolving it; existing identical files are not overwritten.',file=sys.stderr)
        return 2

if __name__=='__main__':
    raise SystemExit(main())
