#!/usr/bin/env sh
set -eu

IMAGE="${PAMIE_IMAGE:-pamie:smoke}"
CONTAINER="pamie-smoke-$$"
TOKEN="pamie-smoke-token"

cleanup() {
	docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
}

if ! command -v docker >/dev/null 2>&1; then
	echo "docker not found; skipping smoke test"
	exit 0
fi

if ! docker info >/dev/null 2>&1; then
	echo "docker daemon unavailable; skipping smoke test"
	exit 0
fi

if ! command -v curl >/dev/null 2>&1; then
	echo "curl not found; skipping smoke test"
	exit 0
fi

docker build --build-arg VERSION=smoke -t "$IMAGE" .

trap cleanup EXIT INT TERM
docker run -d --rm \
	--name "$CONTAINER" \
	-p 127.0.0.1::8080 \
	-e PAMIE_TOKEN="$TOKEN" \
	-e PAMIE_TOKEN_ID=smoke \
	-e PAMIE_TOKEN_SCOPES=all \
	"$IMAGE" >/dev/null

port=""
i=0
while [ "$i" -lt 30 ]; do
	port="$(docker port "$CONTAINER" 8080/tcp 2>/dev/null | sed -n 's/.*:\([0-9][0-9]*\)$/\1/p' | head -n 1 || true)"
	if [ -n "$port" ] && curl -fsS "http://127.0.0.1:$port/health" >/dev/null 2>&1; then
		break
	fi
	i=$((i + 1))
	sleep 1
done

if [ -z "$port" ]; then
	echo "container did not publish an HTTP port" >&2
	docker logs "$CONTAINER" >&2 || true
	exit 1
fi

curl -fsS "http://127.0.0.1:$port/health" >/dev/null
curl -fsS "http://127.0.0.1:$port/ready" >/dev/null

curl -fsS \
	-X POST "http://127.0.0.1:$port/mcp" \
	-H "Authorization: Bearer $TOKEN" \
	-H "Content-Type: application/json" \
	-d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' >/dev/null

status="$(curl -sS -o /dev/null -w "%{http_code}" \
	-X POST "http://127.0.0.1:$port/mcp" \
	-H "Content-Type: application/json" \
	-d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' || true)"
if [ "$status" != "401" ]; then
	echo "unauthenticated /mcp status = $status, want 401" >&2
	exit 1
fi

echo "docker smoke passed for $IMAGE on 127.0.0.1:$port"
