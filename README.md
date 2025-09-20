# K0pern1cus

[![Go Version](https://img.shields.io/github/go-mod/go-version/rofleksey/k0pern1cus)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/docker-available-blue.svg)](Dockerfile)

K0pern1cus is a high-performance Go application that automatically streams curated Twitch clips to a live channel.
It fetches clips from specified broadcasters, processes them with FFmpeg, and streams them continuously to a Twitch RTMP endpoint.

## Features

- **Automated Clip Streaming**: Continuously streams clips from specified Twitch broadcasters
- **Smart Clip Selection**: Filters clips by game ID and date range
- **FFmpeg Processing**: Applies professional video processing with fade effects, scaling, and text overlays
- **Preloading System**: Preloads multiple clips for seamless transitions
- **Resilient Design**: Automatic retry mechanisms and error handling
- **Telegram Integration**: Error logging and notifications via Telegram
- **Docker Ready**: Containerized deployment with optimized Ubuntu base image

## Prerequisites

- Go 1.25+
- FFmpeg
- Twitch Developer Account with Client ID and Secret
- Twitch RTMP Stream Key

## Docker image
```bash
docker run -v $(pwd)/config.yaml:/opt/config.yaml rofleksey/k0pern1cus:latest
```

## Build

### From Source

```bash
git clone <repository-url>
cd k0pern1cus
go mod download
make build
./k0pern1cus
```

### Using Docker
```bash
docker build -t k0pern1cus .
docker run -v $(pwd)/config.yaml:/opt/config.yaml k0pern1cus
```

## Configuration
See config_example.yaml file for an example config.
