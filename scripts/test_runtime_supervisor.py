import importlib.util
import sys
import tempfile
import time
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("supervisor", Path(__file__).with_name("runtime-supervisor.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class RecoveryTest(unittest.TestCase):
    def test_backend_exit_stops_frontend(self):
        with tempfile.TemporaryDirectory() as directory:
            started = time.monotonic()
            result = module.supervise([
                ("backend", [sys.executable, "-c", "raise SystemExit(3)"], directory),
                ("frontend", [sys.executable, "-c", "import time; time.sleep(60)"], directory),
            ], directory, grace=2)
            self.assertEqual(result, 1)
            self.assertLess(time.monotonic() - started, 5)

    def test_normal_unexpected_exit_also_requests_recovery(self):
        with tempfile.TemporaryDirectory() as directory:
            self.assertEqual(module.supervise([
                ("frontend", [sys.executable, "-c", "pass"], directory),
            ], directory, grace=2), 1)


if __name__ == "__main__":
    unittest.main()
