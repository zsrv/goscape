#!/usr/bin/env bash
# deploy/bundled/scripts/fake-login.sh
# Trigger one PlayerLogin against the bundled goscape login service via grpcurl.
# Requires: grpcurl (https://github.com/fullstorydev/grpcurl).
set -euo pipefail

LOGIN_ADDR="${LOGIN_ADDR:-localhost:2004}"

# Field names match proto/login/login.proto PlayerLoginRequest.
grpcurl -plaintext -d '{
  "node_id": 1,
  "profile": "main",
  "node_members": true,
  "username": "demo",
  "password": "demo",
  "uid": 1,
  "remote_address": "127.0.0.1",
  "reconnecting": false,
  "has_save": false
}' "$LOGIN_ADDR" login.v1.LoginService/PlayerLogin
