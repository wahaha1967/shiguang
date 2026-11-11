"""The FastAPI backend.

Routes:

    GET  /              the single-file app, with the access token injected
    GET  /api/health    liveness and database check
    GET  /api/status    which providers have a key stored (never the key)
    GET  /api/state     the whole state for the calling owner
    POST /api/ops       apply a batch of changes (the app's write path)
    POST /api/import    replace the owner's state with an exported backup
    POST /api/ai-key    store an encrypted provider key
    DELETE /api/ai-key/{provider}  remove a stored provider key
    POST /api/chat      forward a chat completion to Qwen or DeepSeek

Every ``/api/*`` route except ``/api/health`` requires
``Authorization: Bearer <token>``. Each distinct token owns a separate data
space, so two tokens can never read or overwrite each other's data.
"""
from __future__ import annotations

import hmac

from fastapi import Depends, FastAPI, Header, HTTPException
from fastapi.responses import HTMLResponse, JSONResponse
from sqlalchemy.orm import Session

from .. import keys, llm
from . import config, db, ops

TOKEN_META = '<meta name="api-token" content="%s">'
PROVIDERS = ('qwen', 'deepseek')


def create_app() -> FastAPI:
    keys.load_env_file()
    application = FastAPI(title='Shiguang Study Room', version='3.0.0',
                          docs_url='/api/docs', redoc_url=None)

    def get_session():
        session = db.session_factory()()
        try:
            yield session
            session.commit()
        except Exception:
            session.rollback()
            raise
        finally:
            session.close()

    def require_owner(authorization: str | None = Header(default=None)) -> str:
        expected = config.token()
        if not authorization or not authorization.startswith('Bearer '):
            raise HTTPException(status_code=401, detail='需要访问令牌，请在页面的 AI 设置中填写')
        supplied = authorization[len('Bearer '):].strip()
        if not hmac.compare_digest(supplied, expected):
            raise HTTPException(status_code=403, detail='访问令牌无效')
        return config.owner_id(supplied)

    @application.get('/api/health')
    def health():
        return {'app': 'shiguang-study', 'version': 3, 'database': db.database_exists()}

    @application.get('/api/status')
    def status(owner: str = Depends(require_owner),
               session: Session = Depends(get_session)):
        stored = ops.read_keys(session, owner)
        providers = {}
        for name in PROVIDERS:
            ciphertext = stored.get(name, '')
            preview = ''
            if ciphertext:
                try:
                    preview = keys.preview(keys.decrypt(ciphertext))
                except ValueError:
                    preview = ''
            providers[name] = {'hasKey': bool(ciphertext), 'preview': preview}
        return {'app': 'shiguang-study', 'version': 3,
                'hasKey': any(item['hasKey'] for item in providers.values()),
                'providers': providers}

    @application.get('/api/state')
    def get_state(owner: str = Depends(require_owner),
                  session: Session = Depends(get_session)):
        return ops.read_state(session, owner)

    @application.post('/api/ops')
    def post_ops(payload: dict, owner: str = Depends(require_owner),
                 session: Session = Depends(get_session)):
        try:
            applied = ops.apply_ops(session, owner, payload.get('ops') or [])
        except ops.InvalidOperation as error:
            raise HTTPException(status_code=400, detail=str(error)) from error
        return {'ok': True, 'applied': applied}

    @application.post('/api/import')
    def post_import(payload: dict, owner: str = Depends(require_owner),
                    session: Session = Depends(get_session)):
        state = payload.get('state')
        if not isinstance(state, dict) or not isinstance(state.get('questions'), list):
            raise HTTPException(status_code=400, detail='备份格式不正确')
        try:
            ops.replace_state(session, owner, state)
        except ops.InvalidOperation as error:
            raise HTTPException(status_code=400, detail=str(error)) from error
        return {'ok': True}

    @application.post('/api/ai-key')
    def post_ai_key(payload: dict, owner: str = Depends(require_owner),
                    session: Session = Depends(get_session)):
        provider = payload.get('provider')
        if provider not in PROVIDERS:
            raise HTTPException(status_code=400, detail='服务商只能是 qwen 或 deepseek')
        api_key = payload.get('apiKey')
        if (not isinstance(api_key, str) or not api_key.strip()
                or len(api_key) > 500 or '\n' in api_key or '\r' in api_key):
            raise HTTPException(status_code=400, detail='请填写有效的 API Key')
        try:
            ciphertext = keys.encrypt(api_key.strip())
        except Exception as error:  # pragma: no cover - only a broken secret
            raise HTTPException(status_code=500, detail='密钥加密失败，请检查 .env 配置') from error
        ops.set_key(session, owner, provider, ciphertext)
        return {'ok': True, 'provider': provider, 'hasKey': True,
                'preview': keys.preview(api_key.strip())}

    @application.delete('/api/ai-key/{provider}')
    def delete_ai_key(provider: str, owner: str = Depends(require_owner),
                      session: Session = Depends(get_session)):
        if provider not in PROVIDERS:
            raise HTTPException(status_code=400, detail='服务商只能是 qwen 或 deepseek')
        ops.delete_key(session, owner, provider)
        return {'ok': True, 'provider': provider, 'hasKey': False}

    @application.post('/api/chat')
    def post_chat(payload: dict, owner: str = Depends(require_owner),
                  session: Session = Depends(get_session)):
        try:
            provider = llm.endpoint_provider(payload.get('endpoint'))
        except ValueError as error:
            raise HTTPException(status_code=400, detail=str(error)) from error
        if payload.get('provider', provider) != provider:
            raise HTTPException(status_code=400, detail='服务商与接口地址不一致')
        ciphertext = ops.read_key(session, owner, provider)
        if not ciphertext:
            raise HTTPException(status_code=400, detail='尚未保存所选服务商的 API Key，请在 AI 连接设置中保存')
        try:
            key = keys.decrypt(ciphertext)
        except ValueError as error:
            raise HTTPException(status_code=500, detail=str(error)) from error
        status_code, result = llm.upstream(payload.get('endpoint'), key, payload.get('request'))
        return JSONResponse(result, status_code=status_code)

    @application.get('/', response_class=HTMLResponse)
    @application.get('/index.html', response_class=HTMLResponse)
    def index():
        try:
            html = config.app_path().read_text(encoding='utf-8')
        except OSError as error:
            raise HTTPException(status_code=404, detail='未找到应用页面') from error
        return HTMLResponse(html.replace('</head>', TOKEN_META % config.token() + '</head>'))

    return application


app = create_app()
