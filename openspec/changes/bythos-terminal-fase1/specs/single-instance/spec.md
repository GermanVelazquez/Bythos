# single-instance Specification

## Purpose
Only one Bythos process serves the app at a time.

## Requirements

### Requirement: Single Server
A second launch MUST NOT start a second server. It MUST focus or open the existing window and exit.

#### Scenario: Second launch
- GIVEN Bythos is running
- WHEN `bythos.exe` is launched again
- THEN no new listener starts, the existing window is shown, and the new process exits

#### Scenario: Stale state
- GIVEN the previous process crashed
- WHEN Bythos is launched
- THEN it starts normally

### Requirement: Installer Detection
The installer and uninstaller MUST detect the running app via `AppMutex` and prompt to close it.

#### Scenario: Upgrade while running
- GIVEN Bythos is running
- WHEN the installer runs
- THEN it asks the user to close the app before continuing

#### Scenario: Uninstall while running
- GIVEN Bythos is running
- WHEN the uninstaller runs
- THEN it asks the user to close the app first
