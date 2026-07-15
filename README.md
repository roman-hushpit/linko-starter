# Linko

This is a toy URL shortener project, to be used as the starter repo for the Logging and Telemetry course on [Boot.dev](https://www.boot.dev/).

It's intentionally small, a little messy, and realistic enough to practice adding logs, metrics, and traces in Go.

To build project with build tags, project need to be builded with next command:

```shell
go build  -ldflags "-X boot.dev/linko/internal/build.GitSHA=$(git rev-parse HEAD) -X boot.dev/linko/internal/build.BuildTime=$(date -u '+%Y-%m-%dT%H:%M:%SZ')" -o linko
```

Profiling:  
Go Tool pprof

CPU profiling
```shell
curl -u frodo:ofTheNineFingers "http://localhost:8899/debug/pprof/profile?seconds=30" --output cpu.prof

go tool pprof linko cpu.prof

```

Memory Profiling
```shell
curl -u frodo:ofTheNineFingers "http://localhost:8899/debug/pprof/heap?seconds=30" --output memory.prof

go tool pprof -dot linko memory.prof | dot -Tsvg -o memory.svg
```

Goroutine Profiling
```shell
go tool pprof /path/to/linko goroutine.prof\

go tool pprof -http=:0 /path/to/linko goroutine.prof
```
### What to Log

- Runtime environment name (e.g. production, staging, development)
- The server's hostname
- The server's IP address
- Cloud region
- Node name or IP (for containerized services)
- Docker container name or Kubernetes pod name
- Host operating system and kernel version
- Host/server time zone (particularly if you deploy across geographic regions)