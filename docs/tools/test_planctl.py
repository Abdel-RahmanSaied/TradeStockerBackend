"""Tests of the planning utility, not of the TradeStocker application."""
import contextlib
import copy
import importlib.util
import io
import json
from pathlib import Path
import shutil
import tempfile
import unittest
from planning_fixtures import bootstrap_state

spec=importlib.util.spec_from_file_location('planctl',Path(__file__).with_name('planctl.py'))
pc=importlib.util.module_from_spec(spec); spec.loader.exec_module(pc)
SOURCE=Path(__file__).resolve().parents[1]

class PlanTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory()
        self.docs=Path(self.tmp.name)/'docs'
        shutil.copytree(SOURCE,self.docs,ignore=shutil.ignore_patterns('__pycache__'))
        self.catalog,self.state=pc.load(self.docs)
        self.state=bootstrap_state(self.catalog,self.state)
        self.save()
    def tearDown(self): self.tmp.cleanup()
    def call(self,*args):
        with contextlib.redirect_stdout(io.StringIO()),contextlib.redirect_stderr(io.StringIO()):
            return pc.main(['--docs-dir',str(self.docs),*args])
    def save(self): pc.atomic_write(self.docs/'state/project_state.json',self.state)
    def claim(self): return self.call('claim','S00-T01','--owner','agent-a','--session','test-session','--expected-revision','0')
    def evidence(self):
        (self.docs/'evidence/test.md').write_text('Synthetic planning-tool test evidence; not product verification.\n')
    def test_bootstrap_valid(self):
        self.assertEqual(pc.validate(self.catalog,self.state,self.docs),[])
        self.assertGreaterEqual(len(self.catalog['tasks']),102)
        self.assertIn('S15-T06', self.state['tasks'])
        self.assertTrue(all(x['status']=='TODO' for x in self.state['tasks'].values()))
    def test_first_ready_task(self):
        self.assertEqual([t['id'] for t in pc.ready_tasks(self.catalog,self.state)],['S00-T01'])
    def test_cycle_rejected(self):
        self.catalog['tasks'][0]['depends_on']=['S00-T02']
        self.assertTrue(any('cycle' in e.lower() for e in pc.validate(self.catalog,self.state,self.docs)))
    def test_unknown_dependency_rejected(self):
        self.catalog['tasks'][0]['depends_on']=['S99-T99']
        self.assertTrue(any('invalid dependency' in e for e in pc.validate(self.catalog,self.state,self.docs)))
    def test_state_missing_task_rejected(self):
        del self.state['tasks']['S00-T01']
        self.assertTrue(pc.validate(self.catalog,self.state,self.docs))
    def test_claim_updates_revision_without_lane_enablement(self):
        self.assertEqual(self.claim(),0)
        _,s=pc.load(self.docs)
        self.assertEqual(s['revision'],1); self.assertEqual(s['tasks']['S00-T01']['status'],'IN_PROGRESS')
        self.assertFalse(s['live_advisory_enabled'])
    def test_stale_revision_refused(self):
        self.assertEqual(self.claim(),0)
        self.assertEqual(self.call('transition','S00-T01','--to','BLOCKED','--reason','fixture','--expected-revision','0'),2)
        self.assertEqual(pc.load(self.docs)[1]['revision'],1)
    def test_unmet_dependency_refused(self):
        self.assertEqual(self.call('claim','S00-T02','--owner','agent-a','--session','test','--expected-revision','0'),2)
    def test_existing_claim_not_stolen(self):
        self.assertEqual(self.claim(),0)
        self.assertEqual(self.call('claim','S00-T01','--owner','agent-b','--session','test-b','--expected-revision','1'),2)
    def test_block_requires_reason(self):
        self.assertEqual(self.claim(),0)
        self.assertEqual(self.call('transition','S00-T01','--to','BLOCKED','--expected-revision','1'),2)
    def test_block_and_resume(self):
        self.assertEqual(self.claim(),0)
        self.assertEqual(self.call('transition','S00-T01','--to','BLOCKED','--reason','fixture input','--expected-revision','1'),0)
        self.assertEqual(self.call('claim','S00-T01','--owner','agent-a','--session','test-session-2','--expected-revision','2'),0)
    def test_review_needs_evidence(self):
        self.assertEqual(self.claim(),0)
        self.assertEqual(self.call('transition','S00-T01','--to','IN_REVIEW','--expected-revision','1'),2)
    def test_valid_review_and_done_on_synthetic_copy(self):
        self.assertEqual(self.claim(),0); self.evidence()
        _,s=pc.load(self.docs)
        s['repo_observation']={'status':'VERIFIED','root':'/fixture/repo','head':'abcdef0123456789','branch':'fixture','working_tree':'fixture'}
        pc.atomic_write(self.docs/'state/project_state.json',s)
        self.assertEqual(self.call('transition','S00-T01','--to','IN_REVIEW','--evidence','evidence/test.md','--expected-revision','1'),0)
        self.assertEqual(self.call('transition','S00-T01','--to','DONE','--verified-commit','abcdef0123456789','--reviewed-by','reviewer-b','--summary','Synthetic fixture reviewed','--expected-revision','2'),0)
        self.assertEqual(pc.load(self.docs)[1]['tasks']['S00-T01']['status'],'DONE')
    def test_same_owner_review_rejected(self):
        self.claim(); self.evidence()
        self.call('transition','S00-T01','--to','IN_REVIEW','--evidence','evidence/test.md','--expected-revision','1')
        self.assertEqual(self.call('transition','S00-T01','--to','DONE','--verified-commit','abcdef0','--reviewed-by','agent-a','--summary','fixture','--expected-revision','2'),2)
    def test_evidence_path_escape_rejected(self):
        with self.assertRaises(pc.PlanError): pc.local_reference(self.docs,'../outside.md')
    def test_handoff_reference_checked(self):
        self.assertEqual(self.call('handoff','--session','test','--file','sessions/missing.md','--expected-revision','0'),2)
    def test_generated_documents_match_catalog(self):
        for rel,text in pc.render_documents(self.catalog).items():
            self.assertEqual((self.docs/rel).read_text(),text,rel)
    def test_broken_link_detected(self):
        (self.docs/'bad.md').write_text('[broken](missing-target.md)')
        self.assertTrue(pc.validate_links(self.docs))
    def test_lock_prevents_mutation(self):
        (self.docs/'state/.planctl.lock').write_text('fixture lock')
        self.assertEqual(self.claim(),2)
        self.assertEqual(pc.load(self.docs)[1]['revision'],0)
    def test_live_flag_without_approval_rejected(self):
        self.state['live_advisory_enabled']=True
        self.assertTrue(pc.validate(self.catalog,self.state,self.docs))

if __name__=='__main__': unittest.main(verbosity=2)
