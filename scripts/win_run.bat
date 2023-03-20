@echo off
setlocal enabledelayedexpansion

if "%1"=="" (
    echo Please input the argument: start or stop
    goto :end
)

if /i "%1"=="start" (
    call :startLogic
    goto :end
)

if /i "%1"=="stop" (
    call :stopLogic
    goto :end
)

echo Invalid argument, please input start or stop

:startLogic
echo Executing start logic...
start /min "categraf.exe"
echo categraf.exe process started
goto :eof

:stopLogic
echo Executing stop logic...
for /f "tokens=2" %%A in ('tasklist -v ^| findstr categraf.exe') do (
    set pid=%%A
    goto :killProcess
)

:killProcess
if "%pid%"=="" (
    echo Process not found: categraf.exe
) else (
    echo Preparing to terminate process: categraf.exe, PID: %pid%
    taskkill /pid %pid% /f
    echo Process terminated
)

goto :eof

:end
pause

