# SOURCE OF TRUTH — TradeStocker

**Baseline:** `1.0.0` · **Date:** 2026-10-23 · **Owner:** Abdel-Rahman Saied.

هذه الصفحة هي نقطة الدخول المعتمدة لأي Agent. هي تحدد **ما نريد بناءه**؛ أما ما تم بالفعل فيثبته الكود والاختبارات وسجل التنفيذ. صدور الخطة لا يعني اعتماد كل تفصيل تقني جديد ولا اكتمال أي تنفيذ.

## 1. تعريف المنتج وحدوده الثابتة

TradeStocker منصة تحليل ومشورة تداول للأسهم، وليست وسيطًا أو محرك تنفيذ أو صندوقًا يضمن عائدًا. الأسواق: **EGX وUS**. النمط: **Swing + Intraday**. القناة الرسمية للتوصيات: **Telegram**. التنفيذ: **بشري فقط**.

أربع lanes مستقلة:

```text
EGX_SWING     US_SWING
EGX_INTRADAY  US_INTRADAY
```

لكل lane هوية استراتيجية، بيانات مطلوبة، حدود، اختبارات، رسوم، سجل أداء وبوابة تفعيل منفصلة. البنية مشتركة؛ نتيجة سوق أو استراتيجية لا تثبت نجاح الأخرى.

## 2. Invariants لا يجوز لأي Agent تغييرها من تلقاء نفسه

| ID | القاعدة |
|---|---|
| INV-01 | لا broker order APIs أو صلاحيات شراء/بيع/إلغاء أوامر. التكامل مع الوسيط، إن وجد، قراءة فقط. |
| INV-02 | `acknowledged` ≠ `executed`. المنصة تسجل التنفيذ الفعلي الذي أبلغ به المستخدم؛ لا تفترضه. |
| INV-03 | Go هو مالك ledger والمخاطر وتخصيص رأس المال ودورة التوصية. Python لا يعدل هذه الحالة مباشرة. |
| INV-04 | كل توصية دخول قابلة للتنفيذ تحتاج تقييمًا ماليًا وتاريخيًا صالحين + أدلة + بيانات حديثة + مرور سياسة المخاطر. |
| INV-05 | لا توصية دخول إذا كانت معلومة جوهرية ناقصة أو غير موثوقة؛ `INSUFFICIENT_DATA` و`NO_TRADE` نتائج صحيحة. |
| INV-06 | الهدف المالي لا يزيد حدود المخاطرة، ولا يفرض صفقات، ولا يغيّر أفق الصفقة لتعويض خسارة. |
| INV-07 | لا leverage أو short selling أو derivatives أو scalping في baseline. بيع/تقليل المراكز لا يتجاوز المملوك فعليًا. |
| INV-08 | لا استخدام للمستقبل في الاختبارات: وقت نشر المعلومة وإتاحتها أهم من تاريخ الفترة المحاسبية. |
| INV-09 | لا زيادة رصيد أو تكرار قيد أو حجز نفس رأس المال بسبب retry/concurrency. |
| INV-10 | إيقاف الدخول الجديد لا يوقف مراقبة المراكز أو تنبيهات الخطر. التنبيه لا يغلق مركزًا. |
| INV-11 | كل قرار يربط data/strategy/policy/model/prompt/build versions؛ المصادقة والأدلة جزء من القرار. |
| INV-12 | OpenClaw اختياري في النهاية. تعطله أو حذفه لا يؤثر على Core أو Telegram الرسمي. |
| INV-13 | لا أرقام أرباح مضمونة أو probabilities مخترعة. هدف المستخدم ≠ توقع النموذج ≠ ميزانية المخاطرة. |
| INV-14 | كل live lane تبدأ مغلقة وتتطلب مراجعة مستقلة وموافقة بشرية. اكتمال sprint أو deploy لا يفعّلها. |
| INV-15 | PAPER/SHADOW/REPLAY/ACTUAL حالات بيانات وصلاحيات منفصلة؛ إعادة التشغيل لا ترسل نصائح قديمة للإنتاج. |

## 3. الاختيارات المعمارية المعتمدة كأساس للخطة

Go-first، وثلاثة entrypoints: `platform-core`، `market-ingestion`، `research-worker` (Python). كود منظم modular وليس خدمة مستقلة لكل Agent.

