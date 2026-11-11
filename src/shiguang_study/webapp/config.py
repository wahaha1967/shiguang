"""Runtime configuration for the backend.

Everything is overridable through environment variables so the same code runs
locally and on a server:

    SHIGUANG_TOKEN     access token; generated and persisted when unset
    SHIGUANG_DB        path to the SQLite file
    SHIGUANG_DATA_DIR  directory for the default database and token file
    SHIGUANG_HOST      bind address (default 127.0.0.1)
    SHIGUANG_PORT      bind port (default 4312)
"""
import hashlib
import os
import secrets
from functools import lru_cache
from pathlib import Path

DEFAULT_HOST = '127.0.0.1'
DEFAULT_PORT = 4312


@lru_cache(maxsize=1)
def app_path() -> Path:
    """Locate the single-file app in the source tree.

    ``SHIGUANG_APP`` overrides the search with an explicit path; otherwise walk up
    from the package to find ``src/app/index.html``.
    """
    override = os.environ.get('SHIGUANG_APP')
    if override:
        candidate = Path(override)
        if candidate.is_file():
            return candidate
    here = Path(__file__).resolve().parent
    for base in list(here.parents)[:6]:
        candidate = base / 'src' / 'app' / 'index.html'
        if candidate.is_file():
            return candidate
    return here / 'index.html'


@lru_cache(maxsize=1)
def data_dir() -> Path:
    override = os.environ.get('SHIGUANG_DATA_DIR')
    if override:
        return Path(override).expanduser()
    return Path.home() / '.local' / 'share' / 'shiguang-study'


def database_path() -> Path:
    override = os.environ.get('SHIGUANG_DB')
    if override:
        return Path(override).expanduser()
    return data_dir() / 'study.db'


@lru_cache(maxsize=1)
def token() -> str:
    """The access token.

    Read from SHIGUANG_TOKEN when set, otherwise generated once and persisted
    so that restarting the server does not lock the owner out.
    """
    override = os.environ.get('SHIGUANG_TOKEN')
    if override:
        return override.strip()
    path = data_dir() / 'token'
    try:
        if path.is_file():
            stored = path.read_text(encoding='utf-8').strip()
            if stored:
                return stored
    except OSError:
        pass
    generated = secrets.token_urlsafe(32)
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(generated, encoding='utf-8')
        path.chmod(0o600)
    except OSError:
        pass
    return generated


def owner_id(access_token: str) -> str:
    """Stable owner key derived from the access token.

    There is no user table yet: every distinct token maps to its own data
    space. When accounts are introduced, this becomes a lookup into `users`.
    """
    return hashlib.sha256(access_token.encode()).hexdigest()[:16]


def host() -> str:
    return os.environ.get('SHIGUANG_HOST', DEFAULT_HOST)


def port() -> int:
    try:
        return int(os.environ.get('SHIGUANG_PORT', DEFAULT_PORT))
    except ValueError:
        return DEFAULT_PORT
