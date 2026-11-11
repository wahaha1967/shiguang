"""Reading the state out of the database and writing changes back in.

The wire format is deliberately close to the app's own state object:

    GET  /api/state  -> the state dictionary, exactly as the app expects it
    POST /api/ops    -> {"ops": [...]} applied in order inside one transaction

An operation looks like::

    {"t": "questions", "op": "upsert", "ord": 3, "row": {...}, "trashed": false}
    {"t": "questions", "op": "delete", "id": "q-1"}
    {"t": "prefs",    "op": "replace", "row": {...}}
    {"t": "timer",    "op": "replace", "row": null}
    {"t": "aiSets",   "op": "upsert", "row": {...}}
    {"t": "reset",    "op": "reset"}

The client never sends a row for another owner: every statement is filtered by
``owner_id``, and incoming values are validated here as well - the browser is
not trusted.
"""
from __future__ import annotations

import re
from datetime import datetime, timezone
from typing import Any

from sqlalchemy import delete, select
from sqlalchemy.orm import Session

from .db import (
    AiConfig,
    AiKey,
    AiSet,
    AiSetQuestion,
    Log,
    Prefs,
    Question,
    Redemption,
    Reward,
    Timer,
)

ID_RE = re.compile(r'^[A-Za-z0-9_-]{1,100}$')
DATE_RE = re.compile(r'^\d{4}-\d{2}-\d{2}$')
QUESTION_TEXT = ('subject', 'title', 'answer', 'reason', 'tag', 'cause', 'difficulty')
MAX_TEXT = 10_000
MAX_IMAGE = 2_200_000
LIST_TABLES = ('questions', 'logs', 'rewards', 'redemptions', 'aiSets')


class InvalidOperation(ValueError):
    """Raised when an operation does not pass validation."""


def _text(value: Any, field: str, limit: int = MAX_TEXT) -> str:
    if not isinstance(value, str):
        raise InvalidOperation(f'{field} 必须是文本')
    if len(value) > limit:
        raise InvalidOperation(f'{field} 超过长度上限 {limit}')
    return value


def _ident(value: Any, field: str = 'id') -> str:
    if not isinstance(value, str) or not ID_RE.match(value):
        raise InvalidOperation(f'{field} 格式不正确')
    return value


def _date(value: Any, field: str) -> str:
    if not isinstance(value, str) or not DATE_RE.match(value):
        raise InvalidOperation(f'{field} 必须是 YYYY-MM-DD')
    return value


def _bool(value: Any, field: str) -> bool:
    if not isinstance(value, bool):
        raise InvalidOperation(f'{field} 必须是布尔值')
    return value


def _int(value: Any, field: str, low: int, high: int) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or not low <= value <= high:
        raise InvalidOperation(f'{field} 必须是 {low} 到 {high} 之间的整数')
    return value


def _number(value: Any, field: str, low: float, high: float) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not low <= value <= high:
        raise InvalidOperation(f'{field} 超出允许范围')
    return float(value)


# --------------------------------------------------------------------------- read


