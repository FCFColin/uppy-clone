# Continuous Profiling

## Overview
The system supports always-on continuous profiling via Pyroscope or Parca, integrated with Grafana dashboards.

## Setup

### Prerequisites
- Pyroscope server (or Grafana Cloud Profiles)
- `ENABLE_PYROSCOPE=true` environment variable
- `PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040`

### Profile Types
- **CPU**: Identifies hot paths and optimization opportunities
- **Alloc Objects**: Memory allocation count (GC pressure)
- **Alloc Space**: Memory allocation size
- **Inuse Objects**: Current live objects (memory leaks)
- **Inuse Space**: Current memory usage

### Grafana Dashboard
Import the Pyroscope panel in Grafana to view:
- Real-time CPU flame graphs
- Memory allocation flame graphs
- Diff views (compare before/after deployments)

## Usage
1. Deploy Pyroscope server: `docker run -p 4040:4040 grafana/pyroscope:latest`
2. Set environment variables
3. Deploy the application
4. Open Grafana → Explore → Pyroscope to view flame graphs

## pprof (On-Demand)
For on-demand profiling without Pyroscope, use the built-in pprof endpoints:
- `ENABLE_PPROF=true` to enable
- `curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof`
- `go tool pprof cpu.prof`
