"""The Shiguang Study Room backend package.

Run it with::

    uv run python -m shiguang_study.webapp
"""
from .app import app, create_app

__all__ = ['app', 'create_app']
