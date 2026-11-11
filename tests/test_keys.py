"""Tests for the encryption of the provider API keys (src/shiguang_study/keys.py).

The tests never touch the project's own ``.env``: they point
``SHIGUANG_ENV_FILE`` at a temporary file instead.
"""
import base64
import os

import pytest

from shiguang_study import keys

SECRET = 'test-secret-value'
PLAIN = 'sk-TESTKEY0123456789ABCDEF'


@pytest.fixture()
def env_file(monkeypatch, tmp_path):
    """A temporary .env targeted by SHIGUANG_ENV_FILE, with the secret unset."""
    path = tmp_path / '.env'
    monkeypatch.setenv(keys.ENV_FILE_ENV, str(path))
    monkeypatch.delenv(keys.SECRET_ENV, raising=False)
    keys.reset()
    yield path
    keys.reset()


@pytest.fixture()
def secret_env(monkeypatch):
    """SECRET_ENV pinned to a fixed value; environment restored afterwards."""
    monkeypatch.setenv(keys.SECRET_ENV, SECRET)
    keys.reset()
    yield
    keys.reset()


def test_the_env_file_is_read_into_the_environment(env_file):
    env_file.write_text('# comment\nexport SHIGUANG_OTHER=1\nQUOTED="two"\n',
                        encoding='utf-8')
    keys.load_env_file()
    assert os.environ['SHIGUANG_OTHER'] == '1'
    assert os.environ['QUOTED'] == 'two'


def test_the_environment_wins_over_the_env_file(env_file, monkeypatch):
    env_file.write_text(f'{keys.SECRET_ENV}=from-file\n', encoding='utf-8')
    monkeypatch.setenv(keys.SECRET_ENV, 'from-environment')
    keys.load_env_file()
    assert os.environ[keys.SECRET_ENV] == 'from-environment'


def test_a_missing_secret_is_generated_and_persisted(env_file):
    secret = keys.secret()
    assert secret
    assert keys.SECRET_ENV in env_file.read_text(encoding='utf-8')
    assert keys.secret() == secret, 'the generated secret must be stable'
    assert env_file.stat().st_mode & 0o077 == 0, \
        'the env file must not be readable by other users'


def test_the_secret_comes_from_the_env_file(env_file):
    env_file.write_text(f'{keys.SECRET_ENV}={SECRET}\n', encoding='utf-8')
    assert keys.secret() == SECRET


def test_a_key_survives_the_round_trip(secret_env):
    ciphertext = keys.encrypt(PLAIN)
    assert PLAIN not in ciphertext
    assert keys.decrypt(ciphertext) == PLAIN


def test_every_encryption_uses_a_fresh_nonce(secret_env):
    assert keys.encrypt(PLAIN) != keys.encrypt(PLAIN)


def test_another_secret_cannot_decrypt(secret_env, monkeypatch):
    ciphertext = keys.encrypt(PLAIN)
    monkeypatch.setenv(keys.SECRET_ENV, 'another-secret')
    keys.reset()
    with pytest.raises(ValueError):
        keys.decrypt(ciphertext)


def test_a_tampered_ciphertext_is_rejected(secret_env):
    raw = bytearray(base64.b64decode(keys.encrypt(PLAIN)))
    raw[-1] ^= 0x01
    with pytest.raises(ValueError):
        keys.decrypt(base64.b64encode(bytes(raw)).decode())


@pytest.mark.parametrize('value', [
    '',
    'not base64!!',
    base64.b64encode(b'short').decode(),
])
def test_broken_ciphertext_is_rejected(secret_env, value):
    with pytest.raises(ValueError):
        keys.decrypt(value)


def test_the_preview_hides_the_middle_of_the_key():
    assert keys.preview('sk-abcdefghijklmnop') == 'sk-a****mnop'


@pytest.mark.parametrize('value', ['', 'sk-short', 'a' * 12])
def test_a_short_key_is_masked_completely(value):
    assert keys.preview(value) == '****'
