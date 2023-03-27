#!/bin/sh -eu

cd /app

_term() {
  echo "Caught SIGTERM signal!"
  kill -TERM "$child" 2>/dev/null
}

trap _term TERM INT

if [ -f "/app/.env" ]; then
    . /app/.env
fi

LEVEL=debug LISTEN=:8080 go run . &

child=$!
wait "$child"
