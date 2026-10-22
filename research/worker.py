"""Read-only research boundary. No financial state, provider, model or credentials."""
import json
import re
import sys


def assess(request):
    if not isinstance(request, dict) or set(request) - {"symbol", "mode"}:
        raise ValueError("Only symbol and mode are accepted")
    if request.get("mode") != "PAPER":
        raise ValueError("Only PAPER mode is implemented")
    symbol = request.get("symbol")
    if not isinstance(symbol, str) or not re.fullmatch(r"[A-Z0-9.:-]{1,32}", symbol):
        raise ValueError("A bounded instrument symbol is required")
    return {
        "symbol": symbol,
        "mode": "PAPER",
        "status": "STUB",
        "outcome": "INSUFFICIENT_DATA",
        "evidence": [],
        "version": "research-stub-v1",
        "reason": "No licensed financial statements, historical analysis or model connected",
        "financial_state_write_access": False,
    }


def main():
    for line in sys.stdin:
        try:
            if len(line) > 65536:
                raise ValueError("Input exceeds limit")
            output = assess(json.loads(line))
        except (ValueError, TypeError) as error:
            output = {"status": "REJECTED", "error": str(error)}
        print(json.dumps(output, sort_keys=True), flush=True)


if __name__ == "__main__":
    main()