- Go: `net/http` + chi، pgx/sqlc، goose، policy kernel واحد قابل للاستخدام من live وbacktest.
- Python: uv، Pydantic، LangGraph، NumPy/pandas، وأدوات البحث والتقييم.
- PostgreSQL للحالة التشغيلية؛ NATS JetStream للأحداث؛ S3/Parquet للأرشيف؛ DuckDB للتحليل التاريخي.
- React/TypeScript/Vite للوحة خاصة؛ API وحيد لمنطق المنتج داخل Go.
- Docker Compose محليًا؛ AWS/ECS/RDS/S3 والتشغيل الدائم لـNATS خط أساس مقترح يحتاج حسم التكلفة والتوافر في `D-06`.
- لا Redis/Celery/Kafka/Kubernetes أو قاعدة vector مستقلة بلا حاجة مثبتة وADR.
- لا تثبيت نسخ متوقعة من الذاكرة: `S00-T05` يختبر التوافق ويثبت الإصدارات المدعومة.

تفاصيل التصميم المقترحة تصبح baseline تنفيذ بعد مراجعة المستودع في S00. تعديل invariant يحتاج موافقة مالك صريحة وADR، وليس مجرد رأي Agent.

## 4. من يملك الحقيقة؟

| نوع المعلومة | المصدر الوحيد |
|---|---|
| النطاق والـinvariants | هذه الصفحة + ADR مُعتمد ومتسق معها |
| شرح القرار ومقايضاته | `adrs/` |
| تعريف التاسك واعتمادياتها ومعايير القبول | `state/task_catalog.json` |
| حالة التنفيذ، owner، blockers، reviews | `state/project_state.json` |
| تفاصيل التسليم والأوامر والنتائج | handoff في `sessions/` + أدلة في `evidence/` |
| الـAPI/event/schema الفعلي بعد بنائه | `contracts/` في المستودع واختبارات التوافق؛ `CONTRACTS.md` خط أساس فقط |
| ما تم تنفيذه بالفعل | Git commit + اختبارات قابلة لإعادة التشغيل + runtime evidence إن كان هناك نشر |
| القواعد السوقية أو حقوق البيانات | المصدر الرسمي/العقد بتاريخ تحقق وسريان؛ ليس نص Agent |

**لا نختار مصدرًا بصمت عند التعارض.** سجّل discrepancy، حافظ على شغل المستخدم، وراجع صاحب القرار. وجود كود لا يوافق invariant لا يجعل الكود صحيحًا؛ وجود وثيقة لا يجعل الميزة منفذة.

## 5. الأمور غير المعروفة حاليًا

Repository identity and the observed HEAD are recorded in state/project_state.json.repo_observation and review evidence. First-session no-Git observations are historical, before the owner first commit. Real EGX/US vendors and rights, actual accounts/settlement, capital, approved risk limits, budgets, evaluation thresholds, production identity and infrastructure still require evidence and approval. The local PAPER increment does not resolve these decisions; see OPEN_DECISIONS.md.

قيم المحادثة السابقة الخاصة بنسب المخاطرة والأهداف وSLOs كانت أمثلة/مقترحات، وليست إعدادات إنتاج معتمدة. لا تعيد استخدامها كقيم افتراضية مُفعّلة.

## 6. أول إجراء وآخر إجراء في كل جلسة

ابدأ بقراءة `AGENT_PLAYBOOK.md` وتشغيل `planctl validate/status/ready` وفحص Git. استأنف الحالة القائمة؛ لا تبدأ S00 من جديد إذا كان فحصها قد أُنجز بأدلة.

اختم بتحديث التاسك في `project_state.json`، وإضافة handoff يحدد commit/dirty files/tests/blockers/next exact action، ثم التحقق من الخطة. **لا يُعد الرد في المحادثة تحديثًا للمستودع.**

## 7. Authorized local development increment (2026-10-23)

The owner explicitly authorized local backend and frontend implementation, tests, local app runs and protocol-compliant local commits. This is a private single-owner development workspace. The dashboard lives in the sibling `TradeStockerFrontend` directory requested by the owner; the Go API remains the sole financial authority. S00-T07/T08 track this bounded increment independently of the full production roadmap.

Operating modes: OFFLINE_REPLAY replays dated inputs without external delivery; SHADOW records decisions without actionable delivery or execution; PAPER uses isolated synthetic balances and explicit human paper actions; LIVE_ADVISORY concerns real advice but still human execution only. The trading increment supports PAPER and synthetic prices only. Separately, the owner authorizes read-only official SEC public filing retrieval and local financial research, with the approved contact email in the SEC User-Agent. These real filing reports cannot satisfy missing price history, enable entry, or activate live lanes. Research data and synthetic price fixtures retain separate provenance. Other modes must fail closed until implemented and reviewed. Production and all live lanes default disabled. Paper fixtures are not live data, paper balances are not actual assets and paper outputs do not prove returns. Acknowledging advice never records a fill.

No provider, risk values, identity provider, capital, settlement capability, bot configuration or production budget is approved by this implementation authorization. D-01 through D-11 remain governed by their recorded evidence requirements. No broker orders, leverage, shorts, options, scalping, production deployment or paid resources are authorized.
