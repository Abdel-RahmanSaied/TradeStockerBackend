# TradeStocker — Development Plan

> ملف مولّد من `state/task_catalog.json`. لا تعدّل تفاصيله يدويًا؛ استخدم `planctl.py render`.

**Baseline:** `1.0.0` · **16 سبرينت أساسي + 1 اختياري** · **104 تاسك**.

## طريقة استخدام الخطة

السبرينتات هنا حزم عمل مرتبة وليست وعدًا بمدة تقويمية. حجم الفريق ووقت التطوير وشراء البيانات لم تُحسم. في S00 قسّم المهام الكبيرة إلى وحدات قابلة للإنجاز ضمن cadence متفق عليه، دون إسقاط بوابات الجودة.

اتبع الاعتماديات على مستوى التاسك، لا رقم السبرينت فقط. بعض العمل يمكن تنفيذه بالتوازي بعد جاهزية مدخلاته. لا تبدأ مهام اختيارية بلا اعتماد. حجم S/M/L نسبي وليس تقدير أيام.

الحالة الفعلية في [project_state.json](state/project_state.json). تعريفات التاسكات في [task_catalog.json](state/task_catalog.json). ابدأ من [SOURCE_OF_TRUTH.md](SOURCE_OF_TRUTH.md) وبروتوكول [AGENT_PLAYBOOK.md](AGENT_PLAYBOOK.md).

## خريطة السبرينتات

| Sprint | النطاق | Tasks | بوابة الخروج |
|---|---|---:|---|
| [S00](sprints/S00.md) | اكتشاف المستودع وتثبيت النطاق والمتطلبات الخارجية | 8 | A repository inventory, explicit scope, provider evidence and an actionable unresolved-decision register exist. No live lane is enabled. |
| [S01](sprints/S01.md) | أساس المشروع والأمان وCI | 6 | Go core, Go ingestion and Python worker boot locally; CI runs baseline checks; secrets and service identities are separated. |
| [S02](sprints/S02.md) | العقود وقاعدة البيانات واعتمادية الأحداث | 6 | Schema migrations, instrument/time rules and event contracts pass integration and duplicate-delivery tests. |
| [S03](sprints/S03.md) | بيانات السوق الأمريكي والمصري وجودتها | 6 | Synthetic adapters pass all gates; each real feed has separately recorded coverage, entitlements and validation evidence. |
| [S04](sprints/S04.md) | سجل المحفظة والتنفيذ الفعلي والتسوية | 6 | Account-scoped cash, positions and manual fills reconcile; duplicate and concurrent events cannot create money or double-spend available cash. |
| [S05](sprints/S05.md) | محرك المخاطر ودورة حياة التوصية | 6 | Live and replay use the same risk implementation; stale/unfunded/unauthorized advice is rejected; pausing entries preserves position monitoring. |
| [S06](sprints/S06.md) | المحاكاة التاريخية وخطوط الأساس والتقييم | 6 | Leakage tests, deterministic baselines and realistic execution assumptions are documented; no result is treated as evidence of guaranteed returns. |
| [S07](sprints/S07.md) | القوائم المالية وتاريخ السهم والأدلة | 6 | Evidence-backed assessments exist with publication timing and accounting context; missing material inputs block new entries. |
| [S08](sprints/S08.md) | لجنة المستشارين وتقييم النماذج | 6 | Committee outputs are structured and auditable; no model output can mutate financial state or bypass policy. |
| [S09](sprints/S09.md) | استراتيجيات Swing ومتابعة المراكز | 6 | Both swing lanes have reproducible evaluation artifacts; each remains disabled until its independent activation gates pass. |
| [S10](sprints/S10.md) | Telegram والتنفيذ البشري والتشغيل الورقي | 6 | Authenticated Telegram interactions, expiry, delivery ambiguity and paper-versus-actual separation pass end-to-end tests. |
| [S11](sprints/S11.md) | Trading Intelligence واستراتيجيات Intraday | 6 | Intraday decisions respect real feeds, human latency, account eligibility and session cutoffs; EGX and US are evaluated separately. |
| [S12](sprints/S12.md) | الأهداف والسيناريوهات وتخصيص رأس المال | 6 | Goals are versioned, evidence-based and separated from forecasts; cash flows and actual performance are correctly attributed. |
| [S13](sprints/S13.md) | لوحة المتابعة والتشغيل وسجل المراجعة | 6 | Owner can inspect actual state, evidence, goals, incidents and costs; access and mode separation are tested. |
| [S14](sprints/S14.md) | النشر الإنتاجي والتعافي واختبارات الاعتمادية | 6 | Infrastructure, signed artifacts, migration strategy, backup restore and load/chaos tests pass independently reviewed release gates. |
| [S15](sprints/S15.md) | تفعيل كل سوق واستراتيجية بشكل مستقل | 6 | Each lane has a recorded GO or NO_GO; actual execution remains human-only; continuing evaluation and disable procedures are operational. |
| [S16](sprints/S16.md) | OpenClaw: طبقة اختيارية للقراءة فقط — اختياري | 6 | Removing or compromising the optional assistant cannot submit orders, change risk, mutate portfolio state or stop official notifications. |

## تسلسل القرارات المهمة

**S00–S02:** افحص الموجود، ثبّت العقود والهوية وCI والتخزين والأحداث. بدأ الحصول على البيانات في S00؛ لا تنتظر آخر المشروع.

**S03–S06:** بيانات صحيحة، ثم محفظة وقيود مالية، ثم محاكاة وتقييم مسجل مسبقًا. لا تبنِ لجنة تصدر توصيات قبل حماية المال والوقت والهوية.

**S07–S11:** أدلة مالية وتاريخية، لجنة مشروطة، Swing، Telegram، ثم Intraday القابل للتنفيذ البشري. نفّذ البيانات والاختبارات لكل سوق بشكل مستقل.

**S12–S15:** أهداف لا تتجاوز المخاطر، لوحة تشغيل، اختبارات نشر وتعافٍ، ثم تفعيل مستقل بأدلة. الأمان والاختبارات موجودة منذ S01؛ S14 تحقق إنتاجي وليس أول مرة نفكر فيهما.

**S16:** OpenClaw read-only بعد core وباعتماد منفصل؛ لا يدخل في critical path.

## بوابات لا يتجاوزها التقويم

اكتمال الكود لا يعني جدوى استراتيجية، ونجاح Paper لا يضمن الربح. غياب بيانات أو حقوق استخدام أو قواعد حساب صحيحة يمنع lane المتأثرة. لا يشترط أن تحقق الأربع lanes GO في نفس الوقت.

مراجعة يمكن أن تنتهي NO_GO وتُعد مراجعة مكتملة، لكن adapter غير منفذ أو اختبار لم يُشغل لا يتحول إلى DONE بسبب كتابة تقرير. ملفات التجميع تسجل جاهزية المتاح وعوائق الباقي؛ لا تستعمل نتيجة سوق لتغطية سوق آخر.

## ما تحتويه كل تاسك

ID ثابت، owner role، نطاق، dependencies، قرارات يجب مراجعتها، مخرجات مقترحة، acceptance criteria واختبارات، وربط بالمتطلبات. المسارات مبدئية حتى S00 يطابقها مع المستودع الفعلي.

## بداية التنفيذ

أول تاسك `S00-T01`. شغّل `python3 docs/tools/planctl.py validate` ثم `status` و`ready`. لا تجعل إنشاء هذه الوثائق دليلًا على تنفيذ المنتج.
