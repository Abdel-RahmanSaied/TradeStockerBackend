#!/usr/bin/env python3
"""Local planning workflow only. Never deploys, trades, or enables advisory lanes."""
from __future__ import annotations

import argparse
import collections
import contextlib
import datetime as dt
import json
import os
from pathlib import Path
import re
import sys
import tempfile
from typing import Any

DEFAULT_DOCS = Path(__file__).resolve().parents[1]
STATUSES = {'TODO', 'IN_PROGRESS', 'BLOCKED', 'IN_REVIEW', 'DONE'}
LANES = {'EGX_SWING', 'US_SWING', 'EGX_INTRADAY', 'US_INTRADAY'}
LANE_STATES = {'DISABLED', 'DATA_VALIDATED', 'REPLAY_VALIDATED', 'SHADOW', 'PAPER_VALIDATED', 'LIMITED_ADVISORY', 'ACTIVE_ADVISORY', 'PAUSED'}
TRANSITIONS = {
    'TODO': {'BLOCKED'},
    'IN_PROGRESS': {'IN_REVIEW', 'BLOCKED'},
    'BLOCKED': set(),  # Resume with claim so ownership and dependencies are checked.
    'IN_REVIEW': {'DONE', 'IN_PROGRESS', 'BLOCKED'},
    'DONE': set(),
}

class PlanError(Exception):
    pass

def now() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat(timespec='seconds').replace('+00:00', 'Z')

def load_json(path: Path) -> dict[str, Any]:
    try:
        data = json.loads(path.read_text(encoding='utf-8'))
    except (OSError, ValueError) as exc:
        raise PlanError(f'Cannot read {path}: {exc}') from exc
    if not isinstance(data, dict):
        raise PlanError(f'{path} must contain a JSON object')
    return data

def load(docs: Path):
    return load_json(docs/'state/task_catalog.json'), load_json(docs/'state/project_state.json')

def local_reference(docs: Path, reference: str) -> Path:
    if not isinstance(reference, str) or not reference:
        raise PlanError('Evidence/handoff reference must be a nonempty docs-relative path')
    rel = Path(reference)
    if rel.is_absolute() or '..' in rel.parts:
        raise PlanError(f'Unsafe docs-relative reference: {reference}')
    resolved = (docs/rel).resolve()
    try:
        resolved.relative_to(docs.resolve())
    except ValueError as exc:
        raise PlanError(f'Reference escapes docs directory: {reference}') from exc
    if not resolved.is_file():
        raise PlanError(f'Missing evidence/handoff file: {reference}')
    return resolved

