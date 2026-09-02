# Car Wash API

[![CI](https://github.com/Abdurahim00/go-carwash-api/actions/workflows/ci.yml/badge.svg)](https://github.com/Abdurahim00/go-carwash-api/actions/workflows/ci.yml)

Small REST API built in Go to practice backend development with a stack similar to Mjuk Biltvätt's environment.

## Features
- Create wash jobs
- List wash jobs (filter by status and/or registration number)
- Get a single wash job
- Track when each wash was created and last updated
- Update wash status with a simple lifecycle (`queued -> in_progress -> done`, or `cancelled`)
- Delete wash jobs
- SQLite persistence
- REST API using only the Go standard library (`net/http`)
- Docker support
- Basic tests

## Tech
Go, SQLite, SQL, Docker

No frameworks. Routing uses the Go 1.22+ `net/http` method/pattern mux, and the SQLite driver is
[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite), which is pure Go (no cgo, no C compiler needed).

## Run

```bash
go run .
```

The server starts on `http://localhost:8080` and creates `carwash.db` in the current directory.

| Env var   | Default       | Description                  |
|-----------|---------------|------------------------------|
| `PORT`    | `8080`        | Port to listen on            |
| `DB_PATH` | `carwash.db`  | Path to the SQLite database  |

## Test

```bash
go test ./...
```

## Docker

```bash
docker build -t go-carwash-api .
docker run -p 8080:8080 -v carwash-data:/app/data go-carwash-api
```

## Endpoints

| Method   | Path                    | Description                         |
|----------|-------------------------|-------------------------------------|
| `POST`   | `/washes`               | Create a wash job (status `queued`) |
| `GET`    | `/washes`               | List all wash jobs                  |
| `GET`    | `/washes?status=queued` | List wash jobs with a given status  |
| `GET`    | `/washes?registration_number=ABC123` | List wash jobs for one car (normalised like on create) |
| `GET`    | `/washes/{id}`          | Get one wash job                    |
| `PATCH`  | `/washes/{id}/status`   | Change the status of a wash job     |
| `DELETE` | `/washes/{id}`          | Delete a wash job                   |
| `GET`    | `/health`               | Health check                        |

## Example

### Create a wash

`POST /washes`

```json
{
  "registration_number": "ABC123",
  "wash_type": "premium"
}
```

Response `201 Created`:

```json
{
  "id": 1,
  "registration_number": "ABC123",
  "wash_type": "premium",
  "status": "queued",
  "created_at": "2026-09-02T16:30:00Z",
  "updated_at": "2026-09-02T16:30:00Z"
}
```

With curl:

```bash
curl -X POST http://localhost:8080/washes \
  -H "Content-Type: application/json" \
  -d '{"registration_number":"ABC123","wash_type":"premium"}'
```

### Update status

`PATCH /washes/1/status`

```json
{ "status": "in_progress" }
```

### Validation rules

- `registration_number`: 2–10 letters/digits. Spaces and dashes are stripped and the value is
  uppercased, so `abc 123` is stored as `ABC123`.
- `wash_type`: `basic`, `standard` or `premium`.
- `status`: `queued`, `in_progress`, `done` or `cancelled`. Only these transitions are allowed:
  - `queued` → `in_progress` or `cancelled`
  - `in_progress` → `done` or `cancelled`
  - `done` and `cancelled` are final

Errors are returned as JSON with an appropriate status code:

| Code  | When                                                   |
|-------|--------------------------------------------------------|
| `400` | Invalid JSON, bad ID, or a field fails validation      |
| `404` | Wash does not exist                                    |
| `409` | Status transition not allowed (e.g. `queued` → `done`) |

```json
{ "error": "wash_type must be one of: basic, standard, premium" }
```

## Project structure

```
.
├── main.go              # entrypoint: config, routing, graceful shutdown
├── handlers/            # HTTP layer: JSON decoding, validation, status codes
├── models/              # Wash type, constants and validation rules
├── database/            # SQLite connection + all SQL (schema.sql is embedded)
├── tests/               # HTTP-level tests against a temporary SQLite file
├── Dockerfile           # multi-stage build, ~15 MB image
├── go.mod / go.sum
└── README.md
```
