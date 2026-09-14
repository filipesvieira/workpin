# WorkPin — Plano de Implementação do MVP

> **Nome provisório do projeto:** WorkPin  
> **Categoria:** Field Workforce / Attendance Verification  
> **Objetivo:** controlar presença em campo com prova de localização, cálculo automático de horas e valor a pagar.

---

## 1. Visão do Produto

O **WorkPin** é uma plataforma SaaS multiempresa para controle de trabalhadores externos.

O primeiro caso de uso será uma empresa de limpeza no Reino Unido, mas o domínio deve permanecer genérico para atender futuramente:

- cleaners;
- promoters;
- freelancers;
- cuidadores;
- manutenção;
- técnicos externos;
- segurança;
- jardinagem;
- construção civil;
- equipes de campo em geral.

O problema central que o produto resolve é:

> **Comprovar quando, onde e por quanto tempo uma pessoa executou um serviço presencial, calculando automaticamente as horas trabalhadas e o valor devido.**

O sistema deve substituir processos baseados em WhatsApp, planilhas e cálculos manuais.

---

# 2. Objetivo do MVP

Construir uma aplicação web/PWA mobile-first com:

- autenticação segura por telefone;
- cadastro de funcionários;
- cadastro de valor/hora;
- cadastro de clientes;
- cadastro de locais de trabalho;
- geofence;
- criação de jobs/agendamentos;
- associação de trabalhadores aos jobs;
- check-in e check-out com GPS;
- validação da localização;
- horário oficial definido pelo servidor;
- cálculo automático de horas;
- cálculo automático do valor a pagar;
- dashboard administrativo;
- atualização em tempo real via WebSocket;
- mapa de check-ins/check-outs;
- relatórios;
- exportação CSV;
- correções administrativas auditadas;
- arquitetura multiempresa;
- internacionalização desde o início.

---

# 3. Princípios de Produto

O domínio NÃO deve conter regras específicas de limpeza.

Evitar:

```text
Cleaner
CleaningHouse
CleaningCheckIn
```

Preferir:

```text
Organization
Worker
Customer
ServiceLocation
Job
Assignment
Attendance
Timesheet
```

Princípios obrigatórios:

```text
multi-tenant
mobile-first
PWA
server-side timestamps
auditability
location verification
event-driven realtime
internationalization
timezone-aware
currency-aware
privacy-by-design
```

---

# 4. Stack

| Camada | Tecnologia |
|---|---|
| Frontend | React + TypeScript |
| Build | Vite |
| PWA | Vite PWA / Service Worker |
| Query/cache | TanStack Query |
| Forms | React Hook Form + Zod |
| Backend | Golang |
| HTTP | Chi ou net/http |
| Banco | PostgreSQL |
| Geo | PostGIS |
| Realtime | WebSocket |
| Cache / OTP | Redis |
| Mapas | MapLibre GL |
| Auth | OTP por telefone |
| Containers | Docker Compose |
| Proxy | Nginx ou Caddy |
| Logs | slog / JSON structured logs |
| Migrações | golang-migrate |
| Tradução | i18next |

Não utilizar Kubernetes no MVP.

---

# 5. Estrutura do Repositório

```text
/
├── apps/
│   └── web/
│       ├── src/
│       ├── public/
│       └── package.json
│
├── services/
│   └── api/
│       ├── cmd/
│       │   └── api/
│       ├── internal/
│       │   ├── auth/
│       │   ├── organization/
│       │   ├── worker/
│       │   ├── customer/
│       │   ├── location/
│       │   ├── job/
│       │   ├── assignment/
│       │   ├── attendance/
│       │   ├── timesheet/
│       │   ├── report/
│       │   ├── realtime/
│       │   ├── notification/
│       │   └── audit/
│       └── go.mod
│
├── migrations/
├── docker/
├── docs/
│   ├── PRODUCT.md
│   ├── ARCHITECTURE.md
│   ├── DATABASE.md
│   ├── API.md
│   └── SECURITY.md
│
├── docker-compose.yml
└── README.md
```

---

# 6. Multiempresa

Criar entidade:

```text
organizations
```

Campos:

```text
id UUID
name
country
timezone
currency
default_geofence_radius_meters
max_acceptable_gps_accuracy_meters
locale
created_at
updated_at
```

Exemplo:

```json
{
  "name": "Example Cleaning Ltd",
  "country": "GB",
  "timezone": "Europe/London",
  "currency": "GBP",
  "locale": "en-GB",
  "defaultGeofenceRadiusMeters": 100,
  "maxAcceptableGpsAccuracyMeters": 100
}
```

