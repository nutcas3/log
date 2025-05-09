# API-WatchTower

A comprehensive Go-based monitoring, logging, and analytics platform for API health monitoring and log analysis.

## Overview

API-WatchTower provides:
- External API monitoring and health checks
- Application log ingestion and analysis
- AI-powered log analysis and anomaly detection
- Unified view correlating API health with application behavior
- Robust alerting system

## Features

- **External API Monitoring**
  - Configurable endpoint monitoring
  - Performance tracking
  - Custom assertion rules
  - Flexible authentication support

- **Log Management**
  - JSON log ingestion
  - Scalable storage
  - Advanced search and filtering
  - AI-powered analysis

- **Smart Analytics**
  - Anomaly detection
  - Error pattern clustering
  - Trend analysis
  - Root cause suggestions

- **Alerting**
  - Configurable alert rules
  - Multiple notification channels
  - Alert management system

## Getting Started

### Prerequisites

- Go 1.21 or later
- PostgreSQL 15.0 or later with TimescaleDB extension
- Docker and Docker Compose (for containerized deployment)

### Installation

1. Clone the repository
2. Install dependencies:
```bash
go mod download
```

3. Set up the database:
```bash
docker-compose up -d db
```

4. Run migrations:
```bash
make migrate
```

5. Start the server:
```bash
make run
```

## Project Structure

```
.
├── cmd/                  # Application entry points
├── internal/            
│   ├── api/             # API handlers and routes
│   ├── config/          # Configuration management
│   ├── core/            # Core business logic
│   ├── db/              # Database models and migrations
│   ├── monitoring/      # External API monitoring
│   ├── log/             # Log ingestion and analysis
│   ├── ai/              # AI/ML analysis components
│   └── alert/           # Alerting system
├── pkg/                 # Public packages
└── scripts/             # Utility scripts
```

## Configuration

Configuration is handled through environment variables or a config file. See `.env.example` for available options.

## API Documentation

API documentation is available at `/swagger/index.html` when running in development mode.

## License

MIT License
