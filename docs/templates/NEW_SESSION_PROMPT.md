# رسالة بداية أي Agent session جديدة

```text
اشتغل داخل مستودع TradeStockerBackend الحالي، ولا تفترض أن المشروع فارغ.

اقرأ AGENTS.md وأي تعليمات أعلى المسار، ثم:
- docs/SOURCE_OF_TRUTH.md
- docs/AGENT_PLAYBOOK.md
- docs/state/project_state.json
- آخر handoff المشار إليه في الحالة
- تعريف التاسك والسبرينت والعقود/ADRs المرتبطة به

افحص git status/branch/HEAD بدون حذف أو إعادة ضبط أي شغل قائم.
شغّل python3 docs/tools/planctl.py validate وstatus وready.
استأنف المهمة المملوكة للجلسة السابقة أو اختر أول task جاهزة حسب dependencies.
لا تبدأ الخطة من الصفر، ولا تغيّر النطاق أو حدود المخاطر أو تختار مزودًا مدفوعًا بصمت.

احجز التاسك مع owner/session/revision، ونفّذ معايير قبولها واختباراتها.
Go يملك ledger/risk/recommendations، وPython للـAI/Quant.
EGX/US × Swing/Intraday، Telegram، وتنفيذ بشري فقط.
OpenClaw اختياري في النهاية. لا broker orders ولا ضمان أرباح ولا target-driven risk escalation.

قبل إنهاء الجلسة: أضف evidence وhandoff، وحدّث الحالة، وشغّل validator.
لا تسجل DONE بدون دليل وcommit ومراجعة حقيقية. لا تدّعِ push/deploy/activation لم يحدث.
ابدأ بتقرير قصير عن حالة المستودع والتاسك التي ستعمل عليها، ثم نفّذها.
```
