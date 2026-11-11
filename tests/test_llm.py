"""Unit tests for the Qwen / DeepSeek forwarding logic (src/shiguang_study/llm.py).

Standard library only, no network access: the upstream provider is replaced by
an in-process fake.
"""
import io
import json
import urllib.error
from unittest import mock

import pytest

from shiguang_study import llm

QWEN_BASE = 'https://dashscope.aliyuncs.com/compatible-mode/v1'
DEEPSEEK_BASE = 'https://api.deepseek.com'
SECRET = 'sk-TESTKEY0123456789ABCDEF'

BODY = {'model': 'qwen-plus',
        'messages': [{'role': 'user', 'content': 'hi'}],
        'response_format': {'type': 'json_object'}, 'max_tokens': 8192}


class _FakeResponse:
    def __init__(self, payload):
        self._data = payload if isinstance(payload, bytes) else json.dumps(payload).encode()

    def read(self, n=-1):
        return self._data if n is None or n < 0 else self._data[:n]

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


class _FakeOpener:
    """Stands in for urllib's opener and records the outgoing request."""

    def __init__(self, payload=None, error=None):
        self.payload = payload
        self.error = error
        self.request = None

    def open(self, req, timeout=None):
        self.request = req
        if self.error is not None:
            raise self.error
        return _FakeResponse(self.payload)

    def body(self):
        return json.loads(self.request.data.decode())


@pytest.fixture()
def fake_opener():
    """Patch urllib's opener factory; returns the recorder.

    Usage: ``opener = fake_opener(payload)`` or ``fake_opener(error=exc)``.
    """
    recorders: list[_FakeOpener] = []

    def build(payload=None, error=None):
        opener = _FakeOpener(payload, error)
        recorders.append(opener)
        mock.patch('urllib.request.build_opener', return_value=opener).start()
        return opener

    yield build
    mock.patch.stopall()


@pytest.mark.parametrize('url', [
    QWEN_BASE,
    'https://dashscope-intl.aliyuncs.com/compatible-mode/v1',
    'https://dashscope.aliyuncs.com/compatible-mode/v1/',
    'https://biz-123.aliyuncs.com/compatible-mode/v1',
])
def test_accepts_official_qwen_bases(url):
    assert llm.endpoint_provider(url) == 'qwen'


@pytest.mark.parametrize('url', [
    DEEPSEEK_BASE,
    'https://api.deepseek.com/',
    'https://api.deepseek.com/v1',
])
def test_accepts_official_deepseek_bases(url):
    assert llm.endpoint_provider(url) == 'deepseek'


@pytest.mark.parametrize('url', [
    '',
    None,
    123,
    'http://dashscope.aliyuncs.com/compatible-mode/v1',        # not https
    'https://dashscope.aliyuncs.com:8443/compatible-mode/v1',  # custom port
    'https://user:pw@api.deepseek.com',                        # embedded credentials
    'https://api.deepseek.com/v1?x=1',                         # query string
    'https://api.deepseek.com/v1#frag',                        # fragment
    'https://api.openai.com/v1',                               # wrong host
    'https://dashscope.aliyuncs.com/compatible-mode/v2',       # wrong path
    'https://aliyuncs.com.evil.example/compatible-mode/v1',    # suffix confusion
])
def test_rejects_everything_else(url):
    with pytest.raises(ValueError):
        llm.endpoint_provider(url)


def test_endpoint_url_appends_chat_completions():
    assert llm.endpoint_url(DEEPSEEK_BASE) == 'https://api.deepseek.com/chat/completions'
    assert llm.endpoint_url(QWEN_BASE) == \
        'https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions'


def test_qwen_gets_its_own_thinking_flag(fake_opener):
    opener = fake_opener({'choices': [{'message': {'content': '{"ok":true}'}}]})
    status, result = llm.upstream(QWEN_BASE, SECRET, dict(BODY))
    assert status == 200
    assert result['choices'][0]['message']['content'] == '{"ok":true}'
    body = opener.body()
    assert body['enable_thinking'] is False
    assert 'thinking' not in body
    assert body['stream'] is False
    assert body['response_format'] == {'type': 'json_object'}


def test_deepseek_gets_its_own_thinking_flag(fake_opener):
    opener = fake_opener({'choices': [{'message': {'content': '{"ok":true}'}}]})
    status, _ = llm.upstream(DEEPSEEK_BASE, SECRET, dict(BODY, model='deepseek-flash'))
    assert status == 200
    body = opener.body()
    assert body['thinking'] == {'type': 'disabled'}
    assert 'enable_thinking' not in body


def test_only_whitelisted_fields_are_forwarded(fake_opener):
    opener = fake_opener({'choices': [{'message': {'content': '{}'}}]})
    llm.upstream(QWEN_BASE, SECRET, dict(BODY, temperature=0.7, top_p=1))
    body = opener.body()
    assert 'temperature' not in body
    assert 'top_p' not in body
    assert set(body) - {'model', 'messages', 'response_format', 'max_tokens'} == \
        {'stream', 'enable_thinking'}


@pytest.mark.parametrize('key', ['', '   ', 'a' * 501, 'key\nwith-newline', None])
def test_rejects_missing_key(key):
    with pytest.raises(ValueError):
        llm.upstream(QWEN_BASE, key, dict(BODY))


def test_rejects_request_without_model_or_messages():
    with pytest.raises(ValueError):
        llm.upstream(QWEN_BASE, SECRET, {'messages': []})
    with pytest.raises(ValueError):
        llm.upstream(QWEN_BASE, SECRET, {'model': 'qwen-plus'})


def test_upstream_error_redacts_the_key(fake_opener):
    payload = json.dumps({'error': {'message': f'invalid api key {SECRET}'}}).encode()
    error = urllib.error.HTTPError('https://api.deepseek.com/chat/completions', 401,
                                   'Unauthorized', {}, io.BytesIO(payload))
    fake_opener(error=error)
    status, result = llm.upstream(DEEPSEEK_BASE, SECRET, dict(BODY))
    assert status == 401
    message = result['error']['message']
    assert SECRET not in message
    assert '[密钥已隐藏]' in message


def test_oversized_response_is_rejected(fake_opener):
    fake_opener(b'x' * (llm.MAX_RESPONSE + 2))
    status, result = llm.upstream(QWEN_BASE, SECRET, dict(BODY))
    assert status == 502
    assert '模型响应过大' in result['error']['message']


def test_network_failure_is_reported_as_bad_gateway(fake_opener):
    fake_opener(error=OSError('connection refused'))
    status, result = llm.upstream(QWEN_BASE, SECRET, dict(BODY))
    assert status == 502
    assert '无法连接' in result['error']['message']
