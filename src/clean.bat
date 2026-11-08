:: Copyright 2012 The Go Authors. All rights reserved.
:: Use of this source code is governed by a BSD-style
:: license that can be found in the LICENSE file.

@echo off

setlocal

bolero tool dist env -w -p >env.bat || exit /b 1
call .\env.bat
del env.bat
echo.

if not exist %BOLERO_TOOLDIR%\dist.exe (
    echo cannot find %BOLERO_TOOLDIR%\dist.exe; nothing to clean
    exit /b 1
)

"%BOLERO_BIN%\bolero" clean -i std
"%BOLERO_BIN%\bolero" tool dist clean
"%BOLERO_BIN%\bolero" clean -i cmd
