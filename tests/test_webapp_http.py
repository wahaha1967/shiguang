"""HTTP-level checks for the FastAPI backend (no network)."""
import time
from unittest import mock

import pytest
from fastapi.testclient import TestClient

from shiguang_study import keys
from shiguang_study.webapp import config, db, ops
from shiguang_study.webapp.app import create_app

TOKEN = 'test-token'
QWEN_BASE = 'https://dashscope.aliyuncs.com/compatible-mode/v1'
HEADERS = {'Authorization': f'Bearer {TOKEN}'}


@pytest.fixture()
def data_dir(tmp_path, monkeypatch):
    """An isolated data directory + fresh engine, caches and keys per test."""
    monkeypatch.setenv('SHIGUANG_DATA_DIR', str(tmp_path))
    monkeypatch.setenv('SHIGUANG_TOKEN', TOKEN)
    monkeypatch.setenv('SHIGUANG_ENV_FILE', str(tmp_path / '.env'))
    monkeypatch.setenv(keys.SECRET_ENV, 'http-test-secret')
    keys.reset()
    config.token.cache_clear()
    config.data_dir.cache_clear()
    db.reset_engine()
    yield tmp_path
    db.reset_engine()
    keys.reset()


@pytest.fixture()
def client(data_dir):
    return TestClient(create_app())


def store_key(client, provider: str = 'qwen', api_key: str = 'sk-STORED0123456789') -> str:
    response = client.post('/api/ai-key', headers=HEADERS,
                           json={'provider': provider, 'apiKey': api_key})
    assert response.status_code == 200, response.text
    return api_key


def question() -> dict:
    return {'id': 'q-1', 'subject': '数学', 'title': '1+1', 'answer': '2',
            'reason': '', 'tag': '', 'cause': '概念不清', 'difficulty': '基础',
            'image': '', 'favorite': False, 'stage': 0,
            'due': '2026-11-10', 'created': '2026-11-10', 'demo': False}


def test_state_requires_a_token(client):
    assert client.get('/api/state').status_code == 401
    assert client.get('/api/state', headers={'Authorization': 'Bearer wrong'}).status_code == 403


def test_ops_round_trip(client):
    response = client.post(
        '/api/ops',
        headers=HEADERS,
        json={'ops': [{'t': 'questions', 'op': 'upsert', 'ord': 0, 'row': question()}]},
    )
    assert response.status_code == 200, response.text
    state = client.get('/api/state', headers=HEADERS).json()
    assert state['questions'][0]['title'] == '1+1'


def test_timer_accepts_epoch_milliseconds(client):
    end = int(time.time() * 1000) + 60_000
    response = client.post(
        '/api/ops',
        headers=HEADERS,
        json={'ops': [{'t': 'timer', 'op': 'replace',
                       'row': {'running': True, 'remaining': 60,
                               'duration': 60, 'end': end}}]},
    )
    assert response.status_code == 200, response.text
    timer = client.get('/api/state', headers=HEADERS).json()['timer']
    assert timer['end'] == end


def test_import_replaces_the_owner_state(client):
    state = {'schema': 1, 'questions': [question()], 'trash': [], 'logs': [],
             'rewards': [], 'redemptions': [],
             'prefs': {'name': '同学', 'goal': 5, 'minutes': 25, 'theme': 'light'},
             'timer': None, 'aiSets': []}
    response = client.post('/api/import', headers=HEADERS, json={'state': state})
    assert response.status_code == 200, response.text
    state = client.get('/api/state', headers=HEADERS).json()
    assert len(state['questions']) == 1


def test_new_owner_is_not_initialized(client):
    state = client.get('/api/state', headers=HEADERS).json()
    assert not state['initialized']


def test_ai_config_round_trip(client):
    ai_config = {'provider': 'deepseek',
                 'base': 'https://dashscope.aliyuncs.com/compatible-mode/v1',
                 'vision': 'qwen3-vl-plus', 'text': 'qwen-plus',
                 'deepseekBase': 'https://api.deepseek.com',
                 'deepseekVision': 'deepseek-flash',
                 'deepseekText': 'deepseek-flash'}
    response = client.post(
        '/api/ops',
        headers=HEADERS,
        json={'ops': [{'t': 'aiConfig', 'op': 'replace', 'row': ai_config}]},
    )
    assert response.status_code == 200, response.text
    state = client.get('/api/state', headers=HEADERS).json()
    assert state['aiConfig']['provider'] == 'deepseek'
    assert state['aiConfig']['deepseekText'] == 'deepseek-flash'
    assert 'transport' not in state['aiConfig']
    assert state['initialized']


