"""Database layer: SQLAlchemy models, engine and session helpers.

The schema mirrors the app's state object:

    questions   wrong answers, including the ones in the recycle bin
    logs        review / practice / focus records
    rewards     self-defined rewards
    redemptions reward redemptions
    prefs       one row per owner
    timers      one row per owner
    ai_sets     generated practice sets and their questions
    ai_keys     provider API keys, encrypted with the secret from .env

Every table carries ``owner_id``, so two different access tokens can never see
each other's data.
"""
from __future__ import annotations

import sqlite3
from collections.abc import Iterator
from pathlib import Path

from sqlalchemy import (
    Boolean,
    Float,
    ForeignKey,
    Integer,
    String,
    Text,
    create_engine,
    event,
    text,
)
from sqlalchemy.orm import DeclarativeBase, Mapped, Session, mapped_column, sessionmaker

from . import config


class Base(DeclarativeBase):
    pass


class Question(Base):
    __tablename__ = 'questions'

    id: Mapped[str] = mapped_column(String(100), primary_key=True)
    owner_id: Mapped[str] = mapped_column(String(32), index=True)
    ord: Mapped[int] = mapped_column(Integer, default=0)
    trashed: Mapped[bool] = mapped_column(Boolean, default=False, index=True)
    subject: Mapped[str] = mapped_column(Text, default='')
    title: Mapped[str] = mapped_column(Text, default='')
    answer: Mapped[str] = mapped_column(Text, default='')
    reason: Mapped[str] = mapped_column(Text, default='')
    tag: Mapped[str] = mapped_column(Text, default='')
    cause: Mapped[str] = mapped_column(Text, default='')
    difficulty: Mapped[str] = mapped_column(Text, default='')
    image: Mapped[str] = mapped_column(Text, default='')
    favorite: Mapped[bool] = mapped_column(Boolean, default=False)
    stage: Mapped[int] = mapped_column(Integer, default=0)
    due: Mapped[str] = mapped_column(String(10), default='')
    created: Mapped[str] = mapped_column(String(10), default='')
    demo: Mapped[bool] = mapped_column(Boolean, default=False)


class Log(Base):
    __tablename__ = 'logs'

    id: Mapped[str] = mapped_column(String(100), primary_key=True)
    owner_id: Mapped[str] = mapped_column(String(32), index=True)
    ord: Mapped[int] = mapped_column(Integer, default=0)
    date: Mapped[str] = mapped_column(String(10), index=True)
    at: Mapped[str] = mapped_column(String(40), default='')
    type: Mapped[str] = mapped_column(String(20), default='')
    label: Mapped[str] = mapped_column(Text, default='')
    points: Mapped[float] = mapped_column(Float, default=0)
    qid: Mapped[str | None] = mapped_column(String(100), nullable=True)
    grade: Mapped[str | None] = mapped_column(String(20), nullable=True)
    minutes: Mapped[float | None] = mapped_column(Float, nullable=True)


class Reward(Base):
    __tablename__ = 'rewards'

    id: Mapped[str] = mapped_column(String(100), primary_key=True)
    owner_id: Mapped[str] = mapped_column(String(32), index=True)
    ord: Mapped[int] = mapped_column(Integer, default=0)
    name: Mapped[str] = mapped_column(Text, default='')
    cost: Mapped[int] = mapped_column(Integer, default=0)


class Redemption(Base):
    __tablename__ = 'redemptions'

    id: Mapped[str] = mapped_column(String(100), primary_key=True)
    owner_id: Mapped[str] = mapped_column(String(32), index=True)
    ord: Mapped[int] = mapped_column(Integer, default=0)
    name: Mapped[str] = mapped_column(Text, default='')
    cost: Mapped[int] = mapped_column(Integer, default=0)
    date: Mapped[str] = mapped_column(String(10), default='')


class Prefs(Base):
    __tablename__ = 'prefs'

    owner_id: Mapped[str] = mapped_column(String(32), primary_key=True)
    name: Mapped[str] = mapped_column(Text, default='')
    goal: Mapped[int] = mapped_column(Integer, default=5)
    minutes: Mapped[int] = mapped_column(Integer, default=25)
    theme: Mapped[str] = mapped_column(String(20), default='light')


class Timer(Base):
    __tablename__ = 'timers'

    owner_id: Mapped[str] = mapped_column(String(32), primary_key=True)
    running: Mapped[bool] = mapped_column(Boolean, default=False)
    remaining: Mapped[float] = mapped_column(Float, default=0)
    duration: Mapped[float] = mapped_column(Float, default=0)
    end: Mapped[float | None] = mapped_column(Float, nullable=True)


class AiConfig(Base):
    __tablename__ = 'ai_config'

    owner_id: Mapped[str] = mapped_column(String(32), primary_key=True)
    provider: Mapped[str] = mapped_column(String(20), default='qwen')
    base: Mapped[str] = mapped_column(Text, default='')
    vision: Mapped[str] = mapped_column(String(120), default='')
    text: Mapped[str] = mapped_column(String(120), default='')
    deepseek_base: Mapped[str] = mapped_column(Text, default='')
    deepseek_vision: Mapped[str] = mapped_column(String(120), default='')
    deepseek_text: Mapped[str] = mapped_column(String(120), default='')