def validate(catalog: dict, state: dict, docs: Path) -> list[str]:
    errors: list[str] = []
    if catalog.get('schema_version') != '1.0' or state.get('schema_version') != '1.0':
        errors.append('Unsupported schema_version')
    if state.get('plan_version') != catalog.get('plan_version'):
        errors.append('Plan version mismatch between catalog and state')
    if not isinstance(state.get('revision'), int) or state.get('revision', -1) < 0:
        errors.append('State revision must be a nonnegative integer')
    specs = catalog.get('tasks', [])
    sprints = catalog.get('sprints', [])
    ids = [x.get('id') for x in specs]
    sids = [x.get('id') for x in sprints]
    if len(ids) != len(set(ids)):
        errors.append('Duplicate task IDs')
    if len(sids) != len(set(sids)):
        errors.append('Duplicate sprint IDs')
    by = {x.get('id'): x for x in specs}
    sby = {x.get('id'): x for x in sprints}
    records = state.get('tasks', {})
    if set(ids) != set(records):
        errors.append(f'Task/state mismatch: missing={sorted(set(ids)-set(records))}, extra={sorted(set(records)-set(ids))}')
    if state.get('active_sprint') not in sby:
        errors.append('Invalid active_sprint')
    listed = [tid for s in sprints for tid in s.get('task_ids', [])]
    if collections.Counter(listed) != collections.Counter(ids):
        errors.append('Sprint task lists must contain every task exactly once')
    for t in specs:
        tid = t.get('id', '')
        if not re.fullmatch(r'S\d{2}-T\d{2}', tid):
            errors.append(f'Invalid task ID: {tid}')
        if t.get('sprint_id') not in sby or tid not in sby.get(t.get('sprint_id'), {}).get('task_ids', []):
            errors.append(f'{tid}: inconsistent sprint membership')
        for field in ['title','owner_role','scope']:
            if not t.get(field): errors.append(f'{tid}: missing {field}')
        for field in ['deliverables','acceptance_criteria','validation','requirement_ids']:
            if not isinstance(t.get(field), list) or not t[field]: errors.append(f'{tid}: empty {field}')
        for dep in t.get('depends_on', []):
            if dep not in by or dep == tid: errors.append(f'{tid}: invalid dependency {dep}')
        for req in t.get('requirement_ids', []):
            if req not in {f'REQ-{i:02d}' for i in range(1,16)}: errors.append(f'{tid}: unknown requirement {req}')
        for decision in t.get('decision_dependencies', []):
            if decision not in state.get('decision_status', {}): errors.append(f'{tid}: unknown decision {decision}')
        rec = records.get(tid, {})
        status = rec.get('status')
        if status not in STATUSES: errors.append(f'{tid}: invalid task status {status}')
        if status in {'IN_PROGRESS','IN_REVIEW','DONE'}:
            for field in ['owner','session_id','started_at']:
                if not rec.get(field): errors.append(f'{tid}: {status} requires {field}')
            missing = [d for d in t.get('depends_on',[]) if records.get(d,{}).get('status') != 'DONE']
            if missing: errors.append(f'{tid}: dependencies not DONE: {missing}')
        if status == 'BLOCKED' and not rec.get('blocked_reason'):
            errors.append(f'{tid}: BLOCKED requires a reason')
        if status in {'IN_REVIEW','DONE'} and not rec.get('evidence'):
            errors.append(f'{tid}: {status} requires evidence')
        for ref in rec.get('evidence', []):
            try: local_reference(docs, ref)
            except PlanError as exc: errors.append(f'{tid}: {exc}')
        if status == 'DONE':
            if not re.fullmatch(r'[0-9a-fA-F]{7,64}', rec.get('verified_commit') or ''):
                errors.append(f'{tid}: DONE requires a real recorded Git commit hash')
            if not rec.get('reviewed_by') or rec['reviewed_by'] == rec.get('owner'):
                errors.append(f'{tid}: DONE requires a named reviewer distinct from owner')
            if not rec.get('completion_summary'):
                errors.append(f'{tid}: DONE requires a completion summary')
    # Detect cycles with DFS.
    seen: set[str] = set(); active: set[str] = set()
    def visit(tid: str):
        if tid in active:
            errors.append(f'Dependency cycle involving {tid}'); return
        if tid in seen or tid not in by: return
        active.add(tid)
        for dep in by[tid].get('depends_on', []): visit(dep)
        active.remove(tid); seen.add(tid)
    for tid in ids: visit(tid)
    if set(state.get('lanes', {})) != LANES:
        errors.append('Exactly four lane records are required')
    live = False
    for lane, rec in state.get('lanes', {}).items():
        if rec.get('state') not in LANE_STATES: errors.append(f'{lane}: unknown lane state')
        if rec.get('state') in {'LIMITED_ADVISORY','ACTIVE_ADVISORY'}:
            live = True
            try: local_reference(docs, rec.get('approval_evidence'))
            except PlanError as exc: errors.append(f'{lane}: {exc}')
    if bool(state.get('live_advisory_enabled')) != live:
        errors.append('live_advisory_enabled must match recorded active/limited lane approvals')
    for decision, rec in state.get('decision_status', {}).items():
        if rec.get('status') not in {'OPEN','RESOLVED','DEFERRED'}:
            errors.append(f'{decision}: invalid decision status')
        if rec.get('status') == 'RESOLVED':
            try: local_reference(docs, rec.get('evidence'))
            except PlanError as exc: errors.append(f'{decision}: {exc}')
    try: local_reference(docs, state.get('last_handoff'))
    except PlanError as exc: errors.append(str(exc))
    if records.get('S00-T01',{}).get('status') == 'DONE':
        obs=state.get('repo_observation',{})
        if obs.get('status') != 'VERIFIED' or not obs.get('root') or not obs.get('head'):
            errors.append('S00-T01 cannot be DONE before an actual repository observation is recorded')
    return errors

