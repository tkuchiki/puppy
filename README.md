# puppy

A lightweight CLI client for Datadog API, designed to efficiently retrieve and analyze monitors and service dependencies.

## Motivation

[Dogshell](https://github.com/DataDog/datadogpy) doesn't work well when dealing with large datasets. `puppy` was created to address this limitation by providing:

- Efficient pagination handling
- File-based caching to reduce API calls
- Automatic rate limiting and retry mechanisms
- Support for large-scale data retrieval

## Features

### Monitor Command

Retrieve a list of Datadog monitors with pagination support.

```bash
puppy monitor --api-key=<YOUR_API_KEY> --app-key=<YOUR_APP_KEY> --page-size=100
```

### Service Command (Under Development)

Analyze service dependencies using Datadog trace data. Supports two modes:

- **Fast mode**: Quickly retrieve external dependencies for a service
- **Accurate mode**: Detailed dependency analysis for specific endpoints by collecting trace data

```bash
# Fast mode - get external dependencies
puppy service --api-key=<YOUR_API_KEY> --app-key=<YOUR_APP_KEY> \
  --service=my-service \
  --env=prod \
  --mode=fast

# Accurate mode - detailed endpoint analysis
puppy service --api-key=<YOUR_API_KEY> --app-key=<YOUR_APP_KEY> \
  --service=my-service \
  --env=prod \
  --endpoint="GET /api/users" \
  --mode=accurate \
  --max-traces=1000
```

## Installation

```bash
go install github.com/tkuchiki/puppy@latest
```

Or build from source:

```bash
git clone https://github.com/tkuchiki/puppy.git
cd puppy
go build
```

## Usage

```
Usage: puppy <command> [flags]

Lightweight Datadog CLI client

Flags:
  -h, --help              Show context-sensitive help.
      --api-key=STRING    Datadog API key ($DD_CLIENT_API_KEY)
      --app-key=STRING    Datadog Application key ($DD_CLIENT_APP_KEY)
      --enable-retry      Enable retry mode
      --max-retries=3     Maximum number of retries to perform

Commands:
  monitor    Monitor commands
  service    Service commands

Run "puppy <command> --help" for more information on a command.
```

### Authentication

Provide your Datadog API credentials via command-line flags or environment variables:

**Command-line flags:**
```bash
puppy --api-key=<YOUR_API_KEY> --app-key=<YOUR_APP_KEY> <command>
```

**Environment variables:**
```bash
export DD_CLIENT_API_KEY=<YOUR_API_KEY>
export DD_CLIENT_APP_KEY=<YOUR_APP_KEY>
puppy <command>
```

### Monitor Command

```
Usage: puppy monitor [flags]

Monitor commands

Flags:
  -h, --help                Show context-sensitive help.
      --api-key=STRING      Datadog API key ($DD_CLIENT_API_KEY)
      --app-key=STRING      Datadog Application key ($DD_CLIENT_APP_KEY)
      --enable-retry        Enable retry mode
      --max-retries=3       Maximum number of retries to perform

  -p, --page-size=INT-32    Maximum number of results to return
```

### Service Command

```
Usage: puppy service [flags]

Service commands

Flags:
  -h, --help                    Show context-sensitive help.
      --api-key=STRING          Datadog API key ($DD_CLIENT_API_KEY)
      --app-key=STRING          Datadog Application key ($DD_CLIENT_APP_KEY)
      --enable-retry            Enable retry mode
      --max-retries=3           Maximum number of retries to perform

      --site="datadoghq.com"    Datadog site
      --cache-dir="./cache"     Cache directory
      --cache-ttl=1h            Cache TTL
      --mode="fast"             accurate or fast
      --service=STRING          Service name
      --env=STRING              Environment name
      --endpoint=STRING         Endpoint
      --loopback=1h             Loopback interval
      --from=TIME               From
      --to=TIME                 To
      --page-limit=INT          Page limit
      --max-traces=INT          Maximum number of traces
```

## Output Format

All commands output results in JSON format for easy integration with other tools.

### Monitor Command Output

```json
[
  {
    "id": 12345,
    "name": "Monitor Name",
    "type": "metric alert",
    ...
  }
]
```

### Service Command Output (Fast Mode)

```json
{
  "mode": "fast",
  "site": "datadoghq.com",
  "service": "my-service",
  "env": "prod",
  "from": "2025-12-26T10:00:00Z",
  "to": "2025-12-26T11:00:00Z",
  "approximate": true,
  "external_deps": {
    "GET /api/endpoint": {
      "postgres-db": 150,
      "redis-cache": 89
    }
  }
}
```

### Service Command Output (Accurate Mode)

```json
{
  "mode": "accurate",
  "site": "datadoghq.com",
  "service": "my-service",
  "env": "prod",
  "incoming_endpoint": "GET /api/users",
  "from": "2025-12-26T10:00:00Z",
  "to": "2025-12-26T11:00:00Z",
  "collected_trace_ids": 1000,
  "external_deps": {
    "postgres-db": 450,
    "redis-cache": 250
  },
  "internal_services": {
    "auth-service": 150,
    "profile-service": 100
  }
}
```

## Technical Details

### Caching

To minimize API calls and respect rate limits, `puppy` implements a file-based caching mechanism:

- Cache files are stored in the directory specified by `--cache-dir` (default: `./cache`)
- Cache keys are generated using SHA256 hashes of request parameters
- Cache TTL is configurable via `--cache-ttl` (default: 1 hour)

### Rate Limiting

`puppy` automatically handles Datadog API rate limits by:

- Reading rate limit headers from API responses
- Implementing exponential backoff with jitter
- Configurable retry mechanism

## Requirements

- Go 1.24.3 or later
- Valid Datadog API key and Application key

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
