# Spec: mobile-arrival-notice (bythos-movil-feedback)

## Purpose
Tell the desktop user, inside the app, when a file or link arrives from the phone, and keep the folder view current without a manual reload.

## ADDED Requirements

### Requirement: Arrival Polling
The desktop UI MUST poll `GET /api/eventos?limite=1&origen=celular` at an interval between 2 and 3 seconds while the window is visible. The UI MUST NOT require any new API, endpoint, or protocol change.

#### Scenario: Poll while visible
- GIVEN the desktop window is visible
- WHEN 2 to 3 seconds elapse
- THEN the UI issues one request to `/api/eventos?limite=1&origen=celular`

#### Scenario: Pause while hidden
- GIVEN `document.hidden` is true
- WHEN the polling interval elapses
- THEN no request is issued

#### Scenario: Resume on visibility
- GIVEN polling was paused because the window was hidden
- WHEN the window becomes visible again
- THEN polling resumes and a request is issued within one polling interval

### Requirement: New Event Detection
The UI MUST treat an event as an arrival only when its `id` differs from the last seen `id`. The first successful poll after the UI starts MUST establish the baseline and MUST NOT trigger a toast or a refresh.

#### Scenario: Baseline on startup
- GIVEN the UI has just loaded and the latest phone event has id 40
- WHEN the first poll returns id 40
- THEN no toast is shown and the id is stored as the baseline

#### Scenario: Same event repeated
- GIVEN the last seen id is 40
- WHEN a poll returns id 40
- THEN no toast is shown and no refresh is triggered

#### Scenario: New event
- GIVEN the last seen id is 40
- WHEN a poll returns id 41
- THEN the UI treats it as an arrival and the id 41 becomes the last seen id

#### Scenario: Poll failure
- GIVEN a poll request fails or returns an error
- WHEN the failure occurs
- THEN the UI MUST NOT show a toast, MUST keep the last seen id, and MUST retry on the next interval

### Requirement: Automatic Folder Refresh
On a new arrival the UI MUST reload the current folder and file data (the same effect as `recargar()`) without user action and without a page reload.

#### Scenario: File appears without F5
- GIVEN the user is viewing a folder in the desktop app
- WHEN the phone uploads a file into that folder
- THEN the file is listed within about 3 seconds without pressing F5

### Requirement: In-App Arrival Toast
On a new arrival the UI MUST show a transient in-app toast that names the sending device (the event `actor`) and the received file or link (from the event `detalle`). The toast MUST dismiss itself automatically. The system MUST NOT raise a Windows or OS-level notification.

#### Scenario: Toast content
- GIVEN a new arrival event with actor "SM-A546E" and a file name in `detalle`
- WHEN the UI detects it
- THEN a toast shows the device name and the file name

#### Scenario: Toast auto-dismiss
- GIVEN a toast is displayed
- WHEN a few seconds pass without interaction
- THEN the toast disappears on its own

#### Scenario: No OS notification
- GIVEN the window is minimized or hidden
- WHEN a file arrives
- THEN no OS notification is produced, and the toast and refresh occur when the window becomes visible and the next poll runs

#### Scenario: Several arrivals between polls
- GIVEN more than one event occurred between two polls
- WHEN the poll returns only the latest event
- THEN a single toast is shown for the latest event and the folder is refreshed once
