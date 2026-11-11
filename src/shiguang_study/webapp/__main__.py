"""Entry point for ``python -m shiguang_study.webapp``."""
import argparse
import threading
import time
import webbrowser

import uvicorn

from .. import keys
from . import config


def open_when_ready(server: uvicorn.Server, url: str) -> None:
    """Open the browser once the server owns its listening socket.

    Opening before ``uvicorn`` binds leaves the tab on a dead port, so the
    window stays blank until the server comes up.
    """
    while not server.started:
        time.sleep(0.05)
    webbrowser.open(url)


def main() -> None:
    keys.load_env_file()
    parser = argparse.ArgumentParser(description='Shiguang Study Room backend')
    parser.add_argument('--port', type=int, default=config.port())
    parser.add_argument('--host', default=config.host())
    parser.add_argument('--open', action='store_true',
                        help='open the app in a browser once the server is ready')
    args = parser.parse_args()
    host = args.host
    port = args.port
    url = f'http://{"127.0.0.1" if host == "0.0.0.0" else host}:{port}/'
    print(f'Shiguang Study Room backend: {url}')
    print(f'Database: {config.database_path()}')
    print('Press Ctrl+C to stop.')
    server = uvicorn.Server(uvicorn.Config('shiguang_study.webapp.app:app',
                                           host=host, port=port, log_level='info'))
    if args.open:
        threading.Thread(target=open_when_ready, args=(server, url), daemon=True).start()
    server.run()


if __name__ == '__main__':
    main()
