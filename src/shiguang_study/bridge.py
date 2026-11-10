#!/usr/bin/env python3
"""Local-only Qwen / DeepSeek bridge; standard library, no package installation.

The bridge serves the single-file app and proxies model calls so that API keys
can stay on the server side (DASHSCOPE_API_KEY / DEEPSEEK_API_KEY).
"""
import argparse
import hmac
import json
import os
from pathlib import Path
import secrets
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import urllib.error
import urllib.parse
import urllib.request
import webbrowser

ROOT = Path(__file__).resolve().parent


def find_app() -> Path:
    """Locate the single-file app.

    Three layouts have to work:
      * the built bundle   -> ``study-room.html`` next to this script
      * the source tree    -> ``src/app/index.html`` above this script
      * an installed venv  -> either of the above, found by walking up

    ``SHIGUANG_APP`` overrides the search with an explicit path.
    """
    override = os.environ.get('SHIGUANG_APP')
    if override:
        candidate = Path(override)
        if candidate.is_file():
            return candidate
    here = Path(__file__).resolve().parent
    for candidate in (here / 'study-room.html', here / 'index.html'):
        if candidate.is_file():
            return candidate
    for base in list(here.parents)[:6]:
        for candidate in (base / 'src' / 'app' / 'index.html',
                          base / 'dist' / 'study-room.html'):
            if candidate.is_file():
                return candidate
    return here / 'study-room.html'


APP = find_app()
MAX_REQUEST = 25 * 1024 * 1024
MAX_RESPONSE = 3 * 1024 * 1024


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def endpoint_provider(value):
    if not isinstance(value, str):
        raise ValueError('接口地址不能为空')
    u = urllib.parse.urlsplit(value)
    host = (u.hostname or '').lower()
    if (u.scheme != 'https' or u.username or u.password or u.query or u.fragment
            or u.port not in (None, 443)
            ):
        raise ValueError('仅接受服务商官方 HTTPS 接口基础地址')
    if ((host == 'aliyuncs.com' or host.endswith('.aliyuncs.com'))
            and u.path.rstrip('/').endswith('/compatible-mode/v1')):
        return 'qwen'
    if host == 'api.deepseek.com' and u.path.rstrip('/') in ('', '/v1'):
        return 'deepseek'
    raise ValueError('仅接受百炼或 DeepSeek 官方接口基础地址')


def endpoint_url(value):
    endpoint_provider(value)
    return value.rstrip('/') + '/chat/completions'


def upstream(endpoint, key, body):
    url = endpoint_url(endpoint)
    if not isinstance(key, str) or not key.strip() or len(key) > 500 or '\n' in key or '\r' in key:
        raise ValueError('请在页面填写所选服务商的 API Key，或配置对应的环境变量')
    if not isinstance(body, dict) or not isinstance(body.get('messages'), list) or not isinstance(body.get('model'), str):
        raise ValueError('请求缺少模型或消息')
    provider = endpoint_provider(endpoint)
    allowed = ('model', 'messages', 'response_format', 'max_tokens', 'stream')
    payload = {k: body[k] for k in allowed if k in body}
    # Each provider receives only its own thinking parameter.
    if provider == 'deepseek':
        payload['thinking'] = {'type': 'disabled'}
    else:
        payload['enable_thinking'] = False
    payload['stream'] = False
    req = urllib.request.Request(url, data=json.dumps(payload).encode(), headers={
        'Authorization': 'Bearer ' + key.strip(), 'Content-Type': 'application/json'
    }, method='POST')
    opener = urllib.request.build_opener(NoRedirect)
    try:
        with opener.open(req, timeout=100) as r:
            data = r.read(MAX_RESPONSE + 1)
            if len(data) > MAX_RESPONSE:
                return 502, {'error': {'message': '模型响应过大，请减少题量'}}
            return 200, json.loads(data)
    except urllib.error.HTTPError as e:
        try:
            data = json.loads(e.read(MAX_RESPONSE))
            msg = str(data.get('error', {}).get('message') or data.get('message') or '上游请求失败')
        except Exception:
            msg = '上游请求失败'
        return e.code if 400 <= e.code < 600 else 502, {'error': {'message': msg.replace(key, '[密钥已隐藏]')[:500]}}
    except (TimeoutError, OSError, ValueError):
        return 502, {'error': {'message': '无法连接所选服务商，请检查网络和接口设置后重试'}}


class StudyServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address, handler=None):
        super().__init__(address, handler or StudyHandler)
        self.token = secrets.token_urlsafe(32)
        self.api_key = os.environ.get('DASHSCOPE_API_KEY', '')
        self.deepseek_api_key = os.environ.get('DEEPSEEK_API_KEY', '')
        self.hosts = {f'127.0.0.1:{self.server_port}', f'localhost:{self.server_port}'}
        self.origins = {'http://' + h for h in self.hosts}