Todas as entidades aplicáveis devem carregar:

```text
organization_id
```

Toda query multiempresa deve obrigatoriamente utilizar o `organization_id`.

Nunca fazer:

```sql
SELECT * FROM workers WHERE id = $1;
```

Preferir:

```sql
SELECT *
FROM workers
WHERE id = $1
AND organization_id = $2;
```

---

# 7. Usuários e Permissões

Papéis iniciais:

```text
OWNER
ADMIN
WORKER
```

Tabela:

```text
users
```

Campos:

```text
id
organization_id
name
phone_e164
role
active
last_login_at
created_at
updated_at
```

Telefone no formato:

```text
E.164
```

Exemplo:

```text
+447700900123
```

---

# 8. Autenticação

Fluxo:

```text
telefone
  ↓
POST /auth/request-otp
  ↓
validar usuário previamente cadastrado
  ↓
gerar OTP
  ↓
enviar SMS
  ↓
POST /auth/verify-otp
  ↓
sessão autenticada
```

Criar abstração:

```go
type SMSProvider interface {
    SendOTP(ctx context.Context, phone, code string) error
}
```

No desenvolvimento utilizar:

```text
MockSMSProvider
```

Preparar para provedores futuros como:

- Twilio;
- AWS SNS;
- MessageBird;
- outros.

Implementar:

- rate limiting;
- expiração de OTP;
- limite de tentativas;
- refresh token rotativo;
- logout;
- revogação de sessões;
- cookies `HttpOnly`, `Secure`, `SameSite` quando aplicável.

---

# 9. Worker

Criar tabela:

```text
workers
```

Campos:

```text
id
organization_id
user_id
employee_code
default_hourly_rate
active
created_at
updated_at
```

O valor/hora deve usar tipo decimal:

```sql
NUMERIC(12,2)
```

Nunca usar `float` para valores financeiros.

---

# 10. Valor por Hora

Cada trabalhador terá:

```text
default_hourly_rate
```

Exemplo:

```text
Maria
£15.00/hour
```

Um assignment pode opcionalmente sobrescrever este valor:

```text
job_assignments.hourly_rate_override
```

Regra:

```text
Assignment hourly_rate_override
        ↓
se NULL
        ↓
Worker default_hourly_rate
```

Exemplo:

```text
Maria normalmente:
£15/h

Job especial:
£18/h
```

Ao iniciar um attendance, salvar um snapshot do valor/hora utilizado.

Isto garante que alterações futuras no valor padrão não alterem registros históricos.

---

# 11. Customers

Tabela:

```text
customers
```

Campos:

```text
id
organization_id
name
phone
email
notes
active
created_at
updated_at
```

O cliente não terá login no MVP.

---

# 12. Service Locations

Tabela:

```text
service_locations
```

Campos:

```text
id
organization_id
customer_id
name

address_line_1
address_line_2
city
postal_code
country

latitude
longitude

geofence_radius_meters

notes

active

created_at
updated_at
```

Utilizar PostGIS:

```text
geography(Point,4326)
```

Uma pessoa/empresa pode possuir múltiplos locais.

---

# 13. Jobs

Tabela:

```text
jobs
```

Campos:

```text
id
organization_id
customer_id
service_location_id

title
description

scheduled_start_at
scheduled_end_at

status

created_at
updated_at
```

Status:

```text
DRAFT
SCHEDULED
IN_PROGRESS
COMPLETED
CANCELLED
```

---

# 14. Job Assignments

Separar o serviço de quem irá realizá-lo.

Tabela:

```text
job_assignments
```

Campos:

```text
id
organization_id
job_id
worker_id
hourly_rate_override
status
created_at
updated_at
```

Um job pode possuir múltiplos workers.

---

# 15. Attendance Session

Tabela:

```text
attendance_sessions
```

Representa:

```text
1 worker
+
1 assignment
+
1 período de trabalho
```

Campos:

```text
id
organization_id
job_id
assignment_id
worker_id

checkin_at
checkout_at

worked_seconds

hourly_rate
calculated_amount

status

approved_at
approved_by

created_at
updated_at
```

---

# 16. Attendance Events

Não armazenar apenas o estado final.

Criar:

```text
attendance_events
```

Campos:

```text
id
organization_id
attendance_session_id
worker_id

type

occurred_at

latitude
longitude
accuracy_meters

expected_latitude
expected_longitude

distance_from_location_meters

verification_status

device_timestamp
server_timestamp

metadata JSONB

created_at
```

