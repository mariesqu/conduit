# Installation

## Prerequisites

- Go 1.24+
- Node 20+
- npm

## Build from source

### Windows

```powershell
git clone https://github.com/mariesqu/conduit.git
Set-Location conduit
./build.ps1 build
.\conduit.exe
```

### Linux/macOS

```bash
git clone https://github.com/mariesqu/conduit.git
cd conduit
make build
./conduit
```

## First run

Conduit creates `conduit.config.json` if missing, generates a token, detects shells, and starts on `127.0.0.1:7180` by default.

## Config location

`conduit.config.json` in the current working directory.

## Uninstall

Delete:

- `conduit`/`conduit.exe`
- `conduit.config.json`
- optional files root directory (default `~/Conduit-Files`)

## Platform notes

- Windows requires Windows 10 1809+ (ConPTY)
- Unix-like systems require PTY support
