"""Qwen / DeepSeek proxy logic.

Pure standard library, no web framework: the FastAPI layer in
:mod:`shiguang_study.webapp` calls into this module, and the unit tests exercise
it directly with a fake upstream.

The module owns two responsibilities:
  * decide which provider an endpoint belongs to (endpoint allowlist)
  * forward a chat completion request without ever leaking the API key

The key itself is not read here: the backend decrypts the stored key and
hands it over for the single request.
"""
import json
import urllib.error
import urllib.parse
import urllib.request

MAX_REQUEST = 25 * 1024 * 1024
MAX_RESPONSE = 3 * 1024 * 1024


class NoRedirect(urllib.request.HTTPRedirectHandler):
    """Reject redirects so that a key can never be replayed to another host."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def endpoint_provider(value):
    """Return ``'qwen'`` or ``'deepseek'`` for an official base URL.

    Anything else - plain http, a custom port, embedded credentials, a query
    string, an unknown host - is rejected.
    """
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
    """Forward one chat completion request.

    Returns ``(status, payload)``. The key never appears in the payload: it is
    only used for the Authorization header, and it is redacted from upstream
    error messages.
    """
    url = endpoint_url(endpoint)
    if not isinstance(key, str) or not key.strip() or len(key) > 500 or '\n' in key or '\r' in key:
        raise ValueError('服务商密钥无效，请重新保存 API Key')
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
