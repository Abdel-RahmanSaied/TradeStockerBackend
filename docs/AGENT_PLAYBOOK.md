# Agent Playbook — Start, Work, Verify, Handoff

## 1. بداية أي session

نفّذ من جذر المستودع الحقيقي. ابدأ بقراءة `AGENTS.md` وأي تعليمات أعلى المسار، ثم بالترتيب:

1. `docs/SOURCE_OF_TRUTH.md`.
2. `docs/state/project_state.json` والحالة الفعلية من Git.
3. الـhandoff المشار إليه في `last_handoff` وأي جلسة حجزت التاسك الحالي.
4. ملف السبرينت الحالي وتعريف التاسك من `task_catalog.json`.
5. الـADR والعقود والأدلة المتعلقة بالتاسك فقط؛ لا يلزم قراءة كل الأرشيف في كل مرة.

```bash
git rev-parse --show-toplevel
git status --short
git branch --show-current
git rev-parse HEAD
python3 docs/tools/planctl.py validate
python3 docs/tools/planctl.py status
python3 docs/tools/planctl.py ready
```

لا تنفذ `git reset --hard` أو clean أو حذف ملفات غير معروفة. لا تعدّل نطاقًا فيه شغل متعارض لمستخدم أو Agent آخر. لا تفترض أن `main` أو remote أو test command محدد موجود قبل الفحص.

## 2. اختيار التاسك وحجزها

اختر أعلى أولوية ذات dependencies مكتملة. `ready` يحدد الجاهزية البنيوية فقط؛ راجع `decision_dependencies` والمتطلبات الخارجية قبل التنفيذ. لا تتظاهر بتوفر provider/credential/approval.

```bash
python3 docs/tools/planctl.py claim S00-T01 \
  --owner local-agent --session 2026-10-23-local-001 \
  --expected-revision 0
```

استبدل revision بالقيمة الحالية. حدّث `repo_observation` بالملاحظات الحقيقية في أول فحص. الأداة تمنع الكتابات المتزامنة في نفس working tree وتفحص revision؛ **ليست قفلًا موزعًا بين clones أو worktrees**. مع عدة agents، يحتاج قائد التنفيذ/لوحة مشتركة لتوزيع المهام وتنسيق دمج ملف الحالة. لا تعتمد على ملف JSON وحده كقفل عالمي.

للاستئناف: اقرأ من يملك IN_PROGRESS/IN_REVIEW وتواصل عبر handoff. لا تسرق الحجز تلقائيًا بسبب قِدم التاريخ. إعادة إسناد العمل تحتاج سببًا مسجلًا ومراجعة التغييرات القائمة.

## 3. بروتوكول التنفيذ

ابدأ بعقد/اختبار failing يوضح المطلوب حيث أمكن. ابنِ أصغر تغيير يحل معايير القبول. لا توسع scope إلى infrastructure أو provider أو risk policy غير معتمد. افصل الفشل التقني عن نقص بيانات حقيقي.

حدود الأمان: لا أوامر تداول، لا أسرار داخل docs، لا نشر أو spend أو تغيير صلاحيات إنتاج بلا إذن. التعليقات في الكود بالإنجليزية. معالجة بيانات مالية أصلية لا تعني السماح بنقلها إلى LLM خارجي؛ تحقق من الحقوق أولًا.

إذا وجدت عملًا منفذًا مسبقًا: تحقق من الاختبارات والسلوك، ثم سجّل دليل الاستفادة منه. لا تعِد كتابته ولا تحسبه DONE بناءً على الاسم فقط.

## 4. الحالة المسموحة

```text
TODO → IN_PROGRESS → IN_REVIEW → DONE
             ↘ BLOCKED ↗
IN_REVIEW → IN_PROGRESS    (review changes)
```