Tipos iniciais:

```text
CHECK_IN
CHECK_OUT
ADMIN_EDIT
ADMIN_APPROVAL
```

---

# 17. Horário Oficial

O horário oficial do check-in/check-out deve ser definido pelo backend.

Usar:

```go
time.Now().UTC()
```

O frontend pode enviar:

```text
device_timestamp
```

somente para auditoria.

Nunca utilizá-lo como horário oficial.

---

# 18. Geolocalização

No clique de check-in/check-out utilizar a Geolocation API.

Preferencialmente:

```javascript
navigator.geolocation.watchPosition(...)
```

Obter múltiplas leituras por alguns segundos.

Exemplo:

```text
±124m
±58m
±21m
±8m
```

Selecionar a melhor leitura aceitável.

Payload:

```json
{
  "assignmentId": "...",
  "latitude": 51.123,
  "longitude": -0.123,
  "accuracyMeters": 9,
  "deviceTimestamp": "..."
}
```

---

# 19. Fluxo do Check-in

```text
autenticar
↓
identificar organization
↓
identificar worker
↓
validar assignment
↓
validar vínculo com worker
↓
validar job
↓
buscar ServiceLocation
↓
calcular distância via PostGIS
↓
avaliar GPS accuracy
↓
classificar verificação
↓
capturar horário do servidor
↓
resolver hourly_rate
↓
criar attendance_session
↓
criar attendance_event
↓
commit transaction
↓
publicar evento realtime
```

---

# 20. Estados de Verificação

Criar:

```text
VERIFIED
OUTSIDE_GEOFENCE
LOW_ACCURACY
REVIEW_REQUIRED
MANUAL
```

Regra inicial:

```text
distance <= geofence_radius
AND
accuracy <= max_acceptable_accuracy
```

Resultado:

```text
VERIFIED
```

Caso contrário, marcar a razão correspondente.

Não bloquear obrigatoriamente o registro.

Um GPS ruim ou posição externa pode continuar sendo registrado, mas deve ser sinalizado para revisão.

---

# 21. Checkout

Endpoint:

```http
POST /api/v1/attendance/{id}/checkout
```

Repetir validações de:

```text
GPS
accuracy
distance
server timestamp
verification
```

Depois calcular:

```text
worked_seconds = checkout_at - checkin_at
```

---

# 22. Cálculo de Horas

Salvar internamente:

```text
worked_seconds
```

Exemplo:

```text
8100 segundos
```

Representação:

```text
2h15m
```

Conversão para cálculo financeiro:

```text
8100 / 3600 = 2.25 h
```

---

# 23. Cálculo Automático do Valor

Exemplo:

```text
Check-in: 09:00
Checkout: 11:15

Worked:
2h15m

Hourly rate:
£15
```

Cálculo:

```text
2.25 × £15 = £33.75
```

O backend deve calcular oficialmente:

```text
calculated_amount
```

O frontend apenas exibe.

Não implementar arredondamento de minutos no MVP.

Usar o tempo real.

---

# 24. Dashboard Administrativo

Página:

```text
/dashboard
```

Cards:

```text
Workers Today
Currently Working
Upcoming
Completed
Needs Attention
Hours Today
Estimated Pay Today
```

Exemplo:

```text
TODAY

12 Workers
7 Working
3 Upcoming
2 Completed

1 Needs Attention

Hours Today
42h17m

Estimated Pay
£634.25
```

---

# 25. Live Activity

Exemplo:

```text
09:15 🔴 Maria checked out
      Mrs Smith
      2h13m
      £33.25

09:02 🟢 Maria checked in
      Mrs Smith
      14m from location
      VERIFIED

08:58 🟢 Ana checked in
      Mrs Brown
      VERIFIED

08:44 ⚠ Julia checked in
      412m from location
```

---

# 26. WebSocket

Criar conexão autenticada:

```text
GET /ws
```

O backend determina a partir da sessão:

```text
user_id
organization_id
role
```

O frontend nunca deve definir arbitrariamente o `organization_id` do canal.

---

# 27. Realtime Hub

Estrutura lógica:

```text
Hub
├── orgA
│   ├── connection1
│   └── connection2
│
└── orgB
    └── connection1
```

Evento de uma empresa jamais pode ser enviado a outra.

---

# 28. Eventos Realtime

Envelope:

