"""Regression tests for ownership and transaction boundaries."""

import sqlite3

import pytest
from sqlalchemy import create_engine
from sqlalchemy.orm import Session

from shiguang_study.webapp import db, ops

LEGACY_AI_CONFIG = """
CREATE TABLE ai_config (
    owner_id VARCHAR(32) NOT NULL,
    provider VARCHAR(20),
    base TEXT,
    vision VARCHAR(120),
    text VARCHAR(120),
    deepseek_base TEXT,
    deepseek_vision VARCHAR(120),
    deepseek_text VARCHAR(120),
    transport VARCHAR(20) NOT NULL,
    PRIMARY KEY (owner_id)
)
"""


@pytest.fixture()
def session():
    engine = create_engine('sqlite:///:memory:')
    db.Base.metadata.create_all(engine)
    with Session(engine) as new_session:
        yield new_session
    engine.dispose()


OWNER_TAKEN_CASES = [
    ('questions', {'id': 'same', 'title': '原题', 'answer': '答案',
                   'due': '2026-11-10', 'created': '2026-11-10'}),
    ('logs', {'id': 'same', 'date': '2026-11-10', 'type': 'add'}),
    ('rewards', {'id': 'same', 'name': '奖励', 'cost': 1}),
    ('redemptions', {'id': 'same', 'name': '奖励', 'cost': 1, 'date': '2026-11-10'}),
    ('aiSets', {'id': 'same', 'created': '2026-11-10',
                'questions': [{'id': f'item-{index}'} for index in range(3)]}),
]


@pytest.mark.parametrize('kind,row', OWNER_TAKEN_CASES, ids=[c[0] for c in OWNER_TAKEN_CASES])
def test_upsert_cannot_take_another_owners_record(session, kind, row):
    operation = {'t': kind, 'op': 'upsert', 'row': row}
    ops.apply_ops(session, 'owner-a', [operation])
    session.commit()
    with pytest.raises(ops.InvalidOperation):
        ops.apply_ops(session, 'owner-b', [operation])
    session.rollback()
    assert len(ops.read_state(session, 'owner-a')[kind]) == 1
    assert len(ops.read_state(session, 'owner-b')[kind]) == 0


def test_owner_can_update_existing_question(session):
    row = {'id': 'question-1', 'title': '原题', 'answer': '答案',
           'due': '2026-11-10', 'created': '2026-11-10'}
    operation = {'t': 'questions', 'op': 'upsert', 'row': row}
    ops.apply_ops(session, 'owner-a', [operation])
    session.commit()
    row['title'] = '修改后的题目'
    ops.apply_ops(session, 'owner-a', [operation])
    session.commit()
    assert ops.read_state(session, 'owner-a')['questions'][0]['title'] == '修改后的题目'


def test_import_with_existing_ai_set_replaces_successfully(session):
    ai_set = {'id': 'set-1', 'created': '2026-11-10',
              'questions': [{'id': f'item-{index}'} for index in range(3)]}
    ops.apply_ops(session, 'owner-a', [{'t': 'aiSets', 'op': 'upsert', 'row': ai_set}])
    session.commit()
    ops.replace_state(session, 'owner-a', {'schema': 1, 'questions': [], 'logs': [],
                                           'prefs': {'name': '同学'}})
    session.commit()
    assert ops.read_state(session, 'owner-a')['aiSets'] == []


def test_invalid_import_does_not_delete_existing_data(session):
    row = {'id': 'original', 'title': '原题', 'answer': '答案',
           'due': '2026-11-10', 'created': '2026-11-10'}
    ops.apply_ops(session, 'owner-a', [{'t': 'questions', 'op': 'upsert', 'row': row}])
    session.commit()
    with pytest.raises(ops.InvalidOperation):
        ops.replace_state(session, 'owner-a', {'schema': 1, 'questions': [None],
                                               'logs': [], 'prefs': {'name': '同学'}})
    session.rollback()
    assert ops.read_state(session, 'owner-a')['questions'][0]['title'] == '原题'


def test_incomplete_import_is_rejected(session):
    with pytest.raises(ops.InvalidOperation):
        ops.replace_state(session, 'owner-a', {'questions': []})


def test_failed_replacement_rolls_back_deletions(session):
    row = {'id': 'original', 'title': '原题', 'answer': '答案',
           'due': '2026-11-10', 'created': '2026-11-10'}
    ops.apply_ops(session, 'owner-a', [{'t': 'questions', 'op': 'upsert', 'row': row}])
    session.commit()
    invalid = {**row, 'stage': 10}
    with pytest.raises(ops.InvalidOperation):
        ops.replace_state(session, 'owner-a', {'schema': 1, 'questions': [invalid],
                                               'logs': [], 'prefs': {'name': '同学'}})
    session.rollback()
    assert ops.read_state(session, 'owner-a')['questions'][0]['title'] == '原题'


def test_failed_batch_rolls_back_previous_operations(session):
    row = {'id': 'new', 'title': '新题', 'answer': '答案',
           'due': '2026-11-10', 'created': '2026-11-10'}
    with pytest.raises(ops.InvalidOperation):
        ops.apply_ops(session, 'owner-a', [
            {'t': 'questions', 'op': 'upsert', 'row': row},
            {'t': 'unknown', 'op': 'upsert'},
        ])
    session.rollback()
    assert ops.read_state(session, 'owner-a')['questions'] == []


def test_keys_are_not_shared_between_owners(session):
    ops.set_key(session, 'owner-a', 'qwen', 'ciphertext-a')
    session.commit()
    assert ops.read_key(session, 'owner-a', 'qwen') == 'ciphertext-a'
    assert ops.read_key(session, 'owner-b', 'qwen') == ''
    assert ops.read_keys(session, 'owner-b') == {}


def test_clearing_the_owner_keeps_the_keys(session):
    ops.set_key(session, 'owner-a', 'qwen', 'ciphertext-a')
    session.commit()
    ops.clear_owner(session, 'owner-a')
    session.commit()
    assert ops.read_key(session, 'owner-a', 'qwen') == 'ciphertext-a'


@pytest.fixture()
def legacy_db(tmp_path, monkeypatch):
    """A database from an older release: ai_config still has the transport column."""
    path = tmp_path / 'study.db'
    monkeypatch.setenv('SHIGUANG_DB', str(path))
    db.reset_engine()
    with sqlite3.connect(path) as connection:
        connection.execute(LEGACY_AI_CONFIG)
        connection.execute("INSERT INTO ai_config (owner_id, provider, transport) "
                           "VALUES ('owner-a', 'qwen', 'proxy')")
    yield path
    db.reset_engine()


def test_the_transport_column_is_removed_and_the_data_survives(legacy_db):
    engine = db.engine()
    with engine.connect() as connection:
        columns = {row[1] for row in connection.exec_driver_sql('PRAGMA table_info(ai_config)')}
        provider = connection.exec_driver_sql(
            'SELECT provider FROM ai_config WHERE owner_id = ?', ('owner-a',)).scalar()
    assert 'transport' not in columns
    assert provider == 'qwen'


def test_the_ai_config_can_still_be_written(legacy_db):
    with db.session_factory()() as session:
        ops.apply_ops(session, 'owner-a', [{'t': 'aiConfig', 'op': 'replace', 'row': {
            'provider': 'qwen', 'base': 'https://dashscope.aliyuncs.com/compatible-mode/v1',
            'vision': 'qwen3-vl-plus', 'text': 'qwen-plus'}}])
        session.commit()
        state = ops.read_state(session, 'owner-a')
    assert state['aiConfig']['provider'] == 'qwen'
    assert 'transport' not in state['aiConfig']