def read_state(session: Session, owner: str) -> dict:
    """Assemble the state dictionary for one owner."""
    questions = session.scalars(
        select(Question).where(Question.owner_id == owner).order_by(Question.ord)).all()
    logs = session.scalars(
        select(Log).where(Log.owner_id == owner).order_by(Log.ord)).all()
    rewards = session.scalars(
        select(Reward).where(Reward.owner_id == owner).order_by(Reward.ord)).all()
    redemptions = session.scalars(
        select(Redemption).where(Redemption.owner_id == owner).order_by(Redemption.ord)).all()
    sets = session.scalars(
        select(AiSet).where(AiSet.owner_id == owner).order_by(AiSet.ord)).all()

    prefs = session.get(Prefs, owner)
    timer = session.get(Timer, owner)
    ai_cfg = session.get(AiConfig, owner)

    state: dict[str, Any] = {
        'schema': 1,
        'questions': [],
        'trash': [],
        'logs': [],
        'rewards': [],
        'redemptions': [],
        'prefs': {'name': '同学', 'goal': 5, 'minutes': 25, 'theme': 'light'},
        'timer': None,
        'aiSets': [],
    }
    for row in questions:
        item = {
            'id': row.id,
            'subject': row.subject,
            'title': row.title,
            'answer': row.answer,
            'reason': row.reason,
            'tag': row.tag,
            'cause': row.cause,
            'difficulty': row.difficulty,
            'image': row.image,
            'favorite': row.favorite,
            'stage': row.stage,
            'due': row.due,
            'created': row.created,
            'demo': row.demo,
        }
        (state['trash'] if row.trashed else state['questions']).append(item)

    for row in logs:
        item = {
            'id': row.id,
            'date': row.date,
            'at': row.at,
            'type': row.type,
            'label': row.label,
            'points': row.points,
        }
        if row.qid is not None:
            item['qid'] = row.qid
        if row.grade is not None:
            item['grade'] = row.grade
        if row.minutes is not None:
            item['minutes'] = row.minutes
        state['logs'].append(item)

    for row in rewards:
        state['rewards'].append({'id': row.id, 'name': row.name, 'cost': row.cost})
    for row in redemptions:
        state['redemptions'].append({'id': row.id, 'name': row.name, 'cost': row.cost, 'date': row.date})

    if prefs is not None:
        state['prefs'] = {'name': prefs.name, 'goal': prefs.goal,
                          'minutes': prefs.minutes, 'theme': prefs.theme}
    if timer is not None:
        item = {'running': timer.running, 'remaining': timer.remaining, 'duration': timer.duration}
        if timer.end is not None:
            item['end'] = timer.end
        state['timer'] = item
    if ai_cfg is not None:
        state['aiConfig'] = {
            'provider': ai_cfg.provider,
            'base': ai_cfg.base,
            'vision': ai_cfg.vision,
            'text': ai_cfg.text,
            'deepseekBase': ai_cfg.deepseek_base,
            'deepseekVision': ai_cfg.deepseek_vision,
            'deepseekText': ai_cfg.deepseek_text,
        }

    for row in sets:
        items = session.scalars(
            select(AiSetQuestion).where(AiSetQuestion.set_id == row.id)
            .order_by(AiSetQuestion.ord)).all()
        state['aiSets'].append({
            'id': row.id,
            'aiReviewed': row.ai_reviewed,
            'sourceId': row.source_id,
            'sourceTitle': row.source_title,
            'subject': row.subject,
            'created': row.created,
            'questions': [{
                'id': q.id,
                'title': q.title,
                'answer': q.answer,
                'explanation': q.explanation,
                'knowledge': q.knowledge,
                'note': q.note,
                'revealed': q.revealed,
                'grade': q.grade,
            } for q in items],
        })
    state['initialized'] = (prefs is not None or ai_cfg is not None
                            or bool(questions or logs or rewards or redemptions or sets or timer))
    return state


# --------------------------------------------------------------------------- write


def _upsert_question(session: Session, owner: str, op: dict) -> None:
    row = op.get('row') or {}
    ident = _ident(op.get('id') or row.get('id'))
    values = {field: _text(row.get(field, ''), field) for field in QUESTION_TEXT}
    values['image'] = _text(row.get('image', ''), 'image', MAX_IMAGE)
    values['favorite'] = _bool(row.get('favorite', False), 'favorite')
    values['demo'] = _bool(row.get('demo', False), 'demo')
    values['stage'] = _int(row.get('stage', 0), 'stage', 0, 5)
    values['due'] = _date(row.get('due', '1970-01-01'), 'due')
    values['created'] = _date(row.get('created', '1970-01-01'), 'created')
    values['trashed'] = _bool(op.get('trashed', False), 'trashed')
    values['ord'] = _int(op.get('ord', 0), 'ord', 0, 1_000_000)

    existing = session.get(Question, ident)
    if existing is not None and existing.owner_id != owner:
        raise InvalidOperation('记录编号已被使用')
    if existing is None:
        existing = Question(id=ident, owner_id=owner)
        session.add(existing)
    for field, value in values.items():
        setattr(existing, field, value)


def _upsert_log(session: Session, owner: str, op: dict) -> None:
    row = op.get('row') or {}
    ident = _ident(row.get('id'))
    values = {
        'date': _date(row.get('date'), 'date'),
        'at': _text(row.get('at', ''), 'at', 40),
        'type': _text(row.get('type', ''), 'type', 20),
        'label': _text(row.get('label', ''), 'label'),
        'points': _number(row.get('points', 0), 'points', 0, 30),
        'qid': row.get('qid'),
        'grade': row.get('grade'),
        'minutes': row.get('minutes'),
        'ord': _int(op.get('ord', 0), 'ord', 0, 1_000_000),
    }
    if values['type'] not in ('add', 'review', 'practice', 'focus'):
        raise InvalidOperation('type 只能是 add/review/practice/focus')
    if values['type'] in ('review', 'practice') and values['grade'] not in ('again', 'hard', 'good'):
        raise InvalidOperation('复习记录必须带有自评结果')
    if values['minutes'] is not None:
        values['minutes'] = _number(values['minutes'], 'minutes', 0, 24 * 60)

    existing = session.get(Log, ident)
    if existing is not None and existing.owner_id != owner:
        raise InvalidOperation('记录编号已被使用')
    if existing is None:
        existing = Log(id=ident, owner_id=owner)
        session.add(existing)
    for field, value in values.items():
        setattr(existing, field, value)


