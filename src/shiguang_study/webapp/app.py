"""The FastAPI backend.

Routes:

    GET  /              the single-file app, with the access token injected
    GET  /api/health    liveness and database check
    GET  /api/state     the whole state for the calling owner
    POST /api/ops       apply a batch of changes (the app's write path)
    POST /api/import    replace the owner's state with an exported backup
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

from .. import llm
from . import config, db, ops

TOKEN_META = '<meta name="api-token" content="%s">'


def create_app() -> FastAPI:
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
    def status(owner: str = Depends(require_owner)):
        return {'app': 'shiguang-study', 'version': 3,
                'hasKey': bool(llm.server_key('qwen')),
                'providers': {'qwen': {'hasKey': bool(llm.server_key('qwen'))},
                              'deepseek': {'hasKey': bool(llm.server_key('deepseek'))}}}

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

    @application.post('/api/chat')
    def post_chat(payload: dict, owner: str = Depends(require_owner)):
        try:
            provider = llm.endpoint_provider(payload.get('endpoint'))
        except ValueError as error:
            raise HTTPException(status_code=400, detail=str(error)) from error
        if payload.get('provider', provider) != provider:
            raise HTTPException(status_code=400, detail='服务商与接口地址不一致')
        key = payload.get('apiKey') or llm.server_key(provider)
        if not key:
            raise HTTPException(status_code=400, detail='尚未配置所选服务商的密钥')
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
