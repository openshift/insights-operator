#!/usr/bin/env python3

"""Script to generate certificate and user key from provided Kubernetes configuration file.

Generated files k8s.crt and k8s.key might be used to access Insights Operator
REST API endpoints and Prometheus metrics as well.
"""

# Copyright © 2020 Red Hat
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import yaml
import base64
import sys


def get_data_for_user(payload, user_name):
    """
    Try to retrieve data for given user.

    KeyError will be raised in case of improper payload format.
    """
    users = payload["users"]
    for user_data in users:
        if "name" in user_data and user_data["name"] == user_name:
            return user_data


def get_value_assigned_to_user(user_data, key):
    """
    Try to retrieve (attribute) value assigned to an user.

    In practise it will be certificate or key. KeyError will be raised in case
    of improper payload format or when the attribute for given key does not
    exist.
    """
    d = user_data["user"]
    return d[key]


def decode(b64):
    """
    Decode given attribute encoded by using Base64 encoding.

    The result is returned as regular Python string. Note that TypeError might
    be thrown when the input data are not encoded properly.
    """
    barray = base64.b64decode(b64)
    return barray.decode('ascii')


def generate_cert_and_key_files(input_file):
    """Generate file with certificate and user key from k8s configuration file."""
    with open(input_file) as f:
        payload = yaml.load(f)
        if payload is not None:
            user_data = get_data_for_user(payload, "admin")
            encoded_certificate = get_value_assigned_to_user(user_data, "client-certificate-data")
            encoded_key = get_value_assigned_to_user(user_data, "client-key-data")
            decoded_certificate = decode(encoded_certificate)
            decoded_key = decode(encoded_key)
            with open("k8s.crt", "w") as cert:
                cert.write(decoded_certificate)
            with open("k8s.key", "w") as cert:
                cert.write(decoded_key)


def main():
    """Entry point to this script."""
    if len(sys.argv) <= 1:
        print("Usage: gen_cert_file.py kubeconfig.yaml")
        sys.exit(1)
    generate_cert_and_key_files(sys.argv[1])


# Common Python's black magic
if __name__ == "__main__":
    main()