def _upsert_named(session: Session, owner: str, op: dict, model, date_field: bool = False) -> None:
    row = op.get('row') or {}
    ident = _ident(row.get('id'))
    name = _text(row.get('name', ''), 'name')
    cost = _int(row.get('cost', 0), 'cost', 0, 10_000)
    values = {'name': name, 'cost': cost,
              'ord': _int(op.get('ord', 0), 'ord', 0, 1_000_000)}
    if date_field:
        values['date'] = _date(row.get('date'), 'date')
    existing = session.get(model, ident)
    if existing is not None and existing.owner_id != owner:
        raise InvalidOperation('记录编号已被使用')
    if existing is None:
        existing = model(id=ident, owner_id=owner)
        session.add(existing)
    for field, value in values.items():
        setattr(existing, field, value)


def _upsert_ai_set(session: Session, owner: str, op: dict) -> None:
    row = op.get('row') or {}
    ident = _ident(row.get('id'))
    questions = row.get('questions')
    if not isinstance(questions, list) or len(questions) != 3:
        raise InvalidOperation('AI 练习组必须恰好包含三道题')
    existing = session.get(AiSet, ident)
    if existing is not None and existing.owner_id != owner:
        raise InvalidOperation('记录编号已被使用')
    if existing is None:
        existing = AiSet(id=ident, owner_id=owner)
        session.add(existing)
    existing.source_id = _text(row.get('sourceId', ''), 'sourceId', 100)
    existing.source_title = _text(row.get('sourceTitle', ''), 'sourceTitle')
    existing.subject = _text(row.get('subject', ''), 'subject')
    existing.created = _date(row.get('created'), 'created')
    existing.ai_reviewed = _bool(row.get('aiReviewed', False), 'aiReviewed')
    existing.ord = _int(op.get('ord', 0), 'ord', 0, 1_000_000)

    session.execute(delete(AiSetQuestion).where(AiSetQuestion.set_id == ident))
    for index, item in enumerate(questions):
        if not isinstance(item, dict):
            raise InvalidOperation('练习题格式不正确')
        session.add(AiSetQuestion(
            id=_ident(item.get('id')),
            set_id=ident,
            ord=index,
            title=_text(item.get('title', ''), 'title'),
            answer=_text(item.get('answer', ''), 'answer'),
            explanation=_text(item.get('explanation', ''), 'explanation'),
            knowledge=_text(item.get('knowledge', ''), 'knowledge', 200),
            note=_text(item.get('note', ''), 'note', 5000),
            revealed=_bool(item.get('revealed', False), 'revealed'),
            grade=_text(item.get('grade', ''), 'grade', 20),
        ))


def _replace_prefs(session: Session, owner: str, op: dict) -> None:
    row = op.get('row') or {}
    values = {
        'name': _text(row.get('name', ''), 'name', 20),
        'goal': _int(row.get('goal', 5), 'goal', 1, 100),
        'minutes': _int(row.get('minutes', 25), 'minutes', 1, 120),
        'theme': _text(row.get('theme', 'light'), 'theme', 20),
    }
    if values['theme'] not in ('light', 'night'):
        raise InvalidOperation('theme 只能是 light 或 night')
    prefs = session.get(Prefs, owner)
    if prefs is None:
        prefs = Prefs(owner_id=owner)
        session.add(prefs)
    for field, value in values.items():
        setattr(prefs, field, value)


def _replace_timer(session: Session, owner: str, op: dict) -> None:
    row = op.get('row')
    timer = session.get(Timer, owner)
    if row is None:
        if timer is not None:
            session.delete(timer)
        return
    if not isinstance(row, dict):
        raise InvalidOperation('计时数据格式不正确')
    running = _bool(row.get('running', False), 'running')
    duration = _number(row.get('duration', 0), 'duration', 0, 24 * 3600)
    remaining = _number(row.get('remaining', 0), 'remaining', 0, 24 * 3600)
    end = row.get('end')
    if end is not None:
        # The browser sends an absolute epoch timestamp in milliseconds.
        end = _number(end, 'end', 0, 4_102_444_800_000)
    if remaining > duration:
        raise InvalidOperation('剩余时间不能超过总时长')
    if timer is None:
        timer = Timer(owner_id=owner)
        session.add(timer)
    timer.running = running
    timer.duration = duration
    timer.remaining = remaining
    timer.end = end


