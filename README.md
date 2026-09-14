# WorkPin

Plataforma para presença verificada e gestão de equipes externas. O desenvolvimento segue os milestones de [WORKPIN_IMPLEMENTATION_PLAN.md](WORKPIN_IMPLEMENTATION_PLAN.md).

## Execução local

Requisito: Docker com Compose v2 ou superior.

```bash
docker compose up -d --build
docker compose ps -a
```

Frontend: http://localhost:8080. API: http://localhost:8081/healthz.
O proxy do frontend também oferece `/api/healthz`.

Os valores padrão são exclusivos para desenvolvimento local. Para personalizar portas e credenciais, copie `.env.example` para `.env`. Senhas usadas na URL de migrations precisam ser codificadas para URL se contiverem caracteres reservados. PostgreSQL e Redis não publicam portas no host. Não exponha esta configuração diretamente à internet.

```bash
docker compose logs -f api
docker compose stop
```

Os dados persistem em volumes. O serviço `migrate` aplica migrations antes da API iniciar; sua saída com código zero é esperada. `healthz` é liveness; a API valida PostgreSQL antes de iniciar. Redis ainda está reservado para etapas futuras.

## Validação

Com Go 1.22+ instalado corretamente:

```bash
cd services/api
go test ./...
go vet ./...
go build ./cmd/api
```

Com Node 22 e npm:

```bash
cd apps/web
npm ci
npm run lint
npm run build
```

O lint inicial do frontend é a verificação estrita de tipos TypeScript. O build Docker também executa lint e build do frontend.

## Escopo atual

M0 fornece fundação, API HTTP, frontend React/TypeScript, PostgreSQL/PostGIS, Redis, migrations e healthchecks.

M1 acrescenta organizações, usuários com papéis `OWNER`, `ADMIN` e `WORKER`, login OTP, sessões opacas rotativas e `GET /me`. O SMS é mockado no desenvolvimento: o código OTP aparece somente nos logs JSON da API.

Para experimentar o fluxo local, crie uma organização e usuário de teste. Este SQL é para desenvolvimento local; o cadastro administrativo será entregue no M2.

```bash
docker compose exec postgres psql -U workpin -d workpin
```

```sql
INSERT INTO organizations (name, country, timezone, currency)
VALUES ('Example Ltd', 'GB', 'Europe/London', 'GBP')
RETURNING id;

INSERT INTO users (organization_id, name, phone_e164, role)
VALUES ('<organization-id>', 'Maria Silva', '+447700900123', 'WORKER');
```

Solicite o código e consulte os logs da API:

```bash
curl -i -X POST http://localhost:8081/auth/request-otp -H 'Content-Type: application/json' -d '{"phone":"+447700900123"}'
docker compose logs api --tail=20
```

Use o campo `code` emitido pelo `MockSMSProvider` para chamar `/auth/verify-otp`. Ele envia os cookies `HttpOnly` de acesso e refresh. Nunca use o provedor mock em produção.

A próxima etapa é M2. Veja [docs/PROGRESS.md](docs/PROGRESS.md).