```json
{
  "type": "attendance.checked_in",
  "eventId": "...",
  "organizationId": "...",
  "occurredAt": "...",
  "payload": {}
}
```

Eventos iniciais:

```text
attendance.checked_in
attendance.checked_out
attendance.review_required
attendance.updated

job.created
job.updated
job.cancelled
```

---

# 29. Evento de Check-in

```json
{
  "type": "attendance.checked_in",
  "payload": {
    "attendanceId": "...",
    "worker": {
      "id": "...",
      "name": "Maria Silva"
    },
    "location": {
      "id": "...",
      "name": "Mrs Smith"
    },
    "checkinAt": "2026-09-14T08:02:17Z",
    "distanceMeters": 14.2,
    "accuracyMeters": 8,
    "verificationStatus": "VERIFIED"
  }
}
```

---

# 30. Evento de Checkout

```json
{
  "type": "attendance.checked_out",
  "payload": {
    "workerName": "Maria Silva",
    "locationName": "Mrs Smith",
    "checkoutAt": "...",
    "workedSeconds": 8100,
    "hourlyRate": "15.00",
    "calculatedAmount": "33.75",
    "currency": "GBP"
  }
}
```

---

# 31. Event Bus Interno

Não acoplar attendance diretamente ao WebSocket.

Evitar:

```go
func CheckIn() {
    db.Save()
    websocket.Send()
}
```

Criar:

```go
type EventPublisher interface {
    Publish(ctx context.Context, event Event) error
}
```

Fluxo:

```text
AttendanceService
       ↓
Database transaction
       ↓
Domain event
       ↓
EventPublisher
       ↓
Realtime
```

No futuro, o mesmo barramento poderá alimentar:

```text
WebSocket
Web Push
Email
Webhook
Audit
```

---

# 32. Dashboard React Realtime

Após login administrativo:

```text
React
  ↓
abre WebSocket
```

Ao receber:

```text
attendance.checked_in
```

atualizar:

```text
TanStack Query cache
```

e mostrar toast.

Exemplo:

```text
Maria checked in

Mrs Smith
09:02

✓ Location verified
```

---

# 33. Mapa Administrativo

Página:

```text
/dashboard/map
```

Utilizar MapLibre.

Mostrar:

```text
🏠 Service Location
🟢 Check-in
🔴 Check-out
```

Mostrar também:

```text
geofence
```

Ao clicar:

```text
Maria Silva

CHECK-IN
09:02

GPS accuracy
8m

Distance
14.2m

Status
VERIFIED
```

---

# 34. Tela do Worker

Mobile-first.

Exemplo:

```text
Good morning, Maria

TODAY

09:00 - 11:00
Mrs Smith

10 High Street

[ Open directions ]

You are near the location

[ CHECK IN ]
```

Após check-in:

```text
✅ CHECKED IN

09:02

Working

01:34:52

[ CHECK OUT ]
```

Manter esta interface extremamente simples.

---

# 35. Tela Admin — Workers

Página:

```text
/workers
```

Permitir:

```text
Create Worker
Edit Worker
Deactivate Worker
```

Campos:

```text
Name
Phone
Default hourly rate
Status
```

---

# 36. Customers

Página:

```text
/customers
```

CRUD de:

```text
Customer
Service Locations
```

---

# 37. Jobs

Página:

```text
/jobs
```

Permitir criar:

```text
Customer
Location
Date
Start
End
Worker(s)
Hourly rate override optional
Notes
```

---

# 38. Relatórios

Página:

```text
/reports/timesheets
```

Filtros:

```text
date range
worker
customer
location
status
```

Exemplo:

| Worker | Location | Date | Start | End | Hours | Rate | Amount |
|---|---|---|---|---|---:|---:|---:|
| Maria | Smith | 14/09 | 09:02 | 11:17 | 2h15 | £15 | £33.75 |
| Ana | Brown | 14/09 | 08:57 | 12:10 | 3h13 | £14 | £45.03 |

Rodapé:

```text
Total Hours
32h42m

Estimated Pay
£476.84
```

---

# 39. Resumo por Worker

Exemplo:

```text
Maria Silva

Period
01/09 → 30/09

Jobs
18

Hours
42h35m

Estimated Pay
£638.75
```

Se houver rates diferentes, detalhar por faixa.

---

# 40. Exportação

MVP:

```text
CSV
```

Campos:

```text
Worker
Customer
Location
Check-in
Checkout
Worked time
Hourly rate
Amount
Check-in status
Checkout status
```

PDF pode ser adicionado posteriormente.