def _replace_ai_config(session: Session, owner: str, op: dict) -> None:
    row = op.get('row')
    if row is None:
        cfg = session.get(AiConfig, owner)
        if cfg is not None:
            session.delete(cfg)
        return
    if not isinstance(row, dict):
        raise InvalidOperation('AI 配置格式不正确')
    provider = _text(row.get('provider', 'qwen'), 'provider', 20)
    if provider not in ('qwen', 'deepseek'):
        raise InvalidOperation('provider 只能是 qwen 或 deepseek')
    values = {
        'provider': provider,
        'base': _text(row.get('base', ''), 'base', 500),
        'vision': _text(row.get('vision', ''), 'vision', 120),
        'text': _text(row.get('text', ''), 'text', 120),
        'deepseek_base': _text(row.get('deepseekBase', ''), 'deepseekBase', 500),
        'deepseek_vision': _text(row.get('deepseekVision', ''), 'deepseekVision', 120),
        'deepseek_text': _text(row.get('deepseekText', ''), 'deepseekText', 120),
    }
    cfg = session.get(AiConfig, owner)
    if cfg is None:
        cfg = AiConfig(owner_id=owner)
        session.add(cfg)
    for field, value in values.items():
        setattr(cfg, field, value)


def set_key(session: Session, owner: str, provider: str, ciphertext: str) -> None:
    """Store one encrypted provider key."""
    row = session.get(AiKey, (owner, provider))
    if row is None:
        row = AiKey(owner_id=owner, provider=provider)
        session.add(row)
    row.ciphertext = ciphertext
    row.updated = datetime.now(timezone.utc).isoformat(timespec='seconds')


def read_key(session: Session, owner: str, provider: str) -> str:
    """The encrypted key of one provider, or an empty string."""
    row = session.get(AiKey, (owner, provider))
    return row.ciphertext if row is not None else ''


def delete_key(session: Session, owner: str, provider: str) -> bool:
    """Remove one provider key. Returns whether a key was there."""
    row = session.get(AiKey, (owner, provider))
    if row is None:
        return False
    session.delete(row)
    return True


def read_keys(session: Session, owner: str) -> dict[str, str]:
    """The encrypted keys of one owner, keyed by provider."""
    rows = session.scalars(select(AiKey).where(AiKey.owner_id == owner)).all()
    return {row.provider: row.ciphertext for row in rows}


def clear_owner(session: Session, owner: str) -> None:
    """Remove every row belonging to one owner.

    The encrypted API keys stay: they are service credentials, not learning
    data, so a reset or a restored backup must not silently break the AI
    features.
    """
    set_ids = session.scalars(select(AiSet.id).where(AiSet.owner_id == owner)).all()
    if set_ids:
        session.execute(delete(AiSetQuestion).where(AiSetQuestion.set_id.in_(set_ids)))
    session.execute(delete(AiSet).where(AiSet.owner_id == owner))
    session.execute(delete(Log).where(Log.owner_id == owner))
    session.execute(delete(Reward).where(Reward.owner_id == owner))
    session.execute(delete(Redemption).where(Redemption.owner_id == owner))
    session.execute(delete(Question).where(Question.owner_id == owner))
    session.execute(delete(Prefs).where(Prefs.owner_id == owner))
    session.execute(delete(Timer).where(Timer.owner_id == owner))
    session.execute(delete(AiConfig).where(AiConfig.owner_id == owner))


