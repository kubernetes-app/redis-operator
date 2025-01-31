#!/bin/bash
# 
# Copyright 2021 kubernetes-app Solutions.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#     http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

echo ">>> Installing Operator SDK"

arch=$(uname -m)
local_os=$(uname)
if [[ $local_os == "Linux" ]]; then
    target_os="linux-gnu"
elif [[ $local_os == "Darwin" ]]; then
    target_os="apple-darwin"
else
    echo "This system's OS $local_os isn't recognized/supported"
fi

# Use version 1.6.1
RELEASE_VERSION=v1.6.1
# Download binary
curl -LO https://github.com/operator-framework/operator-sdk/releases/download/${RELEASE_VERSION}/operator-sdk-${RELEASE_VERSION}-${arch}-${target_os}
# Install binary
chmod +x operator-sdk-${RELEASE_VERSION}-${arch}-${target_os} \n
&& mkdir -p /usr/local/bin/ \n
&& cp operator-sdk-${RELEASE_VERSION}-${arch}-${target_os} /usr/local/bin/operator-sdk \n
&& rm operator-sdk-${RELEASE_VERSION}-${arch}-${target_os}