class StudyHandler(BaseHTTPRequestHandler):
    server_version = 'ShiguangLocal/3'

    def log_message(self, format, *args):
        # Do not log URLs, request bodies, credentials or student material.
        pass

    def guard(self, mutation=False):
        if self.headers.get('Host') not in self.server.hosts:
            self.send_json(403, {'error': {'message': '仅允许本机访问'}})
            return False
        origin = self.headers.get('Origin')
        if origin and origin not in self.server.origins:
            self.send_json(403, {'error': {'message': '请从本地工作台页面调用'}})
            return False
        if mutation and not hmac.compare_digest(self.headers.get('X-Study-Token', ''), self.server.token):
            self.send_json(403, {'error': {'message': '页面会话已过期，请刷新本地工作台'}})
            return False
        return True

    def send_data(self, status, body, mime):
        self.send_response(status)
        self.send_header('Content-Type', mime)
        self.send_header('Content-Length', str(len(body)))
        self.send_header('Cache-Control', 'no-store')
        self.send_header('X-Content-Type-Options', 'nosniff')
        self.send_header('Referrer-Policy', 'no-referrer')
        self.send_header('Cross-Origin-Resource-Policy', 'same-origin')
        self.end_headers()
        try:
            self.wfile.write(body)
        except (BrokenPipeError, ConnectionResetError):
            pass

    def send_json(self, status, obj):
        self.send_data(status, json.dumps(obj, ensure_ascii=False).encode(), 'application/json; charset=utf-8')

    def do_GET(self):
        if not self.guard():
            return
        path = urllib.parse.urlsplit(self.path).path
        if path == '/api/status':
            self.send_json(200, {'app': 'shiguang-study', 'version': 3,
                                'hasKey': bool(self.server.api_key),
                                'providers': {'qwen': {'hasKey': bool(self.server.api_key)},
                                              'deepseek': {'hasKey': bool(self.server.deepseek_api_key)}}})
        elif path in ('/', '/index.html', '/' + urllib.parse.quote(APP.name)):
            try:
                html = APP.read_text().replace('</head>', '<meta name="study-token" content="' + self.server.token + '"></head>')
                self.send_data(200, html.encode(), 'text/html; charset=utf-8')
            except OSError:
                self.send_json(404, {'error': {'message': '未找到应用页面，请确认启动脚本与 index.html（或 study-room.html）在同一目录'}})
        else:
            self.send_json(404, {'error': {'message': '页面不存在'}})

    def do_POST(self):
        if not self.guard(mutation=True):
            return
        if self.path != '/api/chat':
            self.send_json(404, {'error': {'message': '接口不存在'}})
            return
        if self.headers.get('Content-Type', '').split(';')[0].strip() != 'application/json':
            self.send_json(415, {'error': {'message': '请使用 JSON 请求'}})
            return
        try:
            length = int(self.headers.get('Content-Length', '0'))
            if length < 1 or length > MAX_REQUEST:
                self.send_json(413, {'error': {'message': '请求内容过大或为空'}})
                return
            self.connection.settimeout(30)
            data = json.loads(self.rfile.read(length))
            if not isinstance(data, dict):
                raise ValueError('请求格式不正确')
            provider = endpoint_provider(data.get('endpoint'))
            if data.get('provider', provider) != provider:
                raise ValueError('服务商与接口地址不一致')
            # Never fall back to the other provider's credential.
            key = data.get('apiKey') or (self.server.deepseek_api_key if provider == 'deepseek' else self.server.api_key)
            if not key:
                raise ValueError('尚未配置所选服务商的密钥')
            status, result = upstream(data.get('endpoint'), key, data.get('request'))
            self.send_json(status, result)
        except (ValueError, TypeError, TimeoutError):
            self.send_json(400, {'error': {'message': '请求格式或接口设置不正确，请检查配置'}})
        except Exception:
            self.send_json(500, {'error': {'message': '本地服务处理失败，请重启后再试'}})


def main():
    parser = argparse.ArgumentParser(description='Shiguang Study Room · local AI bridge')
    parser.add_argument('--port', type=int, default=4312)
    parser.add_argument('--no-open', action='store_true')
    args = parser.parse_args()
    url = f'http://127.0.0.1:{args.port}/'
    try:
        server = StudyServer(('127.0.0.1', args.port))
    except OSError:
        try:
            with urllib.request.urlopen(url + 'api/status', timeout=2) as r:
                if json.load(r).get('app') != 'shiguang-study':
                    raise ValueError()
            print('The workbench is already running: ' + url)
            if not args.no_open:
                webbrowser.open(url)
            return
        except Exception:
            print(f'Port {args.port} is taken by another program. Close it, or pass --port <n> to use a different port.')
            raise SystemExit(1)
    print('Study Room workbench started: ' + url)
    print('Keep this window open. Press Ctrl+C to stop. Set the API key in the app\'s AI settings, or export DASHSCOPE_API_KEY / DEEPSEEK_API_KEY.')
    if not args.no_open:
        webbrowser.open(url)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print('\nWorkbench server stopped.')
    finally:
        server.server_close()


if __name__ == '__main__':
    main()