---

# 41. Correções Administrativas

O administrador pode corrigir:

```text
check-in
checkout
```

mas toda alteração deve exigir:

```text
reason
```

Exemplo:

```text
Original checkout:
17:23

New:
17:00

Reason:
Worker forgot to checkout at the correct time.
```

---

# 42. Audit Log

Tabela:

```text
audit_logs
```

Campos:

```text
id
organization_id
actor_user_id
entity_type
entity_id
action
before_data JSONB
after_data JSONB
reason
created_at
```

Nunca sobrescrever silenciosamente um attendance.

---

# 43. Recalcular Após Correção

Ao alterar check-in ou checkout:

```text
recalcular worked_seconds
recalcular calculated_amount
```

Exemplo:

```text
Antes:
3h
£45

Depois:
2h30
£37.50
```

O audit log mantém a alteração.

---

# 44. PostGIS

Criar extensão:

```sql
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
```

Utilizar PostGIS para cálculo oficial de distância.

Não confiar na distância calculada pelo frontend.

---

# 45. Índices

Criar índices para:

```text
organization_id
worker_id
job_id
attendance_session_id
scheduled_start_at
checkin_at
checkout_at
```

PostGIS:

```text
GIST(location)
```

---

# 46. Privacidade

Não implementar rastreamento contínuo.

Capturar localização somente em:

```text
CHECK-IN
CHECK-OUT
```

Não armazenar:

```text
rotas
histórico permanente de movimento
background tracking
```

Mensagem ao usuário:

> Your location is only collected when you check in or check out. We do not continuously track your location.

---

# 47. PWA

Implementar:

```text
manifest
icons
service worker
installable app
responsive design
safe areas
mobile-first UX
```

Suportar, quando disponível:

```text
Android
iOS
Desktop
```

---

# 48. Internacionalização

Não colocar strings fixas diretamente nos componentes.

Evitar:

```tsx
<h1>Check In</h1>
```

Usar:

```tsx
t("attendance.checkIn")
```

Preparar:

```text
en-GB
pt-BR
```

Idioma inicial da operação:

```text
en-GB
```

---

# 49. Migrations

Ordem:

```text
001_extensions
002_organizations
003_users
004_workers
005_customers
006_service_locations
007_jobs
008_job_assignments
009_attendance_sessions
010_attendance_events
011_audit_logs
012_auth_otps
013_refresh_sessions
```

---

# 50. APIs

```text
POST   /auth/request-otp
POST   /auth/verify-otp
POST   /auth/refresh
POST   /auth/logout

GET    /me

GET    /workers
POST   /workers
GET    /workers/:id
PATCH  /workers/:id

GET    /customers
POST   /customers
PATCH  /customers/:id

GET    /locations
POST   /locations
PATCH  /locations/:id

GET    /jobs
POST   /jobs
GET    /jobs/:id
PATCH  /jobs/:id

GET    /assignments/my/today

POST   /attendance/check-in
POST   /attendance/:id/check-out

GET    /attendance
GET    /attendance/:id

PATCH  /attendance/:id/admin-correction

GET    /reports/timesheets
GET    /reports/timesheets/export

GET    /dashboard/today

WS     /ws
```

---

# 51. Milestones

## M0 — Foundation

Implementar:

- estrutura do repositório;
- Docker Compose;
- Go API;
- React;
- PostgreSQL;
- PostGIS;
- Redis;
- migrations;
- variáveis de ambiente;
- logging;
- healthcheck.

Aceite:

```text
docker compose up
```

deve subir:

```text
frontend
api
postgres
redis
```

---

## M1 — Authentication + Multi-tenancy

Implementar:

- Organization;
- User;
- roles;
- OTP;
- sessões;
- refresh token;
- middleware;
- tenant isolation.

Aceite:

```text
Admin consegue logar via telefone.
Worker consegue logar via telefone.
Empresa A não acessa dados da empresa B.
```

---

## M2 — Workers + Customers + Locations

Implementar:

```text
Workers
Default hourly rate
Customers
Service Locations
PostGIS
Geofence
```

Aceite:

```text
Admin cadastra Maria
£15/h

Cadastra Mrs Smith

Cadastra endereço
coordenadas
geofence de 100m
```

---

## M3 — Jobs + Assignment

Implementar:

```text
Jobs
Assignments
Worker schedule
```

Aceite:

```text
Admin cria:

Maria
Mrs Smith
14/09
09:00 → 11:00
```

e Maria visualiza na PWA.

---