- `TODO`: لم يبدأ التنفيذ.
- `IN_PROGRESS`: owner/session محددان؛ يوجد عمل جارٍ.
- `BLOCKED`: سبب محدد وما يفك العائق؛ ليس بديلًا عن توضيح تقدم جزئي.
- `IN_REVIEW`: التنفيذ جاهز للمراجعة ومعه evidence؛ ليس منشورًا تلقائيًا.
- `DONE`: تحققت معايير القبول، واختبارات موثقة، وcommit معروف، ومراجع مسمى.

لا تعرض status يدويًا في ملفات السبرينتات؛ الحالة موجودة في ملف واحد. إصلاح عيب بعد DONE يكون task إصلاح جديدًا أو reopening موثقًا عبر مراجعة الملف، لا إعادة كتابة سجل النجاح السابق.

```bash
python3 docs/tools/planctl.py transition S00-T01 --to BLOCKED \
  --reason 'Describe the exact missing input and what unblocks it' \
  --expected-revision 1
```

## 5. الدليل والمراجعة

أنشئ evidence من القالب. اكتب الأوامر الفعلية، عدد الاختبارات ونتيجتها، ما لم يُختبر ولماذا، exact commit/tree، والمخاطر المتبقية. Screenshot أو كلام عام ليس بديلًا عن test output قابل للتحقق.

```bash
python3 docs/tools/planctl.py transition S00-T01 --to IN_REVIEW \
  --evidence evidence/repository-inventory.md --expected-revision 2

python3 docs/tools/planctl.py transition S00-T01 --to DONE \
  --evidence evidence/repository-inventory.md \
  --verified-commit <actual-git-commit> --reviewed-by <reviewer> \
  --summary 'What was verified, not what is merely planned' \
  --expected-revision 3
```

الـvalidator يتحقق من شكل السجل والمراجع، لكنه **لا يثبت أن الادعاءات داخل الدليل صحيحة**. المراجع مسؤول عن تشغيل الاختبارات ومطابقة الـcommit. لا تختلق reviewer أو SHA لاجتياز الأداة. عند غياب مراجعة حقيقية، اترك IN_REVIEW.

## 6. تسليم نهاية الجلسة

استخدم `templates/SESSION_HANDOFF.md` وسجّل في `sessions/<date>-<unique-id>.md`:

- ماذا تغير فعليًا، وما لم يكتمل.
- branch/HEAD والملفات غير الملتزمة وسببها؛ لا تفرض commit أو push بلا تفويض.
- التاسكات المتأثرة، الاختبارات بنتائجها، الأدلة، العوائق.
- **الأمر أو الخطوة التالية بالضبط**، ومخاطر تكرار أثر أو إعادة تشغيل job.

حدّث `last_session` و`last_handoff` وrevision مرة واحدة من خلال `planctl handoff`:

```bash
python3 docs/tools/planctl.py handoff \
  --session 2026-10-23-local-001 \
  --file sessions/2026-10-23-local-001.md \
  --expected-revision <current-revision>
python3 docs/tools/planctl.py validate
python3 docs/tools/planctl.py render --check
```

## 7. تعديل الخطة

عدّل تعريفات المهام في `state/task_catalog.json` لا في الملفات المولدة. عند إضافة task أضف سجل TODO مناظرًا في `project_state.json`. حافظ على IDs القديمة. سجل سبب التغيير في `CHANGELOG.md`، وأضف ADR لتغيير معماري مهم، ثم:

```bash
python3 docs/tools/planctl.py render
python3 docs/tools/planctl.py validate
```

قسّم التاسك الكبيرة أو المتوقفة على سوق إلى subtasks بإصدارات واضحة بدل تعطيل lane مستقلة. لا تُسقط شرطًا ماليًا أو أمنيًا لمجرد تجاوز العائق.

## 8. شكل تقرير الـAgent للمستخدم

اذكر: المنجز، الملفات، الاختبارات، ما لم يُفعل (commit/push/deploy/activation)، العوائق، والخطوة التالية. لا تقل «جاهز للإنتاج» إذا الاختبارات كانت mocks فقط، ولا «تم تحديث جهازك» إذا الملفات أُنشئت في sandbox فقط.
