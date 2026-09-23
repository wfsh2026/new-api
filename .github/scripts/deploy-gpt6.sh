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
sudo docker exec -i postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1' <<'SQL'
BEGIN;
UPDATE channels SET models = models || ',gpt-6-sol'
WHERE id IN (12,13) AND type = 57 AND status = 1
AND 'gpt-6-astra' = ANY(string_to_array(models, ','))
AND NOT ('gpt-6-sol' = ANY(string_to_array(models, ',')));
UPDATE channels SET models = models || ',gpt-6-luna'
WHERE id IN (12,13) AND type = 57 AND status = 1
AND 'gpt-6-astra' = ANY(string_to_array(models, ','))
AND NOT ('gpt-6-luna' = ANY(string_to_array(models, ',')));
INSERT INTO abilities ("group",model,channel_id,enabled,priority,weight,tag)
SELECT a."group",m.model,a.channel_id,a.enabled,a.priority,a.weight,a.tag
FROM abilities a CROSS JOIN (VALUES ('gpt-6-sol'),('gpt-6-luna')) AS m(model)
JOIN channels c ON c.id = a.channel_id
WHERE a.channel_id IN (12,13) AND a.model = 'gpt-6-astra' AND c.type = 57 AND c.status = 1
ON CONFLICT DO NOTHING;
COMMIT;
SELECT channel_id,model,enabled FROM abilities WHERE model IN ('gpt-6-sol','gpt-6-luna') ORDER BY channel_id,model;
SQL
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
sudo python3 <<'PY'
import json,subprocess,urllib.request,urllib.error
query="SELECT key FROM tokens WHERE user_id=1 AND status=1 AND deleted_at IS NULL AND (expired_time=-1 OR expired_time>extract(epoch from now())) ORDER BY id LIMIT 1"
command=['docker','exec','postgres','sh','-c','psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c "$1"','sh',query]
result=subprocess.run(command,capture_output=True,text=True,check=True)
token=result.stdout.strip()
assert token, 'No active owner API token available for validation'
if not token.startswith('sk-'): token='sk-'+token
headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'}
request=urllib.request.Request('http://127.0.0.1:3000/v1/models',headers=headers)
with urllib.request.urlopen(request,timeout=20) as response: models=json.load(response)
available={m['id'] for m in models['data']}
failures=[]
for model in ['gpt-6-sol','gpt-6-luna']:
 print('Model advertised:',model,model in available)
 assert model in available
 payload={'model':model,'instructions':'Reply with exactly OK.','input':[{'role':'user','content':'Reply with exactly OK.'}],'stream':True,'store':False,'reasoning':{'effort':'low'}}
 body=json.dumps(payload).encode()
 request=urllib.request.Request('http://127.0.0.1:3000/v1/responses',data=body,headers=headers)
 completed=False
 try:
  with urllib.request.urlopen(request,timeout=90) as response:
   for raw in response:
    if not raw.startswith(b'data: '): continue
    data=raw[6:].strip()
    if data==b'[DONE]':continue
    event=json.loads(data)
    if event.get('type')=='response.completed':
     completed=True
     print('Live response completed:',model,'returned model:',event.get('response',{}).get('model'))
    elif event.get('type') in ['error','response.failed']:
     print('Live response error:',model,event.get('type'))
 except urllib.error.HTTPError as error:
  print('Live response HTTP status:',model,error.code)
 except Exception as error:
  print('Live response exception:',model,type(error).__name__)
 if not completed: failures.append(model)
assert not failures, 'Live validation failed for: '+','.join(failures)
PY
