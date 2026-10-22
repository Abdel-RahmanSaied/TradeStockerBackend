"""Installation safety tests using disposable filesystem targets only."""
import contextlib
import importlib.util
import io
import json
import hashlib
from pathlib import Path
import shutil
import tempfile
import unittest

SOURCE=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('install_docs',SOURCE/'install_docs.py')
installer=importlib.util.module_from_spec(spec); spec.loader.exec_module(installer)
fixture_spec=importlib.util.spec_from_file_location('planning_fixtures',SOURCE/'docs/tools/planning_fixtures.py')
fixtures=importlib.util.module_from_spec(fixture_spec); fixture_spec.loader.exec_module(fixtures)

class InstallTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory(); self.root=Path(self.tmp.name).resolve()
        # Build a disposable package fixture. Never regenerate the real manifest.
        self.source=self.root/'package'; self.source.mkdir()
        manifest=json.loads((SOURCE/'manifest.json').read_text())
        for row in manifest['files']:
            target=self.source/row['path']; target.parent.mkdir(parents=True,exist_ok=True)
            shutil.copyfile(SOURCE/row['path'],target)
        state_path=self.source/'docs/state/project_state.json'
        catalog=json.loads((self.source/'docs/state/task_catalog.json').read_text())
        state_path.write_text(json.dumps(fixtures.bootstrap_state(catalog,json.loads(state_path.read_text())),indent=2))
        for row in manifest['files']:
            raw=(self.source/row['path']).read_bytes()
            row.update(size=len(raw),sha256=hashlib.sha256(raw).hexdigest())
        (self.source/'manifest.json').write_text(json.dumps(manifest))
        self.repo=self.root/'repo'; self.repo.mkdir(); self.docs=self.repo/'docs'
    def tearDown(self): self.tmp.cleanup()
    def run_install(self,apply=True,source=None):
        with contextlib.redirect_stdout(io.StringIO()): installer.install(source or self.source,self.docs,apply)
    def test_dry_run_writes_nothing(self):
        self.run_install(False)
        self.assertEqual(list(self.repo.iterdir()),[])
    def test_fresh_install(self):
        self.run_install()
        self.assertTrue((self.docs/'SOURCE_OF_TRUTH.md').exists())
        self.assertIn(installer.START,(self.repo/'AGENTS.md').read_text())
    def test_existing_agents_preserved_with_backup(self):
        old='Existing repository instructions must remain.\n'
        (self.repo/'AGENTS.md').write_text(old)
        self.run_install()
        self.assertTrue((self.repo/'AGENTS.md').read_text().startswith(old))
        backups=list(self.repo.glob('AGENTS.md.tradestocker-backup-*'))
        self.assertEqual(len(backups),1); self.assertEqual(backups[0].read_text(),old)
    def test_conflict_preflight_writes_nothing_else(self):
        self.docs.mkdir(); (self.docs/'README.md').write_text('Existing different docs')
        with self.assertRaises(installer.InstallError): self.run_install()
        self.assertEqual([p.name for p in self.docs.iterdir()],['README.md'])
        self.assertFalse((self.repo/'AGENTS.md').exists())
    def test_idempotent_reinstall(self):
        self.run_install(); first=(self.repo/'AGENTS.md').read_bytes()
        self.run_install(); self.assertEqual((self.repo/'AGENTS.md').read_bytes(),first)
    def test_changed_execution_state_not_overwritten(self):
        self.run_install(); p=self.docs/'state/project_state.json'
        s=json.loads(p.read_text()); s['revision']=9; p.write_text(json.dumps(s))
        with self.assertRaises(installer.InstallError): self.run_install()
        self.assertEqual(json.loads(p.read_text())['revision'],9)
    def test_symlink_destination_refused(self):
        target=self.root/'external'; target.mkdir(); self.docs.symlink_to(target,target_is_directory=True)
        with self.assertRaises(installer.InstallError): self.run_install()
        self.assertEqual(list(target.iterdir()),[])
    def test_tampered_package_refused(self):
        source=self.root/'tampered'; shutil.copytree(self.source,source)
        (source/'docs/SOURCE_OF_TRUTH.md').write_text('Tampered')
        with self.assertRaises(installer.InstallError): self.run_install(source=source)
        self.assertEqual(list(self.repo.iterdir()),[])
    def test_malformed_managed_block_refused(self):
        (self.repo/'AGENTS.md').write_text(installer.START+'\nconflicting instructions')
        with self.assertRaises(installer.InstallError): self.run_install()
        self.assertFalse(self.docs.exists())
    def test_missing_repository_parent_refused(self):
        self.docs=self.root/'not-created'/'docs'
        with self.assertRaises(installer.InstallError): self.run_install()
        self.assertFalse(self.docs.parent.exists())

if __name__=='__main__': unittest.main(verbosity=2)
