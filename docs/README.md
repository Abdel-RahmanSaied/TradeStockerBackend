# TradeStocker — Development Plan & Agent Continuity

**نسخة الخطة:** `1.0.0` · **تاريخ الإعداد:** 23 أكتوبر 2026 · **نوع الحزمة:** وثائق تخطيط وأدوات إدارة الخطة، وليست تنفيذًا للمنصة.

المسار المقصود على جهاز المالك:

```text
/Users/asaied/WorkDir/TradeStocker/TradeStockerServices/TradeStockerBackend/docs
```

## ابدأ من هنا

اقرأ [SOURCE_OF_TRUTH.md](SOURCE_OF_TRUTH.md)، ثم [AGENT_PLAYBOOK.md](AGENT_PLAYBOOK.md)، ثم شغّل من جذر المستودع:

```bash
python3 docs/tools/planctl.py validate
python3 docs/tools/planctl.py status
python3 docs/tools/planctl.py ready
```

**أول مهمة:** `S00-T01` — فحص المستودع الفعلي ومقارنة حالته بالخطة. لا تفترض أن المشروع فارغ، ولا تنشئ backend جديدًا قبل فحص الموجود.

## المحتويات

| المرجع | وظيفته |
|---|---|
| [SOURCE_OF_TRUTH.md](SOURCE_OF_TRUTH.md) | القرارات الثابتة، حدود المنتج، ومكان الحقيقة لكل نوع معلومة |
| [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md) | خريطة 16 سبرينت أساسيًا + سبرينت اختياري، دون مواعيد وهمية |
| [sprints/](sprints/) | ملف لكل سبرينت؛ النطاق والتاسكات واعتمادياتها ومعايير قبولها |
| [state/task_catalog.json](state/task_catalog.json) | المصدر القابل للمعالجة لتعريف الـ102 تاسك؛ منه تتولد ملفات السبرينتات |
| [state/project_state.json](state/project_state.json) | الحالة الوحيدة المعتمدة للتنفيذ: المهام، الملاك، العوائق، والمراجعات |
| [AGENT_PLAYBOOK.md](AGENT_PLAYBOOK.md) | بداية الجلسة، حجز التاسك، التنفيذ، الاختبار، والتسليم |
| [ARCHITECTURE.md](ARCHITECTURE.md) | حدود Go/Python، التخزين، المسؤوليات، ومسار القرار |
| [CONTRACTS.md](CONTRACTS.md) | خط أساس للعقود والـAPI والبيانات، وليس ملفات تطبيق منفذة |
| [RESEARCH_AND_AGENTS.md](RESEARCH_AND_AGENTS.md) | أدوار اللجنة والمتخصصين الخمسة وتوقيت تشغيلهم |
| [QUALITY_GATES.md](QUALITY_GATES.md) | تعريف الجاهزية والإنجاز وبوابات تفعيل كل lane |
| [DATA_PROVIDER_MATRIX.md](DATA_PROVIDER_MATRIX.md) | قدرات ومصادر البيانات وما يحتاج عقدًا أو تحققًا |
| [OPEN_DECISIONS.md](OPEN_DECISIONS.md) | القرارات غير المحسومة ومن يحسمها ومتى تصبح مانعة |
| [adrs/README.md](adrs/README.md) | قرارات المعمارية ومبرراتها |
| [REQUIREMENTS_TRACEABILITY.md](REQUIREMENTS_TRACEABILITY.md) | ربط كل متطلب بالتاسكات التي تنفذه |
| [SECURITY_AND_OPERATIONS.md](SECURITY_AND_OPERATIONS.md) | الأمان، CI/CD، التشغيل، والتعافي |
| [sessions/](sessions/) | تسليمات الجلسات؛ لا تحل محل حالة التنفيذ |
| [templates/](templates/) | قوالب الأدلة والقرارات والتسليم وإطلاق كل lane |
| [SOURCES.md](SOURCES.md) | المصادر الرسمية وحدود ما تحققنا منه |

## الوضع الحقيقي عند إنشاء الحزمة

تمت كتابة الخطة وأدواتها فقط. **لم يتم فتح مستودع جهاز المالك أو تنفيذ أو اختبار أو نشر منتج TradeStocker.** جميع تاسكات التنفيذ تبدأ `TODO`، وجميع lanes تبدأ `DISABLED`.

الفحص الذي يمكن تنفيذه هنا يخص سلامة الوثائق وأدوات الخطة، ولا يُعد نجاحًا لاختبارات المنتج. أول Agent محلي هو المسؤول عن ربط الخطة بالـHEAD والملفات الفعلية.

## قواعد تمنع ضياع السياق

لا تستخدم ذاكرة المحادثة بدل ملفات المشروع. لا تسجل نسبة إنجاز من تقديرك. لا تضع حالة التاسك في أكثر من ملف. ملفات السبرينتات تعريفات مولدة، وليست قوائم checkboxes يدوية. كل `DONE` يحتاج دليلًا ومراجعة ومرجع commit يمكن التحقق منه.

تقرأ أوامر الخطة ملفات محلية فقط. لا تتصل بوسيط أو مزود بيانات، ولا تفعّل التداول، ولا تُنشئ مهام مجدولة خارج المستودع.
