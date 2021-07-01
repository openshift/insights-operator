#!/usr/bin/env bash
# Copyright 2021 Red Hat, Inc
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

THRESHOLD=50

RED_BG=$(tput setab 1)
GREEN_BG=$(tput setab 2)
NC=$(tput sgr0) # No Color

if ! [[ $* == *do-not-run-tests* ]]; then
    make test || exit 1
fi

go_tool_cover_output=$(go tool cover -func=cover.out)

echo "$go_tool_cover_output"

if (($(echo "$go_tool_cover_output" | tail -n 1 | awk '{print $NF}' | grep -E "^[0-9]+" -o) >= THRESHOLD)); then
    echo -e "${GREEN_BG}[OK]${NC} Code coverage is OK"
    exit 0
else
    echo -e "${RED_BG}[FAIL]${NC} Code coverage have to be at least $THRESHOLD%"
    exit 1
fi
