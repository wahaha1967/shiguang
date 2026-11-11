"""Launch-order tests for the ``shiguang-study`` entry point.

``--open`` points the browser at the app only after uvicorn owns the listening
socket; opening earlier leaves the tab on a dead port and the window blank.
"""
import contextlib
import io
import sys
import threading
from unittest import mock

from shiguang_study.webapp import __main__ as entry


class _FakeServer:
    """Base server that records startup; override ``run`` per test."""

    def __init__(self, config):
        self.config = config
        self.started = False


def test_browser_opens_only_after_the_server_starts():
    events: list[str] = []
    opened_urls: list[str] = []
    opened = threading.Event()

    class FakeServer(_FakeServer):
        def run(self):
            events.append('run')
            self.started = True
            opened.wait(timeout=2)

    def fake_open(url: str) -> bool:
        events.append('open')
        opened_urls.append(url)
        opened.set()
        return True

    with mock.patch.object(entry.uvicorn, 'Server', FakeServer), \
            mock.patch.object(entry.webbrowser, 'open', fake_open), \
            mock.patch.object(sys, 'argv', ['shiguang-study', '--open']), \
            contextlib.redirect_stdout(io.StringIO()):
        entry.main()

    assert events[:2] == ['run', 'open']
    assert opened_urls == ['http://127.0.0.1:4312/']


def test_the_browser_is_not_opened_by_default():
    opened: list[str] = []

    class FakeServer(_FakeServer):
        def run(self):
            self.started = True

    with mock.patch.object(entry.uvicorn, 'Server', FakeServer), \
            mock.patch.object(entry.webbrowser, 'open', opened.append), \
            mock.patch.object(sys, 'argv', ['shiguang-study']), \
            contextlib.redirect_stdout(io.StringIO()):
        entry.main()

    assert opened == []
