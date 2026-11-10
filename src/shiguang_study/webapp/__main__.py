"""Entry point for ``python -m shiguang_study.webapp``."""
import uvicorn

from . import config


def main() -> None:
    host = config.host()
    port = config.port()
    print(f'Shiguang Study Room backend: http://{host}:{port}/')
    print(f'Database: {config.database_path()}')
    print('Press Ctrl+C to stop.')
    uvicorn.run('shiguang_study.webapp.app:app', host=host, port=port, log_level='info')


if __name__ == '__main__':
    main()
