"""Encryption of the provider API keys.

The plain key only exists in the request that carries it. The backend
encrypts it with the secret from ``.env`` and stores the ciphertext in
SQLite; every AI request decrypts it again. The browser never receives a key
back - only the fact that one is configured.

The secret lives in the ``.env`` file next to the project (``SHIGUANG_SECRET``).
When it is missing, one is generated on first use and written back to that
file with owner-only permissions.
"""
from __future__ import annotations

import base64
import hashlib
import os
import secrets
from functools import lru_cache
from pathlib import Path

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

SECRET_ENV = 'SHIGUANG_SECRET'
ENV_FILE_ENV = 'SHIGUANG_ENV_FILE'
NONCE_SIZE = 12


def _parse_env(text: str) -> dict[str, str]:
    """Parse the ``KEY=VALUE`` lines of a dotenv file."""
    values: dict[str, str] = {}
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith('#'):
            continue
        if line.startswith('export '):
            line = line[len('export '):].strip()
        name, separator, value = line.partition('=')
        if not separator:
            continue
        name = name.strip()
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in '"\'':
            value = value[1:-1]
        if name:
            values[name] = value
    return values


def env_file() -> Path:
    """Locate the ``.env`` file that holds the encryption secret.

    ``SHIGUANG_ENV_FILE`` wins, then the working directory, then the project
    root, and finally the data directory of an installed backend.
    """
    override = os.environ.get(ENV_FILE_ENV)
    if override:
        return Path(override).expanduser()
    candidates = [Path.cwd() / '.env']
    here = Path(__file__).resolve().parent
    candidates.extend(base / '.env' for base in list(here.parents)[:4])
    data_dir = os.environ.get('SHIGUANG_DATA_DIR')
    candidates.append((Path(data_dir).expanduser() if data_dir
                       else Path.home() / '.local' / 'share' / 'shiguang-study') / '.env')
    for candidate in candidates:
        if candidate.is_file():
            return candidate
    return candidates[0]


def load_env_file() -> Path:
    """Load the ``.env`` file into the environment; real variables win."""
    path = env_file()
    try:
        text = path.read_text(encoding='utf-8')
    except OSError:
        return path
    for name, value in _parse_env(text).items():
        os.environ.setdefault(name, value)
    return path


def secret() -> str:
    """The encryption secret, generated and persisted on first use."""
    load_env_file()
    value = os.environ.get(SECRET_ENV, '').strip()
    if value:
        return value
    value = secrets.token_urlsafe(32)
    path = env_file()
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open('a', encoding='utf-8') as handle:
            handle.write(f'\n# {SECRET_ENV}：用于加密数据库中的 API Key，请勿泄露或提交\n'
                         f'{SECRET_ENV}={value}\n')
        path.chmod(0o600)
    except OSError:
        pass
    os.environ[SECRET_ENV] = value
    return value


@lru_cache(maxsize=1)
def _cipher() -> AESGCM:
    """AES-256-GCM with a key derived from the secret."""
    return AESGCM(hashlib.sha256(secret().encode('utf-8')).digest())


def reset() -> None:
    """Forget the cached cipher; used when the secret changes."""
    _cipher.cache_clear()


def encrypt(plaintext: str) -> str:
    """Encrypt one API key into a string that is safe to store."""
    nonce = os.urandom(NONCE_SIZE)
    blob = _cipher().encrypt(nonce, plaintext.encode('utf-8'), None)
    return base64.b64encode(nonce + blob).decode('ascii')


def decrypt(ciphertext: str) -> str:
    """Decrypt one stored API key."""
    try:
        raw = base64.b64decode(ciphertext.encode('ascii'), validate=True)
    except ValueError as error:
        raise ValueError('密钥数据损坏，无法解密') from error
    if len(raw) <= NONCE_SIZE:
        raise ValueError('密钥数据损坏，无法解密')
    try:
        plain = _cipher().decrypt(raw[:NONCE_SIZE], raw[NONCE_SIZE:], None)
    except InvalidTag as error:
        raise ValueError('密钥解密失败，请检查 .env 中的加密串是否被改动') from error
    return plain.decode('utf-8')


def preview(plaintext: str) -> str:
    """A masked hint of a key: the first and last four characters only.

    Short keys are masked completely, so the hint never reveals the secret.
    """
    text = plaintext.strip()
    if len(text) <= 12:
        return '****'
    return f'{text[:4]}****{text[-4:]}'