class AiKey(Base):
    """Provider API keys, encrypted with the secret from ``.env``.

    The ciphertext never leaves the database: the backend decrypts it for
    every request and the browser only learns whether a key exists.
    """

    __tablename__ = 'ai_keys'

    owner_id: Mapped[str] = mapped_column(String(32), primary_key=True)
    provider: Mapped[str] = mapped_column(String(20), primary_key=True)
    ciphertext: Mapped[str] = mapped_column(Text, default='')
    updated: Mapped[str] = mapped_column(String(20), default='')


class AiSet(Base):
    __tablename__ = 'ai_sets'

    id: Mapped[str] = mapped_column(String(100), primary_key=True)
    owner_id: Mapped[str] = mapped_column(String(32), index=True)
    ord: Mapped[int] = mapped_column(Integer, default=0)
    source_id: Mapped[str] = mapped_column(String(100), default='')
    source_title: Mapped[str] = mapped_column(Text, default='')
    subject: Mapped[str] = mapped_column(Text, default='')
    created: Mapped[str] = mapped_column(String(10), default='')
    ai_reviewed: Mapped[bool] = mapped_column(Boolean, default=False)


class AiSetQuestion(Base):
    __tablename__ = 'ai_set_questions'

    id: Mapped[str] = mapped_column(String(100), primary_key=True)
    set_id: Mapped[str] = mapped_column(String(100), ForeignKey('ai_sets.id', ondelete='CASCADE'), index=True)
    ord: Mapped[int] = mapped_column(Integer, default=0)
    title: Mapped[str] = mapped_column(Text, default='')
    answer: Mapped[str] = mapped_column(Text, default='')
    explanation: Mapped[str] = mapped_column(Text, default='')
    knowledge: Mapped[str] = mapped_column(Text, default='')
    note: Mapped[str] = mapped_column(Text, default='')
    revealed: Mapped[bool] = mapped_column(Boolean, default=False)
    grade: Mapped[str] = mapped_column(String(20), default='')


_engine = None
_Session = None

# SQLite before 3.35 cannot drop a column, so the table is rebuilt instead.
REBUILD_AI_CONFIG = """
CREATE TABLE ai_config_new (
    owner_id VARCHAR(32) NOT NULL,
    provider VARCHAR(20),
    base TEXT,
    vision VARCHAR(120),
    text VARCHAR(120),
    deepseek_base TEXT,
    deepseek_vision VARCHAR(120),
    deepseek_text VARCHAR(120),
    PRIMARY KEY (owner_id)
);
INSERT INTO ai_config_new (owner_id, provider, base, vision, text, deepseek_base,
                            deepseek_vision, deepseek_text)
    SELECT owner_id, provider, base, vision, text, deepseek_base,
           deepseek_vision, deepseek_text FROM ai_config;
DROP TABLE ai_config;
ALTER TABLE ai_config_new RENAME TO ai_config
"""


def _migrate(engine) -> None:
    """Remove the retired ``ai_config.transport`` column from older databases."""
    with engine.begin() as conn:
        rows = conn.execute(text('PRAGMA table_info(ai_config)')).fetchall()
        if 'transport' not in {row[1] for row in rows}:
            return
        if sqlite3.sqlite_version_info >= (3, 35, 0):
            conn.execute(text('ALTER TABLE ai_config DROP COLUMN transport'))
            return
        for statement in REBUILD_AI_CONFIG.split(';'):
            if statement.strip():
                conn.execute(text(statement))


def _make_engine(url: str):
    engine = create_engine(url, future=True, connect_args={'check_same_thread': False})

    @event.listens_for(engine, 'connect')
    def _sqlite_pragmas(dbapi_connection, _record):
        cursor = dbapi_connection.cursor()
        cursor.execute('PRAGMA foreign_keys=ON')
        cursor.execute('PRAGMA journal_mode=WAL')
        cursor.close()

    return engine


def engine():
    """The process-wide engine, created on first use."""
    global _engine, _Session
    if _engine is None:
        path = config.database_path()
        path.parent.mkdir(parents=True, exist_ok=True)
        _engine = _make_engine(f'sqlite:///{path}')
        Base.metadata.create_all(_engine)
        _migrate(_engine)
        _Session = sessionmaker(bind=_engine, expire_on_commit=False, future=True)
    return _engine


def session_factory():
    engine()
    return _Session


def session_scope() -> Iterator[Session]:
    """A session that is always closed again."""
    session = session_factory()()
    try:
        yield session
    finally:
        session.close()


def reset_engine():
    """Drop the cached engine (used by the tests)."""
    global _engine, _Session
    if _engine is not None:
        _engine.dispose()
    _engine = None
    _Session = None


def database_exists() -> bool:
    return Path(config.database_path()).is_file()
