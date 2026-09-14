# Progresso

## M0 — Foundation

Implementado:

- Monorepo com API Go e frontend React/TypeScript.
- Docker Compose com PostgreSQL/PostGIS, Redis, migrations, API e frontend.
- Logs JSON, timeouts HTTP, shutdown gracioso e endpoint de liveness.
- Migration inicial de extensões; rollback preserva extensões compartilhadas.
- Traduções en-GB e pt-BR desde o primeiro componente.
- Variáveis de ambiente documentadas e dados persistentes em volumes.

Validação concluída em 14/09/2026:

- `go test ./...`, `go vet ./...` e build da API passaram em Go 1.22 via container.
- `npm ci`, `npm run lint` (checagem de tipos) e `npm run build` passaram no build Docker.
- `docker compose up -d --build`: frontend, API, PostgreSQL e Redis saudáveis; migrate terminou com código zero.
- `/api/healthz` respondeu JSON com status `ok` através do Nginx.
- `schema_migrations`: versão 1, `dirty = false`.
- SQL up/down/up executado com sucesso no banco separado `workpin_migration_check`, preservado para inspeção.
- Corrigido healthcheck PostgreSQL para aguardar TCP, evitando liberar migrations durante o servidor temporário de inicialização.

M0 concluído. O frontend foi validado por compilação e HTTP; não houve teste visual em navegador. O Go local apresentou biblioteca padrão incompleta e não há npm local; as verificações usaram containers. Os serviços permanecem em execução para inspeção.

## M1 — Authentication + Multi-tenancy

Implementado:

- Migrations para `organizations`, `users`, `auth_otps` e `refresh_sessions`.
- Papéis `OWNER`, `ADMIN` e `WORKER`; telefone E.164 único e índice por organização.
- Solicitação e validação OTP, expiração, hash com salt, limite de tentativas e rate limit de solicitações.
- `MockSMSProvider` de desenvolvimento que registra o código OTP em log estruturado.
- Sessões opacas com tokens armazenados apenas como hash, cookies HttpOnly/SameSite e refresh token rotativo.
- Logout com revogação e `GET /me` autenticado.
- Middleware que deriva `organization_id` da sessão para uso obrigatório nas queries de domínio.

Pendência consciente: cadastro de usuários e organizações não tem rota administrativa até M2; o README contém SQL local de bootstrap. Redis ainda não é utilizado por M1.

Validação concluída em 14/09/2026:

- `go test ./...`, `go vet ./...` e build da API passaram em Go 1.22 via container.
- O build Docker executou `npm ci`, lint TypeScript e build do frontend sem erros.
- Migrations aplicadas até a versão 6, com `dirty = false`. A versão 6 corrige a validação E.164 da versão 3 sem modificar dados existentes.
- Fluxo HTTP validado: request OTP (204), verify OTP (200), `/me` autenticado (200), refresh rotativo (200), logout (204) e `/me` após logout (401).
- O proxy Nginx também retornou 401 esperado para `/api/me` sem cookie.

O teste HTTP usou organização e worker temporários, removidos ao fim da validação. O frontend foi validado por build e rota HTTP; não houve teste visual em navegador.

## Próximo milestone: M2

Workers, valor/hora, customers, service locations, PostGIS e geofence.

## M2 — Workers, Customers + Locations

Implementado:

- Workers vinculados a usuários `WORKER`, com telefone E.164, código opcional, estado e valor/hora `NUMERIC(12,2)`.
- Customers e service locations com CRUD administrativo.
- Coordenadas validadas no backend e pontos `geography(Point,4326)` no PostGIS, índice GIST e geofence por local.
- Endpoints `GET`, `POST` e `PATCH` para `/workers`, `/customers` e `/locations`, protegidos para `OWNER` e `ADMIN`.
- Toda leitura e escrita usa o `organization_id` da sessão; o customer também é validado antes de criar ou editar seu local.
- Edição de worker em transação, preservando consistência entre `users` e `workers`.

Validação concluída em 14/09/2026:

- Build, testes e `go vet` passaram no container Go.
- Migrations aplicadas até versão 9, sem estado sujo.
- CRUD autenticado testado para worker (£15.00/h), customer e local com coordenadas de Londres e geofence de 100m.

Os dados temporários do teste foram removidos. A interface administrativa completa será consolidada com as telas de workers, customers e jobs nos próximos milestones; a API já é utilizável para esses fluxos.

## Pendências posteriores

- PWA instalável e service worker.
- Modelos de negócio e demais migrations, conforme M1–M8.
- Configuração de produção com HTTPS e gestão de segredos.
