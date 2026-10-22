import unittest
from worker import assess


class ResearchBoundaryTests(unittest.TestCase):
    def test_stub_never_claims_research_or_returns(self):
        result = assess({"symbol": "AAPL", "mode": "PAPER"})
        self.assertEqual(result["outcome"], "INSUFFICIENT_DATA")
        self.assertEqual(result["evidence"], [])
        self.assertFalse(result["financial_state_write_access"])
        self.assertNotIn("returns", result)

    def test_actual_live_and_untrusted_fields_rejected(self):
        for request in [
            {"symbol": "AAPL", "mode": "ACTUAL"},
            {"symbol": "AAPL", "mode": "LIVE_ADVISORY"},
            {"symbol": "AAPL", "mode": "PAPER", "approved": True},
            {"symbol": "AAPL", "mode": "PAPER", "cash": 100},
            {"symbol": "", "mode": "PAPER"},
            {"symbol": "A" * 33, "mode": "PAPER"},
        ]:
            with self.subTest(request=request), self.assertRaises(ValueError):
                assess(request)


if __name__ == "__main__":
    unittest.main()