def test_saving_a_key_stores_it_encrypted(client, data_dir):
    plain = store_key(client, 'qwen', 'sk-SECRET0123456789')
    blobs = b''.join(path.read_bytes() for path in data_dir.glob('study.db*'))
    assert plain.encode() not in blobs, 'the plain key must not reach the database'
    with db.session_factory()() as session:
        ciphertext = ops.read_key(session, config.owner_id(TOKEN), 'qwen')
    assert ciphertext
    assert keys.decrypt(ciphertext) == plain


def test_the_status_never_returns_key_material(client):
    store_key(client, 'qwen', 'sk-SECRET0123456789')
    response = client.get('/api/status', headers=HEADERS)
    assert response.status_code == 200
    assert 'sk-SECRET0123456789' not in response.text
    status = response.json()
    assert status['providers']['qwen']['hasKey']
    assert not status['providers']['deepseek']['hasKey']


def test_the_status_returns_a_masked_preview(client):
    store_key(client, 'qwen', 'sk-SECRET0123456789')
    preview = client.get('/api/status', headers=HEADERS).json()['providers']['qwen']['preview']
    assert preview == 'sk-S****6789'
    assert 'SECRET0123' not in preview


def test_a_short_key_is_masked_completely(client):
    store_key(client, 'qwen', 'sk-short')
    preview = client.get('/api/status', headers=HEADERS).json()['providers']['qwen']['preview']
    assert preview == '****'


def test_saving_a_key_returns_its_masked_preview(client):
    response = client.post('/api/ai-key', headers=HEADERS,
                           json={'provider': 'deepseek', 'apiKey': 'sk-OTHER0123456789'})
    assert response.status_code == 200, response.text
    assert response.json()['preview'] == 'sk-O****6789'
    assert 'OTHER0123' not in response.text


def test_an_unknown_provider_is_rejected(client):
    response = client.post('/api/ai-key', headers=HEADERS,
                           json={'provider': 'openai', 'apiKey': 'sk-x'})
    assert response.status_code == 400


def test_an_empty_key_is_rejected(client):
    response = client.post('/api/ai-key', headers=HEADERS,
                           json={'provider': 'qwen', 'apiKey': '  '})
    assert response.status_code == 400


def test_clearing_a_key_removes_it(client):
    store_key(client, 'qwen')
    response = client.delete('/api/ai-key/qwen', headers=HEADERS)
    assert response.status_code == 200, response.text
    status = client.get('/api/status', headers=HEADERS).json()
    assert not status['providers']['qwen']['hasKey']


def test_chat_rejects_an_endpoint_outside_the_allowlist(client):
    response = client.post(
        '/api/chat',
        headers=HEADERS,
        json={'provider': 'qwen', 'endpoint': 'https://api.openai.com/v1',
              'request': {'model': 'x', 'messages': []}},
    )
    assert response.status_code == 400


def test_chat_requires_a_stored_key(client):
    response = client.post(
        '/api/chat',
        headers=HEADERS,
        json={'provider': 'qwen', 'endpoint': QWEN_BASE,
              'request': {'model': 'qwen-plus', 'messages': []}},
    )
    assert response.status_code == 400
    assert 'API Key' in response.json()['detail']


def test_the_browser_cannot_inject_a_key_per_request(client):
    with mock.patch('shiguang_study.webapp.app.llm.upstream') as upstream:
        response = client.post(
            '/api/chat',
            headers=HEADERS,
            json={'provider': 'qwen', 'endpoint': QWEN_BASE,
                  'apiKey': 'sk-injected',
                  'request': {'model': 'qwen-plus', 'messages': []}},
        )
    assert response.status_code == 400
    upstream.assert_not_called()


def test_chat_forwards_the_stored_key(client):
    plain = store_key(client, 'qwen', 'sk-stored-key')
    with mock.patch(
        'shiguang_study.webapp.app.llm.upstream',
        return_value=(200, {'choices': [{'message': {'content': '{}'}}]}),
    ) as upstream:
        response = client.post(
            '/api/chat',
            headers=HEADERS,
            json={'provider': 'qwen', 'endpoint': QWEN_BASE,
                  'request': {'model': 'qwen-plus', 'messages': []}},
        )
    assert response.status_code == 200
    upstream.assert_called_once()
    assert upstream.call_args.args[1] == plain


def test_index_injects_the_api_token(client):
    response = client.get('/')
    assert response.status_code == 200
    assert 'meta name="api-token"' in response.text