## M4 — Check-in / Checkout

Implementar:

```text
GPS
accuracy
PostGIS distance
server timestamp
geofence validation
attendance session
attendance events
```

Aceite:

Check-in registra:

```text
server time
GPS
accuracy
distance
verification
```

Checkout calcula:

```text
worked_seconds
```

---

## M5 — Automatic Pay

Implementar:

```text
Assignment hourly override
       ↓
Worker hourly rate
       ↓
Attendance rate snapshot
       ↓
worked_seconds
       ↓
calculated_amount
```

Aceite:

```text
Maria

2h15m

£15/h

£33.75
```

calculados automaticamente.

---

## M6 — Realtime

Implementar:

```text
WebSocket
organization rooms
attendance events
dashboard cache updates
toasts
live activity
```

Aceite:

Admin mantém dashboard aberto.

Maria faz check-in.

Sem atualizar a página aparece:

```text
Maria checked in
09:02
Mrs Smith
Verified
```

O mesmo deve ocorrer no checkout.

---

## M7 — Dashboard + Map

Implementar:

```text
Today cards
Live workers
Completed
Upcoming
Exceptions
Map
```

Mapa:

```text
expected location
check-in
checkout
geofence
```

---

## M8 — Reports + Audit

Implementar:

```text
timesheets
worker reports
date range
payment totals
CSV
admin corrections
audit logs
```

Esta fase fecha o MVP comercial.

---

# 52. Fora do Escopo do MVP

Não implementar ainda:

```text
payroll integration
invoice generation
route tracking
continuous GPS
native Android
native iOS
facial recognition
NFC
QR code
chat
shift swapping
vacation management
expenses
mileage
overtime rules
complex rounding
customer portal
worker ratings
AI
Kubernetes
microservices
```

---

# 53. Cenário de Aceite Final

O MVP estará funcional quando este fluxo funcionar de ponta a ponta:

```text
Admin cadastra empresa
↓
Admin cadastra Maria
£15/hour
↓
Admin cadastra Mrs Smith
↓
Admin cadastra residência
com coordenadas e geofence
↓
Admin cria Job
09:00 - 11:00
↓
Associa Maria
↓
Maria entra pela PWA
↓
Vê serviço do dia
↓
Chega à casa
↓
Faz Check-in
↓
Servidor registra:
09:02
GPS ±8m
14m da residência
VERIFIED
↓
Dashboard atualiza em tempo real
↓
Maria trabalha
↓
Faz Checkout às 11:17
↓
Sistema calcula:
2h15m
↓
Rate:
£15/hour
↓
Pay:
£33.75
↓
Admin vê imediatamente
↓
Registro aparece no mapa
↓
Dados entram no relatório
↓
CSV pode ser exportado
```

---

# 54. Regras Obrigatórias para o Codex

O Codex deve trabalhar milestone por milestone.

Ao concluir cada milestone:

1. executar testes;
2. executar lint;
3. compilar frontend;
4. compilar backend;
5. validar migrations;
6. atualizar documentação;
7. registrar o que foi concluído;
8. registrar pendências;
9. não avançar deixando erros conhecidos.

Não tentar implementar todo o projeto de uma vez.

Mocks são permitidos somente para integrações externas, como SMS.

Não criar regras específicas para cleaners.

Toda operação deve respeitar `organization_id`.

Toda informação financeira oficial deve ser calculada pelo backend.

Todo horário oficial de attendance deve vir do servidor.

Toda alteração administrativa deve produzir audit log.

Nunca implementar rastreamento contínuo do trabalhador.

---

# 55. Prioridade Recomendada

Primeiro objetivo:

```text
M0 → M5
```

Ao final já teremos:

```text
login
workers
valor/hora
customers
locations
jobs
GPS
check-in
checkout
horas trabalhadas
pagamento automático
```

Isto já substitui:

```text
WhatsApp
+
planilha
+
cálculo manual
```

Depois:

```text
M6 realtime
M7 dashboard/map
M8 reports/audit
```

transformam o núcleo em produto comercial.

---

# 56. Proposta de Valor

O WorkPin deve responder automaticamente:

```text
Quem trabalhou?
+
Onde trabalhou?
+
Quando entrou?
+
Quando saiu?
+
Quantas horas trabalhou?
+
Quanto deve receber?
```

O produto não deve ser tratado apenas como um relógio de ponto.

A visão é:

> **uma central operacional para equipes externas, com presença verificada, horas calculadas e custo de trabalho automatizado.**

