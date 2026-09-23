# Google Health Data Retriever

Pulls the last 7 days of daily step totals and sleep times from the Google Health API (Fitbit data) over gRPC, and writes them to a CSV. If the CSV already exists, the data is appended underneath, so it can be combined with other exports such as MyFitnessPal macros.

## Setup

### 1. Google Cloud project

1. Create a project in the [Google Cloud console](https://console.cloud.google.com/).
2. Enable the **Google Health API** for the project.
3. Under **Google Auth Platform**, configure the consent screen with the **External** audience.
4. Under **Data Access**, add these scopes:
   - `https://www.googleapis.com/auth/googlehealth.activity_and_fitness.readonly`
   - `https://www.googleapis.com/auth/googlehealth.sleep.readonly`
5. Under **Audience → Test users**, add the Google account that holds your Fitbit data.
6. Under **Clients**, create an OAuth client of type **Desktop application**.
7. Download the client JSON, rename it to `client_secret.json`, and put it in the project folder.

### 2. Install Go

Install Go from [go.dev](https://go.dev/dl/), then check it's working:
```powershell
go version
```

### 3. Build

Dependencies are pinned in `go.mod` and `go.sum`, and the generated gRPC code is already in `gen/healthpb`, so building is one step:
```powershell
go build -o health-fetch.exe
```

## Usage

```powershell
.\health-fetch.exe
```
or, without building first:
```powershell
go run .
```

By default it writes `week_macros.csv` in the current folder. Use `-csv` to point it at another file, for example an existing macros export to append to:
```powershell
.\health-fetch.exe -csv "C:\path\to\week_macros.csv"
```

On first run, a browser window opens for Google sign-in. After you approve access, the token is saved to `token.json` and refreshed automatically on later runs.

### Output

```
2026-09-17 to 2026-09-23
Date,Steps,Asleep,Woke,Sleep (h)
2026-09-17,14119,,,
2026-09-18,14999,23:18,07:23,6.9
...
```
- **Steps** is the daily total, using the API's daily rollup.
- **Asleep / Woke** are local times. Each night is counted against the day you woke up.
- **Sleep (h)** is time actually asleep, not time in bed.
- Days without data are left blank. If today looks empty, sync your watch in the Fitbit or Google Health app and run it again.

## Regenerating the gRPC code (optional)

Only needed if you want to rebuild `gen/healthpb` from Google's latest API definitions. Building and running the program doesn't need any of this.

The `.proto` definitions come from Google's [googleapis](https://github.com/googleapis/googleapis) repo. `gen.ps1` clones it into `googleapis/` the first time it runs. That folder is git-ignored because it's large and only used as input to `protoc`, not by the program itself.

1. Install `protoc` from the [protobuf releases](https://github.com/protocolbuffers/protobuf/releases). Extract the whole zip, including the `include` folder next to `bin`, and add `bin` to your PATH.
2. Install the Go plugins, and make sure `%USERPROFILE%\go\bin` is on your PATH:
   ```powershell
   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
   ```
3. Run the generator, which clones `googleapis` on first use:
   ```powershell
   .\gen.ps1
   go mod tidy
   ```

## Project structure

| File | Purpose |
|---|---|
| `main.go` | Entry point: sets up the gRPC connection, fetches steps and sleep, builds the CSV rows |
| `auth.go` | OAuth 2.0 desktop flow (local loopback server, PKCE), token storage and refresh |
| `export.go` | Creates the CSV, or appends to it if it already exists |
| `gen/healthpb/` | Go code generated from the Google Health API `.proto` files |
| `gen.ps1` | Regenerates `gen/healthpb` |

## Notes

- While the Cloud project is in **Testing** status, Google expires refresh tokens after 7 days.
- If you change the scopes in `auth.go`, delete `token.json` so the consent screen asks for the new permissions.