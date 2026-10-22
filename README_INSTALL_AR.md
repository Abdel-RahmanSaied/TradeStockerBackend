# تثبيت خطة TradeStocker على جهازك

الحزمة تحتوي خطة تطوير وملفات تسليم للـAgents، وليست كود التطبيق. تم إعدادها في بيئة منفصلة؛ لم تُكتب تلقائيًا في مجلد جهازك.

## التثبيت

فك ضغط الحزمة أولًا. في Terminal، انتقل إلى المجلد الناتج `TradeStocker_Development_Pack`، ثم:

```bash
python3 install_docs.py \
  --docs-dir "/Users/asaied/WorkDir/TradeStocker/TradeStockerServices/TradeStockerBackend/docs"
```

الأمر السابق معاينة فقط. بعد التأكد من المسار:

```bash
python3 install_docs.py \
  --docs-dir "/Users/asaied/WorkDir/TradeStocker/TradeStockerServices/TradeStockerBackend/docs" \
  --apply
```

يشترط وجود مجلد `TradeStockerBackend` بالفعل. يحتاج Python 3.9 أو أحدث؛ لا يحتاج مكتبات إضافية أو اتصالًا بالشبكة.

## ماذا تفعل أداة التثبيت؟

تتحقق من hashes الحزمة، وتنسخ ملفات docs الجديدة، ثم تنشئ `AGENTS.md` في جذر TradeStockerBackend أو تضيف له قسم البداية مع الحفاظ على محتواه ونسخة احتياطية. لا تنشئ commit أو push أو cloud resources، ولا تضبط credentials أو تفعّل توصيات.

لو وجدت ملفًا بنفس الاسم بمحتوى مختلف — خصوصًا حالة تنفيذ سبق تحديثها — **تتوقف قبل النسخ** وتطبع التعارضات. لا يوجد خيار force overwrite. عالج الاختلاف مع Agent بدل خسارة سياق الجلسات السابقة.

تحافظ على الملفات غير المتعلقة بالحزمة. الملفات المتطابقة تُترك كما هي، لذلك إعادة التثبيت قبل أي تعديلات آمنة. بعد بدء التنفيذ لا تعِد استيراد bootstrap state فوق الحالة الحالية.

لو تعطل نظام الملفات أثناء النسخ قد توجد ملفات جديدة جزئيًا؛ الأداة لا تدّعي أن النسخ كله transaction ذرية. إعادة التشغيل بعد معالجة السبب تتخطى المتطابق وترفض التعارض.

## التحقق والبدء

```bash
cd "/Users/asaied/WorkDir/TradeStocker/TradeStockerServices/TradeStockerBackend"
python3 docs/tools/planctl.py validate
python3 docs/tools/planctl.py status
python3 docs/tools/planctl.py ready
```

في أول تثبيت بدون شغل سابق، أول مهمة هي `S00-T01`. اقرأ `docs/templates/NEW_SESSION_PROMPT.md` للرسالة الجاهزة التي يبدأ بها أي Agent.

لو عندك Agent يعمل محليًا على المشروع، أعطه الحزمة واطلب منه معاينة وتثبيت الملفات وحل التعارضات إن وجدت، ثم تنفيذ بروتوكول S00. لا تطلب منه افتراض أن الملفات موجودة قبل أن يفحصها.
