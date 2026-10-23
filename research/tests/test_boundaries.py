"""Regression check on source: the package imports only these standard-library
modules, so today its code contains no network, database, process or file client.

This reads import lines only. It is not a sandbox: the package and the CLI run
with the caller's OS permissions. Only the parser subprocess that Go starts gets
an isolated environment (checked by TestPythonReceivesNoSecrets in Go)."""
import ast
from pathlib import Path
import unittest

PACKAGE = Path(__file__).resolve().parents[1] / "src" / "tradestocker_research"
ALLOWED = {"collections", "datetime", "decimal", "json", "re", "sys", "tradestocker_research"}


def imports(source):
    names = set()
    for node in ast.walk(ast.parse(source)):
        if isinstance(node, ast.Import):
            names.update(alias.name.split(".")[0] for alias in node.names)
        elif isinstance(node, ast.ImportFrom):
            names.add("." if node.level else node.module.split(".")[0])
        elif isinstance(node, ast.Call) and getattr(node.func, "id", "") in {"__import__", "exec", "eval", "compile"}:
            names.add(node.func.id)
    return names


class PackageBoundaryTests(unittest.TestCase):
    def test_modules_import_only_allowed_standard_library(self):
        modules = sorted(PACKAGE.glob("*.py"))
        self.assertEqual([m.name for m in modules], ["__init__.py", "financials.py", "worker.py"])
        for module in modules:
            with self.subTest(module=module.name):
                self.assertLessEqual(imports(module.read_text()), ALLOWED)

    def test_parser_runs_alone_so_imports_nothing_from_the_package(self):
        self.assertNotIn("tradestocker_research", imports((PACKAGE / "financials.py").read_text()))

    def test_checker_flags_forbidden_imports(self):
        for source in ["import socket", "from urllib import request", "import os.path", "from . import x",
                       "__import__('http')", "import json, sqlite3", "exec('import ssl')"]:
            with self.subTest(source=source):
                self.assertFalse(imports(source) <= ALLOWED)

    def test_only_one_console_entrypoint(self):
        try:
            import tomllib
        except ImportError:
            self.skipTest("tomllib needs Python 3.11+; the supported interpreter is in .python-version")
        project = tomllib.loads((PACKAGE.parents[1] / "pyproject.toml").read_text())["project"]
        self.assertEqual(project["scripts"], {"research-worker": "tradestocker_research.worker:main"})


if __name__ == "__main__":
    unittest.main()
