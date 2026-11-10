#!/usr/bin/env python3
"""Unit and integration tests for the local AI bridge (src/shiguang_study/bridge.py).

Standard library only, no network access: the upstream provider is replaced by
an in-process fake, and the HTTP server is started on an ephemeral port.

Run:  uv run python -m unittest discover -s tests -v
"""
import http.client
import io
import json
import sys
import threading
import unittest
import urllib.error
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'src'))

import shiguang_study.bridge as bridge  # noqa: E402  (path set up above)

QWEN_BASE = 'https://dashscope.aliyuncs.com/compatible-mode/v1'
DEEPSEEK_BASE = 'https://api.deepseek.com'
SECRET = 'sk-TESTKEY0123456789ABCDEF'


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


def fake_opener(payload=None, error=None):
    opener = _FakeOpener(payload, error)
    return opener, mock.patch('urllib.request.build_opener', return_value=opener)


class EndpointWhitelistTests(unittest.TestCase):
    def test_accepts_official_qwen_bases(self):
        for url in (QWEN_BASE,
                    'https://dashscope-intl.aliyuncs.com/compatible-mode/v1',
                    'https://dashscope.aliyuncs.com/compatible-mode/v1/',
                    'https://biz-123.aliyuncs.com/compatible-mode/v1'):
            self.assertEqual(bridge.endpoint_provider(url), 'qwen', url)

    def test_accepts_official_deepseek_bases(self):
        for url in (DEEPSEEK_BASE, 'https://api.deepseek.com/', 'https://api.deepseek.com/v1'):
            self.assertEqual(bridge.endpoint_provider(url), 'deepseek', url)

    def test_rejects_everything_else(self):
        rejected = (
            '', None, 123,
            'http://dashscope.aliyuncs.com/compatible-mode/v1',      # not https
            'https://dashscope.aliyuncs.com:8443/compatible-mode/v1',  # custom port
            'https://user:pw@api.deepseek.com',                      # embedded credentials
            'https://api.deepseek.com/v1?x=1',                       # query string
            'https://api.deepseek.com/v1#frag',                      # fragment
            'https://api.openai.com/v1',                             # wrong host
            'https://dashscope.aliyuncs.com/compatible-mode/v2',     # wrong path
            'https://aliyuncs.com.evil.example/compatible-mode/v1',  # suffix confusion
        )
        for url in rejected:
            with self.assertRaises(ValueError, msg=url):
                bridge.endpoint_provider(url)

    def test_endpoint_url_appends_chat_completions(self):
        self.assertEqual(bridge.endpoint_url(DEEPSEEK_BASE),
                         'https://api.deepseek.com/chat/completions')
        self.assertEqual(bridge.endpoint_url(QWEN_BASE),
                         'https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions')


class UpstreamRequestTests(unittest.TestCase):
    BODY = {'model': 'qwen-plus', 'messages': [{'role': 'user', 'content': 'hi'}],
            'response_format': {'type': 'json_object'}, 'max_tokens': 8192}

    def test_qwen_gets_its_own_thinking_flag(self):
        opener, patch = fake_opener({'choices': [{'message': {'content': '{"ok":true}'}}]})
        with patch:
            status, result = bridge.upstream(QWEN_BASE, SECRET, dict(self.BODY))
        self.assertEqual(status, 200)
        self.assertEqual(result['choices'][0]['message']['content'], '{"ok":true}')
        body = opener.body()
        self.assertIs(body['enable_thinking'], False)
        self.assertNotIn('thinking', body)
        self.assertIs(body['stream'], False)
        self.assertEqual(body['response_format'], {'type': 'json_object'})

    def test_deepseek_gets_its_own_thinking_flag(self):
        opener, patch = fake_opener({'choices': [{'message': {'content': '{"ok":true}'}}]})
        with patch:
            status, _ = bridge.upstream(DEEPSEEK_BASE, SECRET,
                                        dict(self.BODY, model='deepseek-flash'))
        self.assertEqual(status, 200)
        body = opener.body()
        self.assertEqual(body['thinking'], {'type': 'disabled'})
        self.assertNotIn('enable_thinking', body)

    def test_only_whitelisted_fields_are_forwarded(self):
        opener, patch = fake_opener({'choices': [{'message': {'content': '{}'}}]})
        with patch:
            bridge.upstream(QWEN_BASE, SECRET, dict(self.BODY, temperature=0.7, top_p=1))
        body = opener.body()
        for dropped in ('temperature', 'top_p'):
            self.assertNotIn(dropped, body)
        # Only the whitelisted fields plus the bridge's own flags survive.
        self.assertEqual(set(body) - {'model', 'messages', 'response_format', 'max_tokens'},
                         {'stream', 'enable_thinking'})

    def test_rejects_missing_key(self):
        for key in ('', '   ', 'a' * 501, 'key\nwith-newline', None):
            with self.assertRaises(ValueError):
                bridge.upstream(QWEN_BASE, key, dict(self.BODY))

    def test_rejects_request_without_model_or_messages(self):
        with self.assertRaises(ValueError):
            bridge.upstream(QWEN_BASE, SECRET, {'messages': []})
        with self.assertRaises(ValueError):
            bridge.upstream(QWEN_BASE, SECRET, {'model': 'qwen-plus'})

    def test_upstream_error_redacts_the_key(self):
        payload = json.dumps({'error': {'message': f'invalid api key {SECRET}'}}).encode()
        error = urllib.error.HTTPError('https://api.deepseek.com/chat/completions', 401,
                                       'Unauthorized', {}, io.BytesIO(payload))
        _opener, patch = fake_opener(error=error)
        with patch:
            status, result = bridge.upstream(DEEPSEEK_BASE, SECRET, dict(self.BODY))
        self.assertEqual(status, 401)
        message = result['error']['message']
        self.assertNotIn(SECRET, message)
        self.assertIn('[密钥已隐藏]', message)

    def test_oversized_response_is_rejected(self):
        _opener, patch = fake_opener(b'x' * (bridge.MAX_RESPONSE + 2))
        with patch:
            status, result = bridge.upstream(QWEN_BASE, SECRET, dict(self.BODY))
        self.assertEqual(status, 502)
        self.assertIn('模型响应过大', result['error']['message'])

    def test_network_failure_is_reported_as_bad_gateway(self):
        _opener, patch = fake_opener(error=OSError('connection refused'))
        with patch:
            status, result = bridge.upstream(QWEN_BASE, SECRET, dict(self.BODY))
        self.assertEqual(status, 502)
        self.assertIn('无法连接', result['error']['message'])


class BridgeServerTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = bridge.StudyServer(('127.0.0.1', 0))
        cls.port = cls.server.server_port
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()

    def request(self, method, path, headers=None, body=None):
        conn = http.client.HTTPConnection('127.0.0.1', self.port, timeout=5)
        try:
            conn.request(method, path, body=body, headers=headers or {})
            response = conn.getresponse()
            payload = response.read().decode('utf-8')
            return response.status, payload
        finally:
            conn.close()

    def test_status_is_reachable_from_the_local_host(self):
        status, payload = self.request('GET', '/api/status')
        self.assertEqual(status, 200)
        data = json.loads(payload)
        self.assertEqual(data['app'], 'shiguang-study')
        self.assertIn('providers', data)

    def test_status_never_returns_key_material(self):
        with mock.patch.dict('os.environ', {'DASHSCOPE_API_KEY': SECRET}):
            server = bridge.StudyServer(('127.0.0.1', 0))
        server.server_close()
        self.assertEqual(server.api_key, SECRET)
        body = json.dumps({'hasKey': bool(server.api_key)})
        self.assertNotIn(SECRET, body)
        self.assertTrue(json.loads(body)['hasKey'])

    def test_foreign_host_header_is_rejected(self):
        status, payload = self.request('GET', '/api/status', headers={'Host': 'evil.example'})
        self.assertEqual(status, 403)
        self.assertIn('仅允许本机访问', json.loads(payload)['error']['message'])

    def test_foreign_origin_is_rejected(self):
        status, _ = self.request('GET', '/api/status',
                                 headers={'Origin': 'http://evil.example'})
        self.assertEqual(status, 403)

    def test_local_origin_is_accepted(self):
        status, _ = self.request('GET', '/api/status',
                                 headers={'Origin': f'http://127.0.0.1:{self.port}'})
        self.assertEqual(status, 200)

    def test_unknown_path_returns_json_404(self):
        status, payload = self.request('GET', '/nope')
        self.assertEqual(status, 404)
        self.assertIn('error', json.loads(payload))

    def test_app_page_carries_the_session_token(self):
        status, payload = self.request('GET', '/')
        self.assertEqual(status, 200)
        self.assertIn(f'name="study-token" content="{self.server.token}"', payload)

    def test_post_requires_the_session_token(self):
        status, payload = self.request('POST', '/api/chat',
                                       headers={'Content-Type': 'application/json'},
                                       body='{}')
        self.assertEqual(status, 403)
        self.assertIn('页面会话已过期', json.loads(payload)['error']['message'])

    def test_post_requires_json_content_type(self):
        status, _ = self.request('POST', '/api/chat',
                                 headers={'Content-Type': 'text/plain',
                                          'X-Study-Token': self.server.token},
                                 body='{}')
        self.assertEqual(status, 415)

    def test_post_rejects_an_endpoint_outside_the_allowlist(self):
        status, payload = self.request('POST', '/api/chat',
                                       headers={'Content-Type': 'application/json',
                                                'X-Study-Token': self.server.token},
                                       body=json.dumps({'endpoint': 'https://api.openai.com/v1',
                                                        'request': {'model': 'x', 'messages': []}}))
        self.assertEqual(status, 400)
        self.assertIn('error', json.loads(payload))

    def test_post_rejects_a_provider_endpoint_mismatch(self):
        status, _ = self.request('POST', '/api/chat',
                                 headers={'Content-Type': 'application/json',
                                          'X-Study-Token': self.server.token},
                                 body=json.dumps({'provider': 'deepseek',
                                                  'endpoint': QWEN_BASE,
                                                  'request': {'model': 'x', 'messages': []}}))
        self.assertEqual(status, 400)


if __name__ == '__main__':
    unittest.main()
