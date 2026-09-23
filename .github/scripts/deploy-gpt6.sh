#!/usr/bin/env bash
set -euo pipefail
umask 077
operation=$1
image=$2
cd /home/ubuntu/my-new-api
sudo docker ps --format '{{.Names}} {{.Image}} {{.Status}}'
sudo docker inspect new-api --format '{{range .Config.Env}}{{println .}}{{end}}' | grep '^MAX_REQUEST_BODY_MB='
free -m
df -h /
if [ "$operation" = inspect ]; then
  sudo docker exec postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT id,type,status,name,models FROM channels ORDER BY id;"'
  exit 0
fi
[ "$operation" = deploy ]
sudo docker pull "$image"
stamp=$(date +%Y%m%d-%H%M%S)
backup="backups/gpt6-$stamp"
mkdir -p "$backup"
sudo cp -p docker-compose.yml "$backup/docker-compose.yml"
sudo docker exec postgres sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > "$backup/database.sql"
test -s "$backup/database.sql"
previous=$(sudo docker inspect new-api --format '{{.Config.Image}}')
printf 'Previous image: %s\nBackup: %s\n' "$previous" "$backup"
sudo python3 - "$image" <<'PY'
import pathlib,re,sys
p=pathlib.Path('docker-compose.yml')
source=p.read_text()
pattern=r'(?m)^(\s+image:\s*)ghcr\.io/wfsh2026/new-api:[^\s#]+([ \t]*)$'
replacement=r'\g<1>'+sys.argv[1]+r'\g<2>'
updated,count=re.subn(pattern,replacement,source)
assert count==1, 'Expected exactly one New API image entry'
p.write_text(updated)
PY
if ! sudo docker compose up -d --no-deps --pull never --no-build --wait --wait-timeout 120 new-api; then
  sudo cp -p "$backup/docker-compose.yml" docker-compose.yml
  sudo docker compose up -d --no-deps --pull never --no-build new-api
  echo 'Deployment failed; restored previous image configuration.'
  exit 1
fi
sudo docker inspect new-api --format 'Image={{.Config.Image}} Health={{.State.Health.Status}} StartedAt={{.State.StartedAt}}'
sudo docker inspect new-api --format '{{range .Config.Env}}{{println .}}{{end}}' | grep '^MAX_REQUEST_BODY_MB='
curl --fail --silent http://127.0.0.1:3000/api/status | python3 -c 'import json,sys; d=json.load(sys.stdin); assert d.get("success"); print("API status healthy, version:",d.get("data",{}).get("version"))'
sudo docker inspect postgres redis --format '{{.Name}} StartedAt={{.State.StartedAt}}'
