#!/usr/bin/env python3
"""Static checks for the single-file app (src/app/index.html).

These guard the invariants that are easy to break while editing the file:
script layout, storage keys, provider wiring, and the absence of leaked keys.

Run:  uv run python -m unittest discover -s tests -v
"""
import re
import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
APP = ROOT / 'src' / 'app' / 'index.html'
HTML = APP.read_text(encoding='utf-8')

# The only inline onclick handlers that may exist. Everything else must go
# through the delegated data-action dispatch.
ALLOWED_ONCLICK = (
    'onclick="window.print()"',        # generated print preview document
    "$('#topfocus').onclick=",
    "$('#mobilemenu').onclick=",
    "$('#overlay').onclick=",
)


class AppStructureTests(unittest.TestCase):
    def test_single_inline_application_script(self):
        blocks = re.findall(r'<script>(.*?)</script>', HTML, re.S)
        self.assertEqual(len(blocks), 1, 'the app must keep exactly one inline <script> block')

    def test_pdfjs_resources_are_embedded(self):
        for asset_id in ('pdfjs-core', 'pdfjs-worker', 'pdfjs-resources'):
            self.assertIn(f'<script id="{asset_id}" type="application/octet-stream">', HTML)
        self.assertIn('<script id="PDFJS_LICENSE" type="text/plain">', HTML)

    def test_ai_module_follows_the_core_module(self):
        core = HTML.index('function render(){')
        ai = HTML.index('const aiNavBase=nav')
        self.assertLess(core, ai, 'the AI module patches core functions and must come after them')

    def test_ai_pages_are_registered_in_the_navigation(self):
        self.assertIn("NAV.splice(4,0,['capture'", HTML)

    def test_no_inline_event_handlers_outside_the_allowlist(self):
        found = re.findall(r'onclick=', HTML)
        self.assertEqual(len(found), len(ALLOWED_ONCLICK))
        for allowed in ALLOWED_ONCLICK:
            self.assertIn(allowed, HTML)

    def test_all_learning_state_writes_go_through_commit(self):
        writes = re.findall(r'localStorage\.setItem\(KEY', HTML)
        self.assertEqual(len(writes), 3, 'state writes belong to commit(), load() and import() only')


class AppWiringTests(unittest.TestCase):
    def test_storage_keys(self):
        self.assertIn("KEY='shiguang-study-v1'", HTML)
        self.assertIn("AI_CFG_KEY='shiguang-ai-config-v1'", HTML)

    def test_no_api_key_literals_are_committed(self):
        self.assertIsNone(re.search(r'sk-[A-Za-z0-9_-]{16,}', HTML),
                          'a literal API key was found in the app source')

    def test_both_providers_are_supported(self):
        self.assertIn("'qwen'", HTML)
        self.assertIn("'deepseek'", HTML)
        self.assertIn('enable_thinking', HTML)
        self.assertIn("{thinking:{type:'disabled'}}", HTML)

    def test_transport_is_detected_from_the_session_token(self):
        self.assertIn("meta[name=\"study-token\"]", HTML)

    def test_generation_is_verified_before_saving(self):
        self.assertIn('verified', HTML)
        self.assertIn('aiReview', HTML)


class PackagingTests(unittest.TestCase):
    def test_pyproject_declares_the_console_script(self):
        text = (ROOT / 'pyproject.toml').read_text(encoding='utf-8')
        self.assertIn('[build-system]', text)
        self.assertIn('shiguang-study = "shiguang_study.bridge:main"', text)

    def test_lock_file_is_committed(self):
        self.assertTrue((ROOT / 'uv.lock').exists(), 'uv.lock must be committed')

    def test_bundled_bridge_stays_standalone(self):
        source = (ROOT / 'src' / 'shiguang_study' / 'bridge.py').read_text(encoding='utf-8')
        self.assertNotIn('from .', source,
                         'the bridge must not use relative imports so it can be copied into dist/')
        self.assertNotIn('import shiguang_study', source)


class BuildTests(unittest.TestCase):
    def test_bundle_matches_the_sources(self):
        result = subprocess.run([sys.executable, str(ROOT / 'scripts' / 'build.py'), '--check'],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_bundle_contains_the_app_and_the_bridge(self):
        dist = ROOT / 'dist'
        if not dist.exists():
            self.skipTest('dist/ not built yet')
        for name in ('study-room.html', 'bridge.py', 'launch.bat', 'launch.command'):
            self.assertTrue((dist / name).exists(), f'dist/{name} is missing')
        self.assertEqual((dist / 'study-room.html').read_bytes(), APP.read_bytes())


if __name__ == '__main__':
    unittest.main()