def require_valid(catalog: dict, state: dict, docs: Path):
    errors=validate(catalog,state,docs)
    if errors: raise PlanError('\n'.join(errors))

def render_documents(catalog: dict) -> dict[str,str]:
    by={t['id']:t for t in catalog['tasks']}
    core=[s for s in catalog['sprints'] if not s['optional']]
    opt=[s for s in catalog['sprints'] if s['optional']]
    overview=[
        '# TradeStocker — Development Plan', '',
        '> ملف مولّد من `state/task_catalog.json`. لا تعدّل تفاصيله يدويًا؛ استخدم `planctl.py render`.', '',
        f"**Baseline:** `{catalog['plan_version']}` · **{len(core)} سبرينت أساسي + {len(opt)} اختياري** · **{len(catalog['tasks'])} تاسك**.", '',
        '## طريقة استخدام الخطة', '',
        'السبرينتات هنا حزم عمل مرتبة وليست وعدًا بمدة تقويمية. حجم الفريق ووقت التطوير وشراء البيانات لم تُحسم. في S00 قسّم المهام الكبيرة إلى وحدات قابلة للإنجاز ضمن cadence متفق عليه، دون إسقاط بوابات الجودة.', '',
        'اتبع الاعتماديات على مستوى التاسك، لا رقم السبرينت فقط. بعض العمل يمكن تنفيذه بالتوازي بعد جاهزية مدخلاته. لا تبدأ مهام اختيارية بلا اعتماد. حجم S/M/L نسبي وليس تقدير أيام.', '',
        'الحالة الفعلية في [project_state.json](state/project_state.json). تعريفات التاسكات في [task_catalog.json](state/task_catalog.json). ابدأ من [SOURCE_OF_TRUTH.md](SOURCE_OF_TRUTH.md) وبروتوكول [AGENT_PLAYBOOK.md](AGENT_PLAYBOOK.md).', '',
        '## خريطة السبرينتات', '',
        '| Sprint | النطاق | Tasks | بوابة الخروج |', '|---|---|---:|---|',
    ]
    for s in catalog['sprints']:
        overview.append(f"| [{s['id']}](sprints/{s['id']}.md) | {s.get('title_ar',s['title'])}{' — اختياري' if s['optional'] else ''} | {len(s['task_ids'])} | {s['exit_gate']} |")
    overview += ['', '## تسلسل القرارات المهمة', '',
        '**S00–S02:** افحص الموجود، ثبّت العقود والهوية وCI والتخزين والأحداث. بدأ الحصول على البيانات في S00؛ لا تنتظر آخر المشروع.', '',
        '**S03–S06:** بيانات صحيحة، ثم محفظة وقيود مالية، ثم محاكاة وتقييم مسجل مسبقًا. لا تبنِ لجنة تصدر توصيات قبل حماية المال والوقت والهوية.', '',
        '**S07–S11:** أدلة مالية وتاريخية، لجنة مشروطة، Swing، Telegram، ثم Intraday القابل للتنفيذ البشري. نفّذ البيانات والاختبارات لكل سوق بشكل مستقل.', '',
        '**S12–S15:** أهداف لا تتجاوز المخاطر، لوحة تشغيل، اختبارات نشر وتعافٍ، ثم تفعيل مستقل بأدلة. الأمان والاختبارات موجودة منذ S01؛ S14 تحقق إنتاجي وليس أول مرة نفكر فيهما.', '',
        '**S16:** OpenClaw read-only بعد core وباعتماد منفصل؛ لا يدخل في critical path.', '',
        '## بوابات لا يتجاوزها التقويم', '',
        'اكتمال الكود لا يعني جدوى استراتيجية، ونجاح Paper لا يضمن الربح. غياب بيانات أو حقوق استخدام أو قواعد حساب صحيحة يمنع lane المتأثرة. لا يشترط أن تحقق الأربع lanes GO في نفس الوقت.', '',
        'مراجعة يمكن أن تنتهي NO_GO وتُعد مراجعة مكتملة، لكن adapter غير منفذ أو اختبار لم يُشغل لا يتحول إلى DONE بسبب كتابة تقرير. ملفات التجميع تسجل جاهزية المتاح وعوائق الباقي؛ لا تستعمل نتيجة سوق لتغطية سوق آخر.', '',
        '## ما تحتويه كل تاسك', '',
        'ID ثابت، owner role، نطاق، dependencies، قرارات يجب مراجعتها، مخرجات مقترحة، acceptance criteria واختبارات، وربط بالمتطلبات. المسارات مبدئية حتى S00 يطابقها مع المستودع الفعلي.', '',
        '## بداية التنفيذ', '',
        'أول تاسك `S00-T01`. شغّل `python3 docs/tools/planctl.py validate` ثم `status` و`ready`. لا تجعل إنشاء هذه الوثائق دليلًا على تنفيذ المنتج.',
    ]
    result={'DEVELOPMENT_PLAN.md':'\n'.join(overview)+'\n'}
    for s in catalog['sprints']:
        lines=[f"# {s['id']} — {s.get('title_ar',s['title'])}", '', f"**Technical scope:** {s['title']}", '',
            '> ملف مولّد؛ مصدر التعريف `../state/task_catalog.json`. الحالة الفعلية في `../state/project_state.json`، وليست في checkboxes هنا.', '',
            f"**اختياري:** {'نعم؛ يتطلب اعتمادًا منفصلًا.' if s['optional'] else 'لا؛ ضمن الخطة الأساسية.'}", '',
            '## الهدف', '', s['goal'], '', '## بوابة الخروج', '', s['exit_gate'], '',
            '## ترتيب المهام داخل السبرينت', '',
            '| Task | Title | Owner role | Dependencies | Size |', '|---|---|---|---|---|']
        for tid in s['task_ids']:
            t=by[tid]
            lines.append(f"| `{tid}` | {t['title']} | {t['owner_role']} | {', '.join(t['depends_on']) or 'None'} | {t['size']} |")
        for tid in s['task_ids']:
            t=by[tid]
            lines += ['', f"## {tid} — {t['title']}", '',
                f"**Owner role:** {t['owner_role']} · **Priority:** {t['priority']} · **Relative size:** {t['size']}", '',
                '**Dependencies:** '+(', '.join(f'`{x}`' for x in t['depends_on']) or 'None'), '',
                '**Decisions to review:** '+(', '.join(f'`{x}`' for x in t['decision_dependencies']) or 'No additional decision ID.'), '',
                '**Requirements:** '+', '.join(f'`{x}`' for x in t['requirement_ids']), '',
                '### النطاق', '',t['scope'], '', '### المخرجات المقترحة', '']
            lines += [f'- `{p}`' for p in t['deliverables']]
            lines += ['', '### معايير القبول', '']+[f'- {x}' for x in t['acceptance_criteria']]
            lines += ['', '### التحقق والاختبارات', '']+[f'- {x}' for x in t['validation']]
            lines += ['', '**Evidence:** أضف دليلًا فعليًا داخل `docs/evidence/` واربطه بالحالة. لا تعني هذه القائمة أن الملفات أو الاختبارات موجودة الآن.', '']
        lines += ['## تسليم السبرينت', '',
            'راجع Definition of Done وQUALITY_GATES. حدّث task state، ثم handoff يحدد HEAD والاختبارات والعوائق والخطوة التالية. لا تفعّل lane تلقائيًا عند اكتمال السبرينت.', '',
            '[خطة التنفيذ](../DEVELOPMENT_PLAN.md) · [بروتوكول الجلسات](../AGENT_PLAYBOOK.md) · [مصدر الحقيقة](../SOURCE_OF_TRUTH.md)', '']
        result[f"sprints/{s['id']}.md"]='\n'.join(lines)
    return result

