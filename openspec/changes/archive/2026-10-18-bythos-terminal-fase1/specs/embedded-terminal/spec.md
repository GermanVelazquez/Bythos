# embedded-terminal Specification

## Purpose
Run a local shell in a ConPTY and expose it to the Terminal screen over a WebSocket that only the Bythos UI can open.

## Requirements

### Requirement: Origin Check
The terminal WebSocket upgrade and the ticket endpoint MUST accept an Origin only if it exactly equals an allowed Bythos origin. An empty Origin and any `chrome-extension://` Origin MUST be rejected with 403. The dev origin on port 5173 MUST be accepted only when `BYTHOS_DEV=1`.

#### Scenario: Exact origin accepted
- GIVEN Origin equals the app origin and a valid ticket
- WHEN the client requests the upgrade
- THEN the upgrade succeeds

#### Scenario: Empty or extension origin
- GIVEN a request with no Origin, or `chrome-extension://abc`
- WHEN it hits the ticket endpoint or the upgrade
- THEN the response is 403 and no PTY is created

#### Scenario: Wrong origin
- GIVEN Origin `http://evil.example` or a prefix/suffix variant of the app origin
- WHEN the request is made
- THEN it is rejected with 403

#### Scenario: Dev origin gating
- GIVEN Origin `http://localhost:5173`
- WHEN `BYTHOS_DEV` is not `1` THEN 403; WHEN it is `1` THEN accepted

### Requirement: Host Check
The server MUST reject requests whose Host header is not a loopback host:port it serves. The checks MUST run in this order: Host, exact Origin, ticket consumption, WebSocket accept. A request rejected by an earlier check MUST NOT consume the ticket.

#### Scenario: Rebinding host
- GIVEN Host `attacker.example:PORT` with a valid Origin and ticket
- WHEN the upgrade is requested
- THEN it is rejected and the ticket is not consumed

### Requirement: One-Time Ticket
`POST` ticket endpoint, under the strict Origin and Host checks, MUST return a 128-bit random ticket bound to one session with a TTL of about 30 seconds. The upgrade MUST consume the ticket. A missing, expired, reused, unknown, or other-session ticket MUST be rejected.

#### Scenario: Valid ticket
- GIVEN a ticket issued for session A
- WHEN the upgrade for session A presents it within the TTL
- THEN the upgrade succeeds and the ticket is consumed

#### Scenario: Reused or expired
- GIVEN a consumed ticket, or one older than the TTL
- WHEN it is presented
- THEN the upgrade is rejected with 401/403

#### Scenario: Wrong session
- GIVEN a ticket issued for session A
- WHEN it is presented for session B
- THEN the upgrade is rejected

#### Scenario: Ticket endpoint under Origin check
- GIVEN a request with an empty or foreign Origin
- WHEN it asks for a ticket
- THEN no ticket is issued

### Requirement: Fixed Command and Limits
The PTY command MUST be fixed by the server (the user shell). No value from the URL, query, headers, or message body MAY be interpolated into the command line, and the system MUST NOT auto-run any AI CLI. The system MUST enforce a maximum of 8 concurrent PTYs and a per-message WebSocket read limit of 1 MiB.

#### Scenario: No injection
- GIVEN a session name or query containing `& calc`
- WHEN the PTY starts
- THEN the child command line is unchanged and nothing extra executes

#### Scenario: PTY cap
- GIVEN 8 PTYs are running
- WHEN a ninth is requested
- THEN it is refused with 429 and a clear error, and no process is started

#### Scenario: Read limit
- GIVEN a client message above 1 MiB
- WHEN it is received
- THEN the connection is closed and the PTY survives

### Requirement: Reattach and Replay
A browser reload or WebSocket drop MUST NOT kill the PTY. A new authenticated connection to the same session MUST reattach and receive the ring buffer replay (the last 256 KiB of output) before live output.

#### Scenario: Reload
- GIVEN a running PTY that printed "hello"
- WHEN the page reloads and reconnects with a new ticket
- THEN the same process is attached and "hello" is replayed

### Requirement: Process Cleanup
All PTY child processes (including descendants) MUST end when `bythos.exe` exits, on normal exit or crash. Whether closing the window ends sessions is left to design.

#### Scenario: App exit
- GIVEN two live PTYs running child processes
- WHEN `bythos.exe` terminates
- THEN no child process of either PTY remains

### Requirement: Platform Guard
The terminal MUST be unavailable with a clear Spanish message on Windows older than 1809 (build 17763).

#### Scenario: Old Windows
- GIVEN Windows build below 17763
- WHEN a terminal is requested
- THEN the UI shows "La terminal requiere Windows 10 versión 1809 o superior."