def apply_ops(session: Session, owner: str, ops: list) -> int:
    """Apply a batch of operations. Returns the number of applied operations."""
    if not isinstance(ops, list):
        raise InvalidOperation('ops 必须是数组')
    if len(ops) > 5000:
        raise InvalidOperation('单次操作过多')
    for op in ops:
        if not isinstance(op, dict):
            raise InvalidOperation('操作格式不正确')
        kind = op.get('t')
        action = op.get('op')
        if kind == 'reset' and action == 'reset':
            clear_owner(session, owner)
            continue
        if kind == 'questions':
            if action == 'upsert':
                _upsert_question(session, owner, op)
            elif action == 'delete':
                row = session.get(Question, _ident(op.get('id')))
                if row is not None and row.owner_id == owner:
                    session.delete(row)
            else:
                raise InvalidOperation('未知的 questions 操作')
        elif kind == 'logs':
            if action in ('upsert', 'append'):
                _upsert_log(session, owner, op)
            elif action == 'delete':
                row = session.get(Log, _ident(op.get('id')))
                if row is not None and row.owner_id == owner:
                    session.delete(row)
            else:
                raise InvalidOperation('未知的 logs 操作')
        elif kind == 'rewards':
            if action == 'upsert':
                _upsert_named(session, owner, op, Reward)
            elif action == 'delete':
                row = session.get(Reward, _ident(op.get('id')))
                if row is not None and row.owner_id == owner:
                    session.delete(row)
            else:
                raise InvalidOperation('未知的 rewards 操作')
        elif kind == 'redemptions':
            if action == 'upsert':
                _upsert_named(session, owner, op, Redemption, date_field=True)
            elif action == 'delete':
                row = session.get(Redemption, _ident(op.get('id')))
                if row is not None and row.owner_id == owner:
                    session.delete(row)
            else:
                raise InvalidOperation('未知的 redemptions 操作')
        elif kind == 'prefs':
            if action == 'replace':
                _replace_prefs(session, owner, op)
            else:
                raise InvalidOperation('未知的 prefs 操作')
        elif kind == 'timer':
            if action == 'replace':
                _replace_timer(session, owner, op)
            else:
                raise InvalidOperation('未知的 timer 操作')
        elif kind == 'aiConfig':
            if action == 'replace':
                _replace_ai_config(session, owner, op)
            else:
                raise InvalidOperation('未知的 aiConfig 操作')
        elif kind == 'aiSets':
            if action == 'upsert':
                _upsert_ai_set(session, owner, op)
            elif action == 'delete':
                row = session.get(AiSet, _ident(op.get('id')))
                if row is not None and row.owner_id == owner:
                    session.execute(delete(AiSetQuestion).where(AiSetQuestion.set_id == row.id))
                    session.delete(row)
            else:
                raise InvalidOperation('未知的 aiSets 操作')
        else:
            raise InvalidOperation(f'未知的数据表 {kind}')
    return len(ops)


def replace_state(session: Session, owner: str, state: dict) -> None:
    """Replace everything belonging to one owner with the given state."""
    if (not isinstance(state, dict) or state.get('schema') != 1
            or not isinstance(state.get('questions'), list)
            or not isinstance(state.get('logs'), list)
            or not isinstance(state.get('prefs'), dict)):
        raise InvalidOperation('备份格式不正确')
    for field in LIST_TABLES + ('trash',):
        items = state.get(field, [])
        if not isinstance(items, list) or any(not isinstance(item, dict) for item in items):
            raise InvalidOperation(f'{field} 备份数据格式不正确')
    clear_owner(session, owner)
    ops: list[dict] = []
    for index, item in enumerate(state.get('questions') or []):
        ops.append({'t': 'questions', 'op': 'upsert', 'id': item.get('id'),
                    'ord': index, 'trashed': False, 'row': item})
    for index, item in enumerate(state.get('trash') or []):
        ops.append({'t': 'questions', 'op': 'upsert', 'id': item.get('id'),
                    'ord': index, 'trashed': True, 'row': item})
    for index, item in enumerate(state.get('logs') or []):
        ops.append({'t': 'logs', 'op': 'upsert', 'id': item.get('id'), 'ord': index, 'row': item})
    for index, item in enumerate(state.get('rewards') or []):
        ops.append({'t': 'rewards', 'op': 'upsert', 'id': item.get('id'), 'ord': index, 'row': item})
    for index, item in enumerate(state.get('redemptions') or []):
        ops.append({'t': 'redemptions', 'op': 'upsert', 'id': item.get('id'), 'ord': index, 'row': item})
    for index, item in enumerate(state.get('aiSets') or []):
        ops.append({'t': 'aiSets', 'op': 'upsert', 'id': item.get('id'), 'ord': index, 'row': item})
    prefs = state.get('prefs')
    if isinstance(prefs, dict):
        ops.append({'t': 'prefs', 'op': 'replace', 'row': prefs})
    ops.append({'t': 'timer', 'op': 'replace', 'row': state.get('timer')})
    ai_config = state.get('aiConfig')
    if isinstance(ai_config, dict):
        ops.append({'t': 'aiConfig', 'op': 'replace', 'row': ai_config})
    apply_ops(session, owner, ops)
