# Arquitetura inicial

- `apps/web`: React e TypeScript, build Vite, traduções i18next. Nginx serve os assets e encaminha `/api/` para a API.
- `services/api`: Go com `net/http`, logs estruturados `slog`, timeouts e encerramento gracioso.
- `migrations`: SQL versionado executado com golang-migrate. M0 instala PostGIS e pgcrypto. As tabelas serão adicionadas no milestone que as utiliza.
- `docker`: imagens de runtime e configuração do proxy.

Compose aguarda PostgreSQL saudável, migrations concluídas e Redis saudável antes de iniciar a API. O frontend aguarda o healthcheck da API. A rede interna não expõe banco nem Redis ao host.

`/healthz` é liveness. A API inicializa conexão PostgreSQL antes de aceitar tráfego. Redis permanece reservado para rate limiting distribuído e integrações futuras; o limite de OTP da M1 está no PostgreSQL para manter a verificação consistente no MVP inicial.

## Autenticação e tenant

`POST /auth/request-otp` aceita telefone E.164 e não revela se ele corresponde a um usuário ativo. O código expira em dez minutos, possui salt e hash, aceita no máximo cinco tentativas e há no máximo cinco solicitações por usuário a cada quinze minutos. O mock de desenvolvimento escreve o OTP nos logs estruturados.

Após `POST /auth/verify-otp`, a API grava somente hashes de tokens de acesso e refresh. O refresh revoga a sessão anterior e emite novos dois tokens. Cookies são `HttpOnly` e `SameSite=Lax`; `AUTH_COOKIE_SECURE=true` é necessário atrás de HTTPS. Logout revoga a sessão que contém o token de acesso.

O middleware resolve o usuário, papel e `organization_id` da sessão. Handlers de domínio devem obter a identidade por `server.UserFromContext`; nenhuma rota autenticada aceitará `organization_id` do cliente. Cada query futura deve incluir esse ID.

O Compose é destinado ao desenvolvimento local. HTTPS, segredos gerenciados e política operacional de produção ficam pendentes.
