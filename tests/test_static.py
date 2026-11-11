"""Static checks for the single-file app (src/app/index.html).

These guard the invariants that are easy to break while editing the file:
script layout, storage keys, provider wiring, and the absence of leaked keys.

Run:  uv run pytest
"""
import re
from pathlib import Path

import pytest

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


def test_single_inline_application_script():
    blocks = re.findall(r'<script>(.*?)</script>', HTML, re.DOTALL)
    assert len(blocks) == 1, 'the app must keep exactly one inline <script> block'


@pytest.mark.parametrize('asset_id', ['pdfjs-core', 'pdfjs-worker', 'pdfjs-resources'])
def test_pdfjs_resource_block_is_embedded(asset_id):
    assert f'<script id="{asset_id}" type="application/octet-stream">' in HTML


def test_pdfjs_license_is_embedded():
    assert '<script id="PDFJS_LICENSE" type="text/plain">' in HTML


def test_ai_module_follows_the_core_module():
    core = HTML.index('function render(){')
    ai = HTML.index('const aiNavBase=nav')
    assert core < ai, 'the AI module patches core functions and must come after them'


def test_ai_pages_are_registered_in_the_navigation():
    assert "NAV.splice(4,0,['capture'" in HTML


def test_no_inline_event_handlers_outside_the_allowlist():
    assert len(re.findall(r'onclick=', HTML)) == len(ALLOWED_ONCLICK)
    for allowed in ALLOWED_ONCLICK:
        assert allowed in HTML


def test_no_browser_storage_reads_or_writes():
    assert 'localStorage.getItem' not in HTML
    assert 'localStorage.setItem' not in HTML


@pytest.mark.parametrize('key', [
    'shiguang-study-v1',
    'shiguang-ai-config-v1',
    'shiguang-sync-pending-v1',
])
def test_retired_storage_key_is_cleaned_up_once(key):
    # Only a one-time cleanup of the retired keys is allowed.
    assert f"'{key}'" in HTML


def test_no_api_key_literals_are_committed():
    assert re.search(r'sk-[A-Za-z0-9_-]{16,}', HTML) is None, \
        'a literal API key was found in the app source'


def test_both_providers_are_supported():
    assert "'qwen'" in HTML
    assert "'deepseek'" in HTML
    assert 'enable_thinking' in HTML
    assert "{thinking:{type:'disabled'}}" in HTML


def test_the_page_never_calls_the_provider_directly():
    assert '/chat/completions' not in HTML, 'the browser must not talk to the provider itself'


def test_the_page_keeps_no_api_key_in_memory():
    assert 'aiKey' not in HTML
    assert 'aiDeepseekKey' not in HTML


def test_there_is_no_transport_option():
    assert 'transport' not in HTML


def test_the_key_is_stored_by_the_backend():
    assert "'/api/ai-key'" in HTML


def test_transport_is_detected_from_the_api_token():
    assert 'meta[name="api-token"]' in HTML


def test_generation_is_verified_before_saving():
    assert 'verified' in HTML
    assert 'aiReview' in HTML


def test_pyproject_declares_the_console_script():
    text = (ROOT / 'pyproject.toml').read_text(encoding='utf-8')
    assert '[build-system]' in text
    assert 'shiguang-study = "shiguang_study.webapp.__main__:main"' in text


def test_lock_file_is_committed():
    assert (ROOT / 'uv.lock').exists(), 'uv.lock must be committed'
