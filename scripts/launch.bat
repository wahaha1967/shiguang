@echo off
setlocal
rem ASCII commands and CRLF line endings keep this launcher compatible with CMD.
rem UTF-8 mode is required when Python reads the bundled UTF-8 HTML file.
set "PYTHONUTF8=1"
pushd "%~dp0"
if errorlevel 1 goto folder_error

rem The built bundle (dist\) runs on plain python3 - no uv, no install.
rem The source tree prefers "uv run shiguang-study" so the managed
rem environment is used, and falls back to plain python3 when uv is absent.
if exist "%~dp0bridge.py" goto run_plain

where uv >nul 2>&1
if errorlevel 1 goto try_python
if not exist "%~dp0..\pyproject.toml" goto try_python
pushd "%~dp0.."
uv run shiguang-study %*
popd
goto finished

:try_python
if not exist "%~dp0..\src\shiguang_study\bridge.py" goto missing_entry
python "%~dp0..\src\shiguang_study\bridge.py" %*
goto finished

:run_plain
where py >nul 2>&1
if errorlevel 1 goto try_python_plain
py -3 "%~dp0bridge.py" %*
goto finished

:try_python_plain
where python >nul 2>&1
if errorlevel 1 goto missing_python
python "%~dp0bridge.py" %*

:finished
set "STUDY_EXIT=%errorlevel%"
if not "%STUDY_EXIT%"=="0" echo The workbench could not start. Check the error above.
popd
pause
exit /b %STUDY_EXIT%

:missing_entry
echo Bridge entry not found. Build the bundle first:  python3 scripts\build.py
goto failed

:missing_python
echo Python 3 was not found. Install Python 3 from https://www.python.org/downloads/windows/
echo Enable the Python launcher or the Add Python to PATH option, then try again.
goto failed

:failed
popd
pause
exit /b 1

:folder_error
echo Cannot access the workbench folder. Extract the whole ZIP to a local folder first.
pause
exit /b 1
