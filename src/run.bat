:: Copyright 2012 The Go Authors. All rights reserved.
:: Use of this source code is governed by a BSD-style
:: license that can be found in the LICENSE file.

@echo off

if not exist ..\bin\bolero.exe (
    echo Must run run.bat from Bolero src directory after installing cmd/go.
    exit /b 1
)

setlocal

set BOLERO_ENV=off
..\bin\bolero tool dist env > env.bat || exit /b 1
call .\env.bat
del env.bat

set BOLERO_PATH=c:\nonexist-gopath
..\bin\bolero tool dist test --rebuild %* || exit /b 1
