#!/usr/bin/env bash
# Copyright 2009 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

set -e

if [ ! -f run.bash ]; then
	echo 'clean.bash must be run from $BOLERO_ROOT/src' 1>&2
	exit 1
fi
export BOLERO_ROOT="$(cd .. && pwd)"

gobin="${BOLERO_ROOT}"/bin
if ! "$gobin"/bolero help >/dev/null 2>&1; then
	echo 'cannot find bolero command; nothing to clean' >&2
	exit 1
fi

"$gobin/bolero" clean -i std
"$gobin/bolero" tool dist clean
"$gobin/bolero" clean -i cmd