def validate_links(docs: Path) -> list[str]:
    errors=[]
    for p in docs.rglob('*.md'):
        for match in re.finditer(r'\[[^\]\n]+\]\(([^)\s]+)\)',p.read_text(encoding='utf-8')):
            target=match.group(1).split('#',1)[0]
            if not target or '://' in target or target.startswith(('mailto:','data:')): continue
            if '<' in target or '>' in target: continue
            if not (p.parent/target).exists():
                errors.append(f'{p.relative_to(docs)}: broken local link {target}')
    return errors

@contextlib.contextmanager
def state_lock(docs: Path):
    path=docs/'state/.planctl.lock'
    try:
        fd=os.open(str(path),os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
    except FileExistsError as exc:
        raise PlanError(f'Lock exists: {path}. Inspect its owner/process; do not automatically delete it.') from exc
    try:
        with os.fdopen(fd,'w') as f:
            f.write(json.dumps({'pid':os.getpid(),'created_at':now()})); f.flush(); os.fsync(f.fileno())
        yield
    finally:
        path.unlink(missing_ok=True)

def atomic_write(path: Path, data: dict):
    temp=None
    try:
        with tempfile.NamedTemporaryFile('w',encoding='utf-8',dir=path.parent,prefix='.state-',delete=False) as f:
            temp=Path(f.name); json.dump(data,f,indent=2,ensure_ascii=False); f.write('\n'); f.flush(); os.fsync(f.fileno())
        os.replace(temp,path)
    finally:
        if temp and temp.exists(): temp.unlink()

def ready_tasks(catalog: dict,state: dict,include_optional=False):
    for t in catalog['tasks']:
        if t.get('optional') and not include_optional: continue
        if state['tasks'][t['id']]['status'] != 'TODO': continue
        if all(state['tasks'][x]['status']=='DONE' for x in t['depends_on']): yield t

def mutate(args, docs: Path):
    with state_lock(docs):
        catalog,state=load(docs); require_valid(catalog,state,docs)
        if state['revision'] != args.expected_revision:
            raise PlanError(f'Revision conflict: expected {args.expected_revision}, actual {state["revision"]}. Reload and reconcile.')
        stamp=now(); event={'at':stamp,'action':args.command}
        by={t['id']:t for t in catalog['tasks']}
        if args.command=='handoff':
            local_reference(docs,args.file)
            state['last_session']=args.session; state['last_handoff']=args.file
            event.update(session_id=args.session,file=args.file)
        else:
            if args.task_id not in by: raise PlanError(f'Unknown task: {args.task_id}')
            t=by[args.task_id]; rec=state['tasks'][args.task_id]
            event.update(task_id=args.task_id,from_status=rec['status'])
            if args.command=='claim':
                if rec['status'] not in {'TODO','BLOCKED'}: raise PlanError('Task already claimed/reviewed/done; inspect its handoff instead of stealing it')
                if rec.get('owner') and rec['owner'] != args.owner: raise PlanError('Existing owner differs. Use a reviewed reassignment, not an automatic takeover.')
                missing=[d for d in t['depends_on'] if state['tasks'][d]['status']!='DONE']
                if missing: raise PlanError(f'Dependencies are not DONE: {missing}')
                if t.get('optional') and not args.authorize_optional: raise PlanError('Optional scope requires --authorize-optional and actual owner approval evidence')
                if t.get('optional'):
                    local_reference(docs,args.approval_evidence)
                    event['optional_approval_evidence']=args.approval_evidence
                rec.update(status='IN_PROGRESS',owner=args.owner,session_id=args.session,started_at=rec.get('started_at') or stamp,updated_at=stamp,blocked_reason=None)
                state['active_sprint']=t['sprint_id']
                event.update(owner=args.owner,session_id=args.session,to_status='IN_PROGRESS')
            else:
                dest=args.to
                if dest not in TRANSITIONS.get(rec['status'],set()): raise PlanError(f'Invalid transition {rec["status"]} → {dest}')
                if dest=='BLOCKED' and not args.reason: raise PlanError('BLOCKED requires --reason')
                refs=list(dict.fromkeys(rec.get('evidence',[])+(args.evidence or [])))
                for ref in refs: local_reference(docs,ref)
                rec.update(status=dest,updated_at=stamp,evidence=refs,blocked_reason=args.reason if dest=='BLOCKED' else None)
                if args.verified_commit: rec['verified_commit']=args.verified_commit
                if args.reviewed_by: rec['reviewed_by']=args.reviewed_by
                if args.summary: rec['completion_summary']=args.summary
                event.update(to_status=dest,reason=args.reason,reviewed_by=args.reviewed_by)
        state['revision']+=1; state['updated_at']=stamp
        state.pop('updated_at_note',None)
        state.setdefault('history',[]).append(event)
        require_valid(catalog,state,docs)
        atomic_write(docs/'state/project_state.json',state)
        print(f'Updated planning state only. revision={state["revision"]}; command={args.command}')
        print('No application, deployment, trading, or lane-activation action was performed.')

def main(argv=None) -> int:
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--docs-dir',type=Path,default=DEFAULT_DOCS)
    sub=parser.add_subparsers(dest='command',required=True)
    sub.add_parser('validate')
    sub.add_parser('status')
    p=sub.add_parser('ready'); p.add_argument('--include-optional',action='store_true')
    p=sub.add_parser('render'); p.add_argument('--check',action='store_true')
    p=sub.add_parser('claim'); p.add_argument('task_id'); p.add_argument('--owner',required=True); p.add_argument('--session',required=True); p.add_argument('--expected-revision',type=int,required=True); p.add_argument('--authorize-optional',action='store_true'); p.add_argument('--approval-evidence')
    p=sub.add_parser('transition'); p.add_argument('task_id'); p.add_argument('--to',choices=sorted(STATUSES),required=True); p.add_argument('--expected-revision',type=int,required=True); p.add_argument('--reason'); p.add_argument('--evidence',action='append'); p.add_argument('--verified-commit'); p.add_argument('--reviewed-by'); p.add_argument('--summary')
    p=sub.add_parser('handoff'); p.add_argument('--session',required=True); p.add_argument('--file',required=True); p.add_argument('--expected-revision',type=int,required=True)
    args=parser.parse_args(argv); docs=args.docs_dir.resolve()
    try:
        if args.command in {'claim','transition','handoff'}: mutate(args,docs); return 0
        catalog,state=load(docs); require_valid(catalog,state,docs)
        if args.command=='validate':
            errors=validate_links(docs)
            if errors: raise PlanError('\n'.join(errors))
            print(f'PASS: {len(catalog["sprints"])} sprints, {len(catalog["tasks"])} tasks; valid IDs, DAG, state, references and local Markdown links.')
            print('This validates planning structure, not product implementation or truth of evidence.')
        elif args.command=='status':
            print(f'TradeStocker plan {catalog["plan_version"]}; revision {state["revision"]}; active sprint {state["active_sprint"]}')
            print('Repository observation:',state['repo_observation']['status'])
            print('Tasks:',dict(sorted(collections.Counter(x['status'] for x in state['tasks'].values()).items())))
            print('Lanes:',', '.join(f'{k}={v["state"]}' for k,v in state['lanes'].items()))
            print('Last handoff:',state['last_handoff'])
            for tid,r in state['tasks'].items():
                if r['status'] not in {'TODO','DONE'}: print(tid,r['status'],r.get('owner'),r.get('blocked_reason') or '')
        elif args.command=='ready':
            print('Structurally ready TODO tasks; external decisions/permissions must still be reviewed:')
            for t in ready_tasks(catalog,state,args.include_optional):
                unresolved=[d for d in t['decision_dependencies'] if state['decision_status'][d]['status']!='RESOLVED']
                print(f'{t["id"]}: {t["title"]} | owner={t["owner_role"]} | unresolved={unresolved or "none"}')
        elif args.command=='render':
            generated=render_documents(catalog); drift=[]
            for rel,content in generated.items():
                target=docs/rel
                if not target.exists() or target.read_text(encoding='utf-8')!=content:
                    drift.append(rel)
                    if not args.check:
                        target.parent.mkdir(parents=True,exist_ok=True); target.write_text(content,encoding='utf-8')
            if args.check and drift: raise PlanError('Generated docs drift: '+', '.join(drift))
            print(f'{"CHECK PASS" if args.check else "RENDERED"}: {len(generated)} generated documents; changed={len(drift)}')
        return 0
    except (PlanError,KeyError,TypeError,ValueError) as exc:
        print(f'ERROR: {exc}',file=sys.stderr); return 2

if __name__=='__main__':
    raise SystemExit(main())
